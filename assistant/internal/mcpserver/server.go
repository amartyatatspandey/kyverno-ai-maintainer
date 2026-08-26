// Package mcpserver serves the existing tool surface over the MCP protocol.
//
// It lives in the assistant binary (`assistant mcp`, stdio transport) rather
// than a separate cmd/mcpserver: MCP clients already spawn a subprocess, and
// one binary keeps policy config, GitHub credentials, and audit wiring
// identical to `assistant run`. This is a new transport, not a new authority
// — every mutating tool calls policy.Engine.Evaluate and then the same
// ghx methods the CLI uses (which fail closed without an ALLOW).
package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/amartyatatspandey/kyverno-ai-maintainer/internal/audit"
	"github.com/amartyatatspandey/kyverno-ai-maintainer/internal/ghx"
	"github.com/amartyatatspandey/kyverno-ai-maintainer/internal/intel"
	"github.com/amartyatatspandey/kyverno-ai-maintainer/internal/policy"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// GitHub is the executor surface the MCP tools call. *ghx.Client implements
// it; tests inject a fake. Mutating methods already require a Decision.
type GitHub interface {
	GetPRFacts(number int) (*policy.PRFacts, string, error)
	GetIssue(number int) (*ghx.IssueView, error)
	GetDiff(number int, maxBytes int) (string, error)
	UpsertComment(d *policy.Decision, kind string, number int, marker, body string) (string, error)
	SetLabels(d *policy.Decision, number int, add, remove []string) (string, error)
	MergePR(d *policy.Decision, number int, method string) (string, error)
	KillSwitchActive(variable string) bool
}

// Host is the backend shared with the CLI: same engine, same GitHub client,
// same audit directory.
type Host struct {
	Engine   *policy.Engine
	GH       GitHub
	Map      *intel.TestMap
	Cfg      *policy.Config
	Repo     string
	RepoDir  string
	AuditDir string
	Classify func(title string) string // runtime.ClassifyUpdateType in production
}

// Tool names match MCP_TOOLS.md / the in-process surface.
const (
	ToolGetPR            = "github.get_pull_request"
	ToolGetDiff          = "github.get_diff"
	ToolGetIssue         = "github.get_issue"
	ToolGetAffectedTests = "kyverno.get_affected_tests"
	ToolComment          = "github.comment"
	ToolSetLabels        = "github.set_labels"
	ToolMergePR          = "github.merge_pr"
)

// New builds an MCP server with the read-only and mutating tools that have
// real methods in ghx/intel. Mutating handlers evaluate policy first.
func New(h *Host) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "kyverno-ai-maintainer", Version: "v0.1.0"}, nil)
	mcp.AddTool(s, &mcp.Tool{Name: ToolGetPR, Description: "PR facts (author, SHA, labels, checks, files). Read-only."}, h.getPR)
	mcp.AddTool(s, &mcp.Tool{Name: ToolGetDiff, Description: "Size-capped unified diff for a PR. Read-only."}, h.getDiff)
	mcp.AddTool(s, &mcp.Tool{Name: ToolGetIssue, Description: "Issue title/body/labels. Read-only; body is untrusted."}, h.getIssue)
	mcp.AddTool(s, &mcp.Tool{Name: ToolGetAffectedTests, Description: "Deterministic path→suite + unit-package selection. Read-only."}, h.getAffectedTests)
	mcp.AddTool(s, &mcp.Tool{Name: ToolComment, Description: "Upsert a comment. Requires policy ALLOW."}, h.comment)
	mcp.AddTool(s, &mcp.Tool{Name: ToolSetLabels, Description: "Add/remove labels. Requires policy ALLOW."}, h.setLabels)
	mcp.AddTool(s, &mcp.Tool{Name: ToolMergePR, Description: "Squash-merge a PR. Requires the full W1 policy gate."}, h.mergePR)
	return s
}

// Run serves MCP over stdin/stdout until the client disconnects.
func Run(ctx context.Context, h *Host) error {
	return New(h).Run(ctx, &mcp.StdioTransport{})
}

type numberIn struct {
	Number int `json:"number" jsonschema:"pull request or issue number"`
}

type diffIn struct {
	Number   int `json:"number" jsonschema:"pull request number"`
	MaxBytes int `json:"max_bytes,omitempty" jsonschema:"diff byte cap; 0 means 32KiB"`
}

type filesIn struct {
	ChangedFiles []string `json:"changed_files" jsonschema:"changed file paths from the PR diff"`
}

type commentIn struct {
	Entity string `json:"entity,omitempty" jsonschema:"pr or issue"`
	Number int    `json:"number" jsonschema:"entity number"`
	Body   string `json:"body" jsonschema:"comment body; host still policy-gates the action"`
}

type labelsIn struct {
	Number   int      `json:"number" jsonschema:"PR or issue number"`
	Add      []string `json:"add,omitempty" jsonschema:"labels to add"`
	Remove   []string `json:"remove,omitempty" jsonschema:"labels to remove"`
	Workflow string   `json:"workflow,omitempty" jsonschema:"dependency_prs or issue_triage; default dependency_prs"`
}

