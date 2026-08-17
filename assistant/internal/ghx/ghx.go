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
	Dir    string // optional local checkout (git tag → date for changelogs)
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

const prJSONFields = "number,title,state,body,author,authorAssociation,baseRefName,headRefOid,isDraft,mergeable,labels,files,statusCheckRollup,createdAt"

// GetPRFacts fetches FRESH facts. Called by context assembly AND re-called
// by the policy path right before ALLOW (fetch-fresh rule).
func (c *Client) GetPRFacts(number int) (*policy.PRFacts, string, error) {
	out, err := run("pr", "view", fmt.Sprint(number), "-R", c.Repo, "--json", prJSONFields)
	if err != nil {
		return nil, "", err
	}
	var v prView
	if err := json.Unmarshal(out, &v); err != nil {
		return nil, "", err
	}
	return prFactsFromView(v), v.Body, nil
}

func prFactsFromView(v prView) *policy.PRFacts {
	f := &policy.PRFacts{
		Number: v.Number, Title: v.Title, State: v.State,
		AuthorLogin: normalizeBot(v.Author.Login, v.Author.IsBot), AuthorIsBot: v.Author.IsBot,
		AuthorAssociation: v.AuthorAssociation,
		BaseRef:           v.BaseRefName, HeadSHA: v.HeadRefOid, IsDraft: v.IsDraft,
		Mergeable: v.Mergeable == "MERGEABLE",
		CreatedAt: v.CreatedAt,
	}
	for _, l := range v.Labels {
		f.Labels = append(f.Labels, l.Name)
	}
	for _, fl := range v.Files {
		f.ChangedFiles = append(f.ChangedFiles, fl.Path)
	}
	f.ChecksGreen, f.ChecksPending = summarizeChecks(v.StatusCheckRollup)
	return f
}

func prFactsFromListJSON(raw []byte) ([]policy.PRFacts, error) {
	var vs []prView
	if err := json.Unmarshal(raw, &vs); err != nil {
		return nil, err
	}
	out := make([]policy.PRFacts, 0, len(vs))
	for _, v := range vs {
		out = append(out, *prFactsFromView(v))
	}
	return out, nil
}

// ListOpenPRs lists open PRs using the same JSON field set as GetPRFacts.
func (c *Client) ListOpenPRs() ([]policy.PRFacts, error) {
	out, err := run("pr", "list", "-R", c.Repo, "--state", "open", "--limit", "100", "--json", prJSONFields)
	if err != nil {
		return nil, err
	}
	return prFactsFromListJSON(out)
}

func resolveSinceDate(ref, dir string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", fmt.Errorf("since ref is empty")
	}
	if t, err := time.Parse("2006-01-02", ref); err == nil {
		return t.Format("2006-01-02"), nil
	}
	cmd := exec.Command("git", "log", "-1", "--format=%aI", ref)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("resolve %q as git ref: %w", ref, err)
	}
	raw := strings.TrimSpace(string(out))
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return "", fmt.Errorf("parse git date %q: %w", raw, err)
	}
	return t.UTC().Format("2006-01-02"), nil
}

func mergedSearchQuery(date string) string {
	return fmt.Sprintf("is:pr is:merged merged:>=%s", date)
}

// ListMergedPRsSince lists PRs merged on or after ref (ISO date YYYY-MM-DD
// or a git tag resolved via `git log -1 --format=%aI`).
func (c *Client) ListMergedPRsSince(ref string) ([]policy.PRFacts, error) {
	date, err := resolveSinceDate(ref, c.Dir)
	if err != nil {
		return nil, err
	}
	out, err := run("pr", "list", "-R", c.Repo, "--state", "merged",
		"--search", mergedSearchQuery(date), "--limit", "100", "--json", prJSONFields)
	if err != nil {
		return nil, err
	}
	return prFactsFromListJSON(out)
}

func issueCountFromJSON(raw []byte) (int, error) {
	var items []struct {
		Number int `json:"number"`
	}
	if err := json.Unmarshal(raw, &items); err != nil {
		return 0, err
	}
	return len(items), nil
}

