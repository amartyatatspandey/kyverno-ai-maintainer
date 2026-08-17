// Package ghx is the executor's GitHub layer — the ONLY code that touches
// credentials (via the gh CLI's auth). Mutating calls require a policy
// Decision passed in; they fail closed without one (POLICY invariant 1).
package ghx

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/amartyatatspandey/kyverno-ai-maintainer/internal/policy"
)

type Client struct {
	Repo   string // "owner/name"
	DryRun bool
}

func run(args ...string) ([]byte, error) {
	out, err := exec.Command("gh", args...).CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("gh %s: %v: %s", strings.Join(args[:min(3, len(args))], " "), err, truncate(string(out), 300))
	}
	return out, nil
}

// ---- reads (no Decision required) ----

type prView struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	State  string `json:"state"`
	Body   string `json:"body"`
	Author struct {
		Login string `json:"login"`
		IsBot bool   `json:"is_bot"`
	} `json:"author"`
	AuthorAssociation string `json:"authorAssociation"`
	BaseRefName       string `json:"baseRefName"`
	HeadRefOid        string `json:"headRefOid"`
	IsDraft           bool   `json:"isDraft"`
	Mergeable         string `json:"mergeable"`
	Labels            []struct {
		Name string `json:"name"`
	} `json:"labels"`
	Files []struct {
		Path string `json:"path"`
	} `json:"files"`
	StatusCheckRollup []struct {
		Name       string `json:"name"`
		Status     string `json:"status"`
		Conclusion string `json:"conclusion"`
	} `json:"statusCheckRollup"`
	CreatedAt time.Time `json:"createdAt"`
}

// GetPRFacts fetches FRESH facts. Called by context assembly AND re-called
// by the policy path right before ALLOW (fetch-fresh rule).
func (c *Client) GetPRFacts(number int) (*policy.PRFacts, string, error) {
	out, err := run("pr", "view", fmt.Sprint(number), "-R", c.Repo, "--json",
		"number,title,state,body,author,authorAssociation,baseRefName,headRefOid,isDraft,mergeable,labels,files,statusCheckRollup,createdAt")
	if err != nil {
		return nil, "", err
	}
	var v prView
	if err := json.Unmarshal(out, &v); err != nil {
		return nil, "", err
	}
	f := &policy.PRFacts{
		Number: v.Number, Title: v.Title, State: v.State,
		AuthorLogin: normalizeBot(v.Author.Login, v.Author.IsBot), AuthorIsBot: v.Author.IsBot,
		AuthorAssociation: v.AuthorAssociation,
		BaseRef:           v.BaseRefName, HeadSHA: v.HeadRefOid, IsDraft: v.IsDraft,
		Mergeable: v.Mergeable == "MERGEABLE",
	}
	for _, l := range v.Labels {
		f.Labels = append(f.Labels, l.Name)
	}
	for _, fl := range v.Files {
		f.ChangedFiles = append(f.ChangedFiles, fl.Path)
	}
	f.ChecksGreen, f.ChecksPending = summarizeChecks(v.StatusCheckRollup)
	return f, v.Body, nil
}

type prCommitsView struct {
	Commits []struct {
		OID             string `json:"oid"`
		MessageHeadline string `json:"messageHeadline"`
		MessageBody     string `json:"messageBody"`
		Authors         []struct {
			Name  string `json:"name"`
			Email string `json:"email"`
			Login string `json:"login"`
		} `json:"authors"`
	} `json:"commits"`
}

// GetCommitFacts reads commit SHAs and messages from `gh pr view --json commits`
// (git trailers), never from PR body text.
func (c *Client) GetCommitFacts(prNumber int) (*policy.CommitFacts, error) {
	out, err := run("pr", "view", fmt.Sprint(prNumber), "-R", c.Repo, "--json", "commits")
	if err != nil {
		return nil, err
	}
	return commitFactsFromJSON(out)
}

func commitFactsFromJSON(raw []byte) (*policy.CommitFacts, error) {
	var v prCommitsView
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	f := &policy.CommitFacts{}
	for _, cm := range v.Commits {
		f.SHAs = append(f.SHAs, cm.OID)
		name, email := "", ""
		if len(cm.Authors) > 0 {
			name, email = cm.Authors[0].Name, cm.Authors[0].Email
		}
		msg := cm.MessageHeadline
		if cm.MessageBody != "" {
			msg = cm.MessageHeadline + "\n\n" + cm.MessageBody
		}
		f.SignedOff = append(f.SignedOff, signedOffByAuthor(msg, name, email))
	}
	return f, nil
}

// signedOffByAuthor reports whether the commit message carries a Signed-off-by
// trailer whose email matches the commit author (DCO).
func signedOffByAuthor(message, authorName, authorEmail string) bool {
	email := strings.ToLower(strings.TrimSpace(authorEmail))
	name := strings.ToLower(strings.TrimSpace(authorName))
	for _, line := range strings.Split(message, "\n") {
		trim := strings.TrimSpace(line)
		lower := strings.ToLower(trim)
		if !strings.HasPrefix(lower, "signed-off-by:") {
			continue
		}
		rest := strings.TrimSpace(trim[len("signed-off-by:"):])
		restLower := strings.ToLower(rest)
		if email != "" && strings.Contains(restLower, email) {
			return true
		}
		if email == "" && name != "" && strings.Contains(restLower, name) {
			return true
		}
	}
	return false
}