type mergeIn struct {
	Number          int    `json:"number" jsonschema:"pull request number"`
	ExpectedHeadSHA string `json:"expected_head_sha,omitempty" jsonschema:"optional SHA; must match the bound decision SHA if set"`
}

func (h *Host) getPR(ctx context.Context, _ *mcp.CallToolRequest, in numberIn) (*mcp.CallToolResult, any, error) {
	log := h.beginAudit("pr", in.Number, ToolGetPR)
	defer h.endAudit(log, "completed")
	h.toolCalled(log, ToolGetPR, in.Number)
	pr, _, err := h.GH.GetPRFacts(in.Number)
	if err != nil {
		return nil, nil, err
	}
	return textJSON(pr)
}

func (h *Host) getDiff(ctx context.Context, _ *mcp.CallToolRequest, in diffIn) (*mcp.CallToolResult, any, error) {
	log := h.beginAudit("pr", in.Number, ToolGetDiff)
	defer h.endAudit(log, "completed")
	h.toolCalled(log, ToolGetDiff, in.Number)
	diff, err := h.GH.GetDiff(in.Number, in.MaxBytes)
	if err != nil {
		return nil, nil, err
	}
	return textJSON(map[string]any{"number": in.Number, "diff": diff})
}

func (h *Host) getIssue(ctx context.Context, _ *mcp.CallToolRequest, in numberIn) (*mcp.CallToolResult, any, error) {
	log := h.beginAudit("issue", in.Number, ToolGetIssue)
	defer h.endAudit(log, "completed")
	h.toolCalled(log, ToolGetIssue, in.Number)
	iss, err := h.GH.GetIssue(in.Number)
	if err != nil {
		return nil, nil, err
	}
	return textJSON(iss)
}

func (h *Host) getAffectedTests(ctx context.Context, _ *mcp.CallToolRequest, in filesIn) (*mcp.CallToolResult, any, error) {
	log := h.beginAudit("select", 0, ToolGetAffectedTests)
	defer h.endAudit(log, "completed")
	h.toolCalled(log, ToolGetAffectedTests, 0)
	if h.Map == nil {
		return nil, nil, fmt.Errorf("test map not loaded")
	}
	sel := intel.Select(h.Map, in.ChangedFiles, h.RepoDir)
	return textJSON(sel)
}

// comment evaluates, then upserts. The MCP client supplies the body; policy
// still gates the *action*. Body is not a policy input (no free-text fields).
func (h *Host) comment(ctx context.Context, _ *mcp.CallToolRequest, in commentIn) (*mcp.CallToolResult, any, error) {
	log := h.beginAudit(in.Entity, in.Number, ToolComment)
	defer h.endAudit(log, "completed")
	kind := in.Entity
	if kind == "" {
		kind = "pr"
	}
	pctx, err := h.entityContext(kind, in.Number)
	if err != nil {
		return nil, nil, err
	}
	d := h.Engine.Evaluate(policy.Action{Type: "comment", Target: fmt.Sprintf("%s/%d", kind, in.Number)}, pctx)
	h.logDecision(log, "comment", d)
	if !d.Allowed {
		return nil, nil, fmt.Errorf("policy DENY: %s", d.DenyReason())
	}
	res, err := h.GH.UpsertComment(&d, kind, in.Number, "ai-maintainer:mcp", in.Body)
	h.logAction(log, "comment", res, err)
	if err != nil {
		return nil, nil, err
	}
	return textJSON(map[string]any{"result": res})
}

// setLabels evaluates with the add/remove names in Params so the denylist
// can strip privileged labels before ghx.SetLabels runs.
func (h *Host) setLabels(ctx context.Context, _ *mcp.CallToolRequest, in labelsIn) (*mcp.CallToolResult, any, error) {
	log := h.beginAudit("pr", in.Number, ToolSetLabels)
	defer h.endAudit(log, "completed")
	wf := in.Workflow
	if wf == "" {
		wf = "dependency_prs"
	}
	pctx, err := h.workflowContext(wf, in.Number)
	if err != nil {
		return nil, nil, err
	}
	d := h.Engine.Evaluate(policy.Action{
		Type: "set_labels", Target: fmt.Sprintf("pr/%d", in.Number),
		Params: map[string]any{"add": in.Add, "remove": in.Remove},
	}, pctx)
	h.logDecision(log, "set_labels", d)
	if !d.Allowed {
		return nil, nil, fmt.Errorf("policy DENY: %s", d.DenyReason())
	}
	res, err := h.GH.SetLabels(&d, in.Number, in.Add, in.Remove)
	h.logAction(log, "set_labels", res, err)
	if err != nil {
		return nil, nil, err
	}
	return textJSON(map[string]any{"result": res})
}

