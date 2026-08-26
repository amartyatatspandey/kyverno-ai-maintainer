package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

const testSecret = "test-webhook-secret"

func sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func post(h http.Handler, event, delivery, sig string, body []byte) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if event != "" {
		req.Header.Set("X-GitHub-Event", event)
	}
	if delivery != "" {
		req.Header.Set("X-GitHub-Delivery", delivery)
	}
	if sig != "" {
		req.Header.Set("X-Hub-Signature-256", sig)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestValidSignatureAccepted(t *testing.T) {
	var got atomic.Int64
	h := New(Options{
		Secret: testSecret,
		RunPR: func(_ context.Context, n int) error {
			got.Store(int64(n))
			return nil
		},
	})
	body := []byte(`{"action":"opened","pull_request":{"number":17067}}`)
	rec := post(h, "pull_request", "del-valid", sign(testSecret, body), body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want 200", rec.Code)
	}
	if got.Load() != 17067 {
		t.Fatalf("ran pr=%d want 17067", got.Load())
	}
}

func TestInvalidSignatureRejected(t *testing.T) {
	var ran atomic.Bool
	h := New(Options{
		Secret: testSecret,
		RunPR: func(context.Context, int) error {
			ran.Store(true)
			return nil
		},
	})
	body := []byte(`{"action":"opened","pull_request":{"number":1}}`)
	sig := sign(testSecret, body)
	// Corrupt one hex nibble of the MAC.
	bad := sig[:len(sig)-1] + "0"
	if bad == sig {
		bad = sig[:len(sig)-1] + "1"
	}
	rec := post(h, "pull_request", "del-bad", bad, body)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401", rec.Code)
	}
	if ran.Load() {
		t.Fatal("invalid signature must not trigger a run")
	}
}

func TestMissingSignatureRejected(t *testing.T) {
	h := New(Options{Secret: testSecret})
	body := []byte(`{"zen":"ping"}`)
	rec := post(h, "ping", "del-nosig", "", body)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401", rec.Code)
	}
}

func TestOversizedBodyRejected(t *testing.T) {
	h := New(Options{Secret: testSecret, MaxBody: 32})
	body := bytes.Repeat([]byte("a"), 64)
	rec := post(h, "ping", "del-big", sign(testSecret, body), body)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status=%d want 413", rec.Code)
	}
}

func TestDuplicateDeliveryIDShortCircuited(t *testing.T) {
	var n atomic.Int64
	h := New(Options{
		Secret: testSecret,
		RunPR: func(context.Context, int) error {
			n.Add(1)
			return nil
		},
	})
	body := []byte(`{"action":"opened","pull_request":{"number":9}}`)
	sig := sign(testSecret, body)
	first := post(h, "pull_request", "same-guid", sig, body)
	second := post(h, "pull_request", "same-guid", sig, body)
	if first.Code != http.StatusOK || second.Code != http.StatusOK {
		t.Fatalf("status first=%d second=%d want 200/200", first.Code, second.Code)
	}
	if n.Load() != 1 {
		t.Fatalf("runs=%d want 1 (duplicate delivery must not re-run)", n.Load())
	}
}

func TestPingReturns200WithoutRun(t *testing.T) {
	var ran atomic.Bool
	h := New(Options{
		Secret: testSecret,
		RunPR: func(context.Context, int) error {
			ran.Store(true)
			return nil
		},
		RunIssue: func(context.Context, int) error {
			ran.Store(true)
			return nil
		},
	})
	body := []byte(`{"zen":"Non-blocking is better than blocking.","hook_id":1}`)
	rec := post(h, "ping", "del-ping", sign(testSecret, body), body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want 200", rec.Code)
	}
	if ran.Load() {
		t.Fatal("ping must not trigger a run")
	}
}

func TestIssuesEventCallsIssueEntrypoint(t *testing.T) {
	var got atomic.Int64
	h := New(Options{
		Secret: testSecret,
		RunIssue: func(_ context.Context, n int) error {
			got.Store(int64(n))
			return nil
		},
	})
	body := []byte(`{"action":"opened","issue":{"number":42}}`)
	rec := post(h, "issues", "del-issue", sign(testSecret, body), body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want 200", rec.Code)
	}
	if got.Load() != 42 {
		t.Fatalf("ran issue=%d want 42", got.Load())
	}
}