// dependabot's API login is "dependabot" with is_bot; policy allowlists "app/dependabot".
func normalizeBot(login string, isBot bool) string {
	if isBot && !strings.HasPrefix(login, "app/") {
		return "app/" + login
	}
	return login
}

func summarizeChecks(checks []struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
}) (green, pending bool) {
	if len(checks) == 0 {
		return false, true // no checks reported => treat as pending, never green
	}
	green = true
	for _, ch := range checks {
		switch strings.ToUpper(ch.Conclusion) {
		case "SUCCESS", "NEUTRAL", "SKIPPED":
		case "": // still running
			pending = true
		default:
			green = false
		}
	}
	return green && !pending, pending
}

type IssueView struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Body   string `json:"body"`
	State  string `json:"state"`
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
}

func (c *Client) GetIssue(number int) (*IssueView, error) {
	out, err := run("issue", "view", fmt.Sprint(number), "-R", c.Repo, "--json", "number,title,body,state,labels")
	if err != nil {
		return nil, err
	}
	var v IssueView
	return &v, json.Unmarshal(out, &v)
}

// KillSwitchActive reads the repo variable fresh.
func (c *Client) KillSwitchActive(variable string) bool {
	out, err := run("variable", "get", variable, "-R", c.Repo, "--json", "value", "-q", ".value")
	if err != nil {
		return false // variable absent => not paused
	}
	return strings.TrimSpace(string(out)) == "true"
}

// ---- mutations (Decision required; fail closed) ----

func (c *Client) requireAllow(d *policy.Decision, action string) error {
	if d == nil || !d.Allowed {
		return fmt.Errorf("SAFETY: %s attempted without ALLOW decision — refusing (fail closed)", action)
	}
	if time.Now().After(d.ExpiresAt) {
		return fmt.Errorf("SAFETY: decision for %s expired — refusing", action)
	}
	return nil
}

// UpsertComment: one comment per (entity, marker), edited in place (G5).
func (c *Client) UpsertComment(d *policy.Decision, kind string, number int, marker, body string) (string, error) {
	if err := c.requireAllow(d, "comment"); err != nil {
		return "", err
	}
	body = body + "\n\n<!-- " + marker + " -->"
	if c.DryRun {
		return "(dry-run) would upsert comment on " + fmt.Sprint(number), nil
	}
	// Find existing comment with marker.
	out, _ := run("api", fmt.Sprintf("repos/%s/issues/%d/comments?per_page=100", c.Repo, number),
		"--jq", `[.[] | {id, body}]`)
	var comments []struct {
		ID   int64  `json:"id"`
		Body string `json:"body"`
	}
	json.Unmarshal(out, &comments)
	markerPrefix := strings.SplitN(marker, ":", 2)[0]
	for _, cm := range comments {
		if strings.Contains(cm.Body, "<!-- "+markerPrefix) {
			_, err := run("api", "-X", "PATCH", fmt.Sprintf("repos/%s/issues/comments/%d", c.Repo, cm.ID), "-f", "body="+body)
			return fmt.Sprintf("updated comment %d", cm.ID), err
		}
	}
	_, err := run("api", "-X", "POST", fmt.Sprintf("repos/%s/issues/%d/comments", c.Repo, number), "-f", "body="+body)
	return "created comment", err
}

func (c *Client) SetLabels(d *policy.Decision, number int, add, remove []string) (string, error) {
	if err := c.requireAllow(d, "set_labels"); err != nil {
		return "", err
	}
	if c.DryRun {
		return fmt.Sprintf("(dry-run) would add %v remove %v", add, remove), nil
	}
	args := []string{"issue", "edit", fmt.Sprint(number), "-R", c.Repo}
	for _, l := range add {
		args = append(args, "--add-label", l)
	}
	for _, l := range remove {
		args = append(args, "--remove-label", l)
	}
	_, err := run(args...)
	return fmt.Sprintf("labels add=%v remove=%v", add, remove), err
}

// MergePR merges with expected head SHA (TOCTOU guard, RISKS G2).
func (c *Client) MergePR(d *policy.Decision, number int, method string) (string, error) {
	if err := c.requireAllow(d, "merge_pr"); err != nil {
		return "", err
	}
	if d.BoundSHA == "" {
		return "", fmt.Errorf("SAFETY: merge decision has no bound SHA — refusing")
	}
	if c.DryRun {
		return "(dry-run) would squash-merge PR " + fmt.Sprint(number) + " at " + d.BoundSHA, nil
	}
	out, err := run("api", "-X", "PUT", fmt.Sprintf("repos/%s/pulls/%d/merge", c.Repo, number),
		"-f", "merge_method="+method, "-f", "sha="+d.BoundSHA)
	if err != nil {
		return "", err
	}
	var res struct {
		SHA    string `json:"sha"`
		Merged bool   `json:"merged"`
	}
	json.Unmarshal(out, &res)
	return res.SHA, nil
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