// mergePR runs the full W1 gate (same mergeRules as `assistant run --pr`).
// expected_head_sha is an extra TOCTOU belt on top of Decision.BoundSHA (G2).
func (h *Host) mergePR(ctx context.Context, _ *mcp.CallToolRequest, in mergeIn) (*mcp.CallToolResult, any, error) {
	log := h.beginAudit("pr", in.Number, ToolMergePR)
	defer h.endAudit(log, "completed")
	pctx, err := h.workflowContext("dependency_prs", in.Number)
	if err != nil {
		return nil, nil, err
	}
	d := h.Engine.Evaluate(policy.Action{Type: "merge_pr", Target: fmt.Sprintf("pr/%d", in.Number)}, pctx)
	h.logDecision(log, "merge_pr", d)
	if !d.Allowed {
		return nil, nil, fmt.Errorf("policy DENY: %s", d.DenyReason())
	}
	if in.ExpectedHeadSHA != "" && d.BoundSHA != "" && in.ExpectedHeadSHA != d.BoundSHA {
		return nil, nil, fmt.Errorf("expected_head_sha %s != bound SHA %s", in.ExpectedHeadSHA, d.BoundSHA)
	}
	method := "squash"
	if h.Cfg != nil && h.Cfg.AutoMerge.Method != "" {
		method = h.Cfg.AutoMerge.Method
	}
	res, err := h.GH.MergePR(&d, in.Number, method)
	h.logAction(log, "merge_pr", res, err)
	if err != nil {
		return nil, nil, err
	}
	return textJSON(map[string]any{"result": res, "bound_sha": d.BoundSHA})
}

// entityContext is a thinner Context than the CLI run loop: MCP comment on
// an issue does not fetch labels. Hold/ai-hold therefore cannot fire here.
// TODO(reviewer): fetch GetIssue labels (and kill switch) so MCP comments
// on a held issue fail closed the same way `assistant run --issue` does.
func (h *Host) entityContext(kind string, number int) (policy.Context, error) {
	if kind == "issue" {
		return policy.Context{
			Workflow: "issue_triage", Repo: h.Repo,
			Issue: &policy.IssueFacts{Number: number, State: "OPEN"},
			Now:   time.Now(),
		}, nil
	}
	return h.workflowContext("dependency_prs", number)
}

// workflowContext fetch-fresh PR facts and classifies the title the same
// way the CLI does (ClassifyUpdateType, never the body).
func (h *Host) workflowContext(workflow string, number int) (policy.Context, error) {
	pr, _, err := h.GH.GetPRFacts(number)
	if err != nil {
		return policy.Context{}, err
	}
	if h.Classify != nil {
		pr.UpdateType = h.Classify(pr.Title)
	}
	kill := false
	if h.Cfg != nil {
		kill = h.GH.KillSwitchActive(h.Cfg.KillSwitch.RepoVariable)
	}
	return policy.Context{
		Workflow: workflow, Repo: h.Repo, PR: pr,
		KillSwitch: kill, Now: time.Now(),
	}, nil
}

// beginAudit is best-effort: a disk error returns nil so the tool still
// evaluates policy. Missing audit is a POC honesty gap (AUDIT.md hash-chain
// is the production follow-up), not a second authority.
func (h *Host) beginAudit(kind string, number int, tool string) *audit.Log {
	if h.AuditDir == "" {
		return nil
	}
	entity := kind
	if number > 0 {
		entity = fmt.Sprintf("%s%d", kind, number)
	}
	log, err := audit.Start(h.AuditDir, audit.NewRunID(entity), map[string]any{
		"trigger": "mcp", "tool": tool, "repo": h.Repo,
		"entity": fmt.Sprintf("%s/%d", kind, number),
	})
	if err != nil {
		return nil
	}
	return log
}

func (h *Host) endAudit(log *audit.Log, outcome string) {
	if log != nil {
		log.Finish(outcome, nil)
	}
}

func (h *Host) toolCalled(log *audit.Log, tool string, number int) {
	if log == nil {
		return
	}
	log.Emit("tool_called", map[string]any{"tool": tool, "args": number, "read_only": true})
}

func (h *Host) logDecision(log *audit.Log, action string, d policy.Decision) {
	if log == nil {
		return
	}
	rules := make([]map[string]any, 0, len(d.Rules))
	for _, ru := range d.Rules {
		rules = append(rules, map[string]any{"rule": ru.Rule, "pass": ru.Pass, "reason": ru.Reason})
	}
	log.Emit("policy_decision", map[string]any{
		"action": action, "allowed": d.Allowed, "rules": rules,
		"bound_sha": d.BoundSHA, "expires_at": d.ExpiresAt,
	})
}

func (h *Host) logAction(log *audit.Log, action string, result any, err error) {
	if log == nil {
		return
	}
	if err != nil {
		log.Emit("action_error", map[string]any{"action": action, "error": err.Error()})
		return
	}
	log.Emit("action_executed", map[string]any{"action": action, "result": result})
}

func textJSON(v any) (*mcp.CallToolResult, any, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(b)}},
	}, v, nil
}
