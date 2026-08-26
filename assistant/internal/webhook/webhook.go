// Package webhook is the GitHub webhook adapter: HMAC-verified ingress that
// calls the same runtime entrypoint as `assistant run --pr N` / `run --issue N`.
// It is a new trigger, not a new authority — mutating work still goes through
// policy.Engine.Evaluate inside those run methods.
package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
)

// DefaultMaxBody is 1 MiB. GitHub documents a 25 MiB cap; PR/issue metadata
// events are far smaller, and a tight cap is the POC-honest choice.
const DefaultMaxBody int64 = 1 << 20

// RunFunc is the seam shared with the CLI: wired to Runner.RunDependencyPR
// or Runner.RunIssueTriage. The webhook package never reimplements those flows.
type RunFunc func(ctx context.Context, number int) error

// Handler is an http.Handler for GitHub's webhook POST.
type Handler struct {
	secret   []byte
	maxBody  int64
	runPR    RunFunc
	runIssue RunFunc

	// seen dedupes on delivery GUID. In-memory is enough for the POC;
	// persistent storage (and the fuller G3 key of delivery+event+entity+SHA)
	// is a production follow-up — must-have vs nice-to-have, same honesty
	// pattern as AUDIT.md's hash-chain.
	mu   sync.Mutex
	seen map[string]struct{}
}

// Options constructs a Handler. Secret must be the GitHub webhook secret
// (from AI_WEBHOOK_SECRET, never a CLI flag).
type Options struct {
	Secret   string
	MaxBody  int64 // 0 → DefaultMaxBody
	RunPR    RunFunc
	RunIssue RunFunc
}

func New(opts Options) *Handler {
	max := opts.MaxBody
	if max <= 0 {
		max = DefaultMaxBody
	}
	return &Handler{
		secret:   []byte(opts.Secret),
		maxBody:  max,
		runPR:    opts.RunPR,
		runIssue: opts.RunIssue,
		seen:     map[string]struct{}{},
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if cl := r.Header.Get("Content-Length"); cl != "" {
		n, ok := parseNonNegInt(cl)
		if !ok {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if n > h.maxBody {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
	}

	ct := r.Header.Get("Content-Type")
	if mediaType(ct) != "application/json" {
		w.WriteHeader(http.StatusUnsupportedMediaType)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, h.maxBody)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	sig := r.Header.Get("X-Hub-Signature-256")
	if !validSignature(h.secret, body, sig) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	event := r.Header.Get("X-GitHub-Event")
	delivery := r.Header.Get("X-GitHub-Delivery")
	if event == "" || delivery == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if event == "ping" {
		w.WriteHeader(http.StatusOK)
		return
	}

	if h.alreadySeen(delivery) {
		w.WriteHeader(http.StatusOK)
		return
	}

	switch event {
	case "pull_request", "issues":
	default:
		w.WriteHeader(http.StatusOK)
		return
	}

	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	switch event {
	case "pull_request":
		if !prAction(env.Action) || env.PullRequest == nil || env.PullRequest.Number <= 0 {
			w.WriteHeader(http.StatusOK)
			return
		}
		if h.runPR == nil {
			h.forget(delivery)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if err := h.runPR(ctx, env.PullRequest.Number); err != nil {
			h.forget(delivery)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	case "issues":
		if !issueAction(env.Action) || env.Issue == nil || env.Issue.Number <= 0 {
			w.WriteHeader(http.StatusOK)
			return
		}
		if h.runIssue == nil {
			h.forget(delivery)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if err := h.runIssue(ctx, env.Issue.Number); err != nil {
			h.forget(delivery)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}
	w.WriteHeader(http.StatusOK)
}

type envelope struct {
	Action      string `json:"action"`
	PullRequest *struct {
		Number int `json:"number"`
	} `json:"pull_request"`
	Issue *struct {
		Number int `json:"number"`
	} `json:"issue"`
}

func prAction(a string) bool {
	switch a {
	case "opened", "synchronize", "reopened", "ready_for_review":
		return true
	}
	return false
}

func issueAction(a string) bool {
	switch a {
	case "opened", "reopened":
		return true
	}
	return false
}

func (h *Handler) alreadySeen(id string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.seen[id]; ok {
		return true
	}
	h.seen[id] = struct{}{}
	return false
}

func (h *Handler) forget(id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.seen, id)
}

// validSignature checks X-Hub-Signature-256 against HMAC-SHA256(secret, body)
// using hmac.Equal (constant-time). Missing/malformed/wrong → false.
func validSignature(secret, body []byte, header string) bool {
	const prefix = "sha256="
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	got, err := hex.DecodeString(header[len(prefix):])
	if err != nil || len(got) != sha256.Size {
		return false
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	return hmac.Equal(mac.Sum(nil), got)
}

func mediaType(ct string) string {
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = ct[:i]
	}
	return strings.ToLower(strings.TrimSpace(ct))
}

func parseNonNegInt(s string) (int64, bool) {
	if s == "" {
		return 0, false
	}
	var n int64
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int64(c-'0')
		if n < 0 {
			return 0, false
		}
	}
	return n, true
}