// ListOpenIssuesByLabel returns the count of open issues with the given label.
func (c *Client) ListOpenIssuesByLabel(label string) (int, error) {
	out, err := run("issue", "list", "-R", c.Repo, "--state", "open", "--label", label,
		"--limit", "1000", "--json", "number")
	if err != nil {
		return 0, err
	}
	return issueCountFromJSON(out)
}

type checkRunRow struct {
	Name       string    `json:"name"`
	Conclusion string    `json:"conclusion"`
	CreatedAt  time.Time `json:"createdAt"`
}

func failureRatesFromJSON(raw []byte, since time.Time) (map[string]float64, error) {
	var rows []checkRunRow
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, err
	}
	type acc struct{ fail, total int }
	by := map[string]*acc{}
	for _, r := range rows {
		if r.CreatedAt.Before(since) {
			continue
		}
		a := by[r.Name]
		if a == nil {
			a = &acc{}
			by[r.Name] = a
		}
		a.total++
		if strings.EqualFold(r.Conclusion, "failure") {
			a.fail++
		}
	}
	out := map[string]float64{}
	for name, a := range by {
		if a.total == 0 {
			continue
		}
		out[name] = float64(a.fail) / float64(a.total)
	}
	return out, nil
}

// RecentCheckRunFailureRate is a coarse per-workflow failure rate over the
// last `days` days (not per-suite; that's flaky_detection).
func (c *Client) RecentCheckRunFailureRate(days int) (map[string]float64, error) {
	out, err := run("run", "list", "-R", c.Repo, "--limit", "200", "--json", "conclusion,name,createdAt")
	if err != nil {
		return nil, err
	}
	since := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	return failureRatesFromJSON(out, since)
}

// CheckRunRecord is one job of one workflow run (per-suite granularity).
type CheckRunRecord struct {
	SHA        string
	Conclusion string
	JobName    string
	CreatedAt  time.Time
}

type jobsView struct {
	Jobs []struct {
		Name       string    `json:"name"`
		Conclusion string    `json:"conclusion"`
		StartedAt  time.Time `json:"startedAt"`
	} `json:"jobs"`
}

func recordsFromJobsJSON(sha string, runCreated time.Time, raw []byte) ([]CheckRunRecord, error) {
	var v jobsView
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	var out []CheckRunRecord
	for _, j := range v.Jobs {
		at := j.StartedAt
		if at.IsZero() {
			at = runCreated
		}
		out = append(out, CheckRunRecord{
			SHA: sha, Conclusion: j.Conclusion, JobName: j.Name, CreatedAt: at,
		})
	}
	return out, nil
}

type workflowRunRow struct {
	DatabaseID int       `json:"databaseId"`
	HeadSha    string    `json:"headSha"`
	CreatedAt  time.Time `json:"createdAt"`
}

// GetCheckRunHistory lists per-job conclusions for a named workflow over
// `days`. Job names embed the chainsaw suite (see intel.SuiteFromJobName).
func (c *Client) GetCheckRunHistory(workflowName string, days int) ([]CheckRunRecord, error) {
	if workflowName == "" {
		workflowName = "Conformance tests"
	}
	if days <= 0 {
		days = 14
	}
	out, err := run("run", "list", "-R", c.Repo, "--workflow", workflowName, "--limit", "40",
		"--json", "databaseId,headSha,createdAt")
	if err != nil {
		return nil, err
	}
	var runs []workflowRunRow
	if err := json.Unmarshal(out, &runs); err != nil {
		return nil, err
	}
	since := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	var all []CheckRunRecord
	for _, r := range runs {
		if r.CreatedAt.Before(since) {
			continue
		}
		jobsOut, err := run("run", "view", fmt.Sprint(r.DatabaseID), "-R", c.Repo, "--json", "jobs")
		if err != nil {
			return all, err
		}
		recs, err := recordsFromJobsJSON(r.HeadSha, r.CreatedAt, jobsOut)
		if err != nil {
			return all, err
		}
		all = append(all, recs...)
	}
	return all, nil
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
