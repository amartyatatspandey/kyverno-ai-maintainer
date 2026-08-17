package policy

import (
	"fmt"
	"path"
	"slices"
	"strings"
	"time"
)

const decisionTTL = 60 * time.Second

// Engine evaluates actions against the config. It is pure: no I/O, no LLM.
type Engine struct{ cfg *Config }

func NewEngine(cfg *Config) *Engine { return &Engine{cfg: cfg} }

// Evaluate runs the full rule order (POLICY_ENGINE.md). First DENY wins;
// ALLOW requires every rule to pass. Unknown action/workflow => DENY.
func (e *Engine) Evaluate(a Action, ctx Context) Decision {
	d := Decision{ExpiresAt: ctx.Now.Add(decisionTTL)}
	add := func(rule string, pass bool, reason string) bool {
		d.Rules = append(d.Rules, RuleResult{Rule: rule, Pass: pass, Reason: reason})
		return pass
	}

	// 1–2. global enable, workflow enable, kill switch
	if !add("assistant_enabled", e.cfg.Enabled, "enabled flag in config") {
		return d
	}
	wf, ok := e.cfg.Workflows[ctx.Workflow]
	if !add("workflow_known", ok, fmt.Sprintf("workflow %q declared in config", ctx.Workflow)) {
		return d
	}
	if !add("workflow_enabled", wf.Enabled, ctx.Workflow) {
		return d
	}
	if !add("kill_switch_off", !ctx.KillSwitch, "repo variable "+e.cfg.KillSwitch.RepoVariable) {
		return d
	}
	if labels := ctx.entityLabels(); slices.Contains(labels, e.cfg.KillSwitch.Label) || slices.Contains(labels, "hold") {
		add("no_hold_label", false, "entity carries hold/"+e.cfg.KillSwitch.Label)
		return d
	}
	add("no_hold_label", true, "no hold labels on entity")

	// 3. action allowed for workflow. Specialized comment_* actions consume
	// the workflow's "comment" github_op (github_ops lists the capability,
	// not every template id).
	allowedOps := e.cfg.GitHubOps[ctx.Workflow]
	op := githubOp(a.Type)
	opAllowed := slices.Contains(allowedOps, op) || a.Type == "run_scoped_tests" || a.Type == ActionRunPolicyLint
	if !add("action_allowed_for_workflow", opAllowed,
		fmt.Sprintf("%s ∈ %v", a.Type, allowedOps)) {
		return d
	}

	switch a.Type {
	case "merge_pr":
		if !e.mergeRules(&d, add, ctx) {
			return d
		}
	case "set_labels":
		if !e.labelRules(&d, add, a, ctx) {
			return d
		}
	case "comment":
		if !e.commentBudget(add, ctx) {
			return d
		}
	case ActionCommentDCOGuidance:
		if !evaluateDCOCheck(ctx, add) || !e.commentBudget(add, ctx) {
			return d
		}
	case ActionCommentWelcome:
		if !evaluateWelcomeBot(ctx, add) || !e.commentBudget(add, ctx) {
			return d
		}
	case ActionCommentReviewerSuggestion:
		if !evaluateReviewerSuggest(ctx, add) || !e.commentBudget(add, ctx) {
			return d
		}
	case ActionCommentDigest:
		if !evaluateMaintainerDigest(ctx, add) || !e.commentBudget(add, ctx) {
			return d
		}
	case ActionCommentFlakyReport:
		if !evaluateFlakyDetection(ctx, add) || !e.commentBudget(add, ctx) {
			return d
		}
	case ActionCommentDocsGap:
		if !evaluateDocsGapDetection(ctx, add) || !e.commentBudget(add, ctx) {
			return d
		}
	case ActionAnswerDiscussion:
		if !evaluateDiscussionQA(ctx, add, e.cfg) || !e.commentBudget(add, ctx) {
			return d
		}
	case ActionRunPolicyLint:
		if !evaluateRunPolicyLint(ctx, add) {
			return d
		}
		if !add("sandbox_budget", ctx.Counters.SandboxRunsToday < e.cfg.RateLimits.SandboxRunsPerDay,
			fmt.Sprintf("%d/%d today", ctx.Counters.SandboxRunsToday, e.cfg.RateLimits.SandboxRunsPerDay)) {
			return d
		}
	case "run_scoped_tests":
		if !add("sandbox_budget", ctx.Counters.SandboxRunsToday < e.cfg.RateLimits.SandboxRunsPerDay,
			fmt.Sprintf("%d/%d today", ctx.Counters.SandboxRunsToday, e.cfg.RateLimits.SandboxRunsPerDay)) {
			return d
		}
	default:
		add("action_known", false, "unknown action type "+a.Type)
		return d
	}

	d.Allowed = true
	if ctx.PR != nil {
		d.BoundSHA = ctx.PR.HeadSHA
	}
	return d
}

func githubOp(actionType string) string {
	switch actionType {
	case ActionCommentDCOGuidance, ActionCommentWelcome, ActionCommentReviewerSuggestion, ActionCommentDigest, ActionCommentFlakyReport, ActionCommentDocsGap, ActionAnswerDiscussion:
		return "comment"
	default:
		return actionType
	}
}

func (e *Engine) commentBudget(add func(string, bool, string) bool, ctx Context) bool {
	return add("comment_budget", ctx.Counters.CommentsTodayEntity < e.cfg.RateLimits.CommentsPerEntityPerDay,
		fmt.Sprintf("%d/%d today on entity", ctx.Counters.CommentsTodayEntity, e.cfg.RateLimits.CommentsPerEntityPerDay))
}

// evaluateDCOCheck authorizes comment_dco_guidance. SignedOff is not consulted
// — whether guidance is *needed* is a runtime content decision.
func evaluateDCOCheck(ctx Context, add func(string, bool, string) bool) bool {
	if !add("dco_workflow", ctx.Workflow == "dco_check", "comment_dco_guidance requires workflow dco_check") {
		return false
	}
	return add("commits_present", ctx.Commits != nil, "DCO check requires commit facts from git trailers")
}

// evaluateWelcomeBot authorizes comment_welcome. First-time-contributor
// detection is a runtime content decision, not a permission gate.
func evaluateWelcomeBot(ctx Context, add func(string, bool, string) bool) bool {
	return add("welcome_workflow", ctx.Workflow == "welcome_bot", "comment_welcome requires workflow welcome_bot")
}

// evaluateReviewerSuggest authorizes comment_reviewer_suggestion. Who to
// suggest is a runtime content decision, not a permission gate.
func evaluateReviewerSuggest(ctx Context, add func(string, bool, string) bool) bool {
	return add("reviewer_workflow", ctx.Workflow == "reviewer_suggest", "comment_reviewer_suggestion requires workflow reviewer_suggest")
}

// evaluateMaintainerDigest authorizes comment_digest against a synthetic
// target (e.g. digest/2026-W33). Rate limit uses CommentsTodayEntity — no
// new counter type. Content of the digest is a runtime decision.
func evaluateMaintainerDigest(ctx Context, add func(string, bool, string) bool) bool {
	return add("digest_workflow", ctx.Workflow == "maintainer_digest", "comment_digest requires workflow maintainer_digest")
}

func evaluateFlakyDetection(ctx Context, add func(string, bool, string) bool) bool {
	return add("flaky_workflow", ctx.Workflow == "flaky_detection", "comment_flaky_report requires workflow flaky_detection")
}

func evaluateDocsGapDetection(ctx Context, add func(string, bool, string) bool) bool {
	if !add("docs_gap_workflow", ctx.Workflow == "docs_gap_detection", "comment_docs_gap requires workflow docs_gap_detection") {
		return false
	}
	if !add("pr_context_present", ctx.PR != nil, "docs gap requires PR facts") {
		return false
	}
	// Structured ChangedFiles only — Context has no PR body field, so poisoned
	// prose cannot affect this rule (see TestDocsGapIgnoresPoisonedPRBody).
	return add("user_facing_changed_files", touchesDocsSurface(ctx.PR.ChangedFiles),
		"changed files touch labels.yml area/cli, area/api, or area/engine")
}

func evaluateDiscussionQA(ctx Context, add func(string, bool, string) bool, cfg *Config) bool {
	if !add("discussion_qa_workflow", ctx.Workflow == "discussion_qa", "answer_discussion requires workflow discussion_qa") {
		return false
	}
	if !add("discussion_present", ctx.Discussion != nil, "discussion Q&A requires discussion facts") {
		return false
	}
	minConf := cfg.DiscussionQA.MinConfidence
	if minConf <= 0 {
		minConf = 0.7
	}
	minRet := cfg.DiscussionQA.MinRetrievalScore
	if minRet <= 0 {
		minRet = 0.2
	}
	d := ctx.Discussion
	if !add("llm_confidence", d.LLMConfidence >= minConf,
		fmt.Sprintf("llm confidence %.2f ≥ %.2f", d.LLMConfidence, minConf)) {
		return false
	}
	// Retrieval score is computed from the local docs index, not the model.
	return add("retrieval_score", d.BestRetrievalScore >= minRet,
		fmt.Sprintf("best snippet score %.3f ≥ %.2f", d.BestRetrievalScore, minRet))
}

// docsGapGlobs matches intel.docsGapGlobs / labels.yml area/cli, area/api, area/engine.
var docsGapGlobs = []string{"cmd/cli/**", "pkg/cli/**", "api/**", "pkg/engine/**"}

func touchesDocsSurface(files []string) bool {
	for _, f := range files {
		for _, g := range docsGapGlobs {
			if globMatch(g, f) {
				return true
			}
		}
	}
	return false
}

// evaluateRunPolicyLint authorizes sandbox execution of kyverno apply/test.
// Whether the PR actually touches the policy library is a runtime filter
// (PolicyLibraryFiles): empty => the workflow is not invoked at all.
func evaluateRunPolicyLint(ctx Context, add func(string, bool, string) bool) bool {
	if !add("policy_lint_workflow", ctx.Workflow == "policy_lint", "run_policy_lint requires workflow policy_lint") {
		return false
	}
	return add("pr_context_present", ctx.PR != nil, "policy lint requires PR facts")
}

// PolicyLibraryPrefixes are in-repo paths that carry community/sample policies.
// charts/kyverno-policies is the Helm policy chart; test/cli/test and test/policy
// are the CLI kyverno-test fixtures (NOTES.md §4). The website policy library
// lives in a separate repo and is not present in this checkout.
var PolicyLibraryPrefixes = []string{
	"charts/kyverno-policies/",
	"test/cli/test/",
	"test/policy/",
}

// PolicyLibraryFiles returns the subset of changed files under the policy
// library prefixes. Empty means policy_lint structurally does not apply.
func PolicyLibraryFiles(changed []string) []string {
	var out []string
	for _, f := range changed {
		if !isYAML(f) {
			continue
		}
		for _, p := range PolicyLibraryPrefixes {
			if strings.HasPrefix(f, p) || f == strings.TrimSuffix(p, "/") {
				out = append(out, f)
				break
			}
		}
	}
	return out
}

func isYAML(f string) bool {
	return strings.HasSuffix(strings.ToLower(f), ".yaml") || strings.HasSuffix(strings.ToLower(f), ".yml")
}

func (e *Engine) mergeRules(d *Decision, add func(string, bool, string) bool, ctx Context) bool {
	pr := ctx.PR
	if !add("pr_context_present", pr != nil, "merge requires PR facts") {
		return false
	}
	am := e.cfg.AutoMerge
	if !add("author_allowlisted", slices.Contains(am.AllowedAuthors, pr.AuthorLogin) && pr.AuthorIsBot,
		fmt.Sprintf("author=%s bot=%v (API author type, not title heuristics)", pr.AuthorLogin, pr.AuthorIsBot)) {
		return false
	}
	if !add("update_type_allowed", slices.Contains(am.UpdateTypes, pr.UpdateType),
		fmt.Sprintf("update=%s ∈ %v", pr.UpdateType, am.UpdateTypes)) {
		return false
	}
	if !add("base_branch_allowed", slices.Contains(e.cfg.Branches.MergeTargetsAllowed, pr.BaseRef), "base="+pr.BaseRef) {
		return false
	}
	// 5. protected paths — evaluated BEFORE the changed-files allowlist and wins.
	if hit := firstMatch(pr.ChangedFiles, e.cfg.ProtectedPaths); hit != "" {
		add("no_protected_paths", false, "touches protected path: "+hit)
		return false
	}
	add("no_protected_paths", true, "no protected paths in diff")
	if bad := firstNotMatching(pr.ChangedFiles, am.ChangedFilesMustMatch); bad != "" {
		add("changed_files_allowlisted", false, "file outside allowlist: "+bad)
		return false
	}
	add("changed_files_allowlisted", true, fmt.Sprintf("%d files ⊆ allowed globs", len(pr.ChangedFiles)))
	if !add("checks_green", pr.ChecksGreen && !pr.ChecksPending, "all required checks green, none pending") {
		return false
	}
	if !add("mergeable", pr.Mergeable && !pr.IsDraft && pr.State == "OPEN", "mergeable, non-draft, open") {
		return false
	}
	if l := intersect(pr.Labels, am.DenyLabels); len(l) > 0 {
		add("no_deny_labels", false, "labels: "+strings.Join(l, ","))
		return false
	}
	add("no_deny_labels", true, "labels ∩ deny = ∅")
	if !add("merge_budget", ctx.Counters.MergesToday < e.cfg.RateLimits.MergesPerDay,
		fmt.Sprintf("%d/%d today", ctx.Counters.MergesToday, e.cfg.RateLimits.MergesPerDay)) {
		return false
	}
	return true
}

func (e *Engine) labelRules(d *Decision, add func(string, bool, string) bool, a Action, ctx Context) bool {
	adds, _ := a.Params["add"].([]string)
	removes, _ := a.Params["remove"].([]string)
	for _, l := range adds {
		if matchesAny(l, e.cfg.Labels.AssignableDenylist) {
			add("label_assignable", false, "label in denylist: "+l)
			return false
		}
	}
	add("label_assignable", true, "adds outside denylist")
	for _, l := range removes {
		if matchesAny(l, e.cfg.Labels.NeverRemove) {
			add("label_removable", false, "never_remove label: "+l)
			return false
		}
	}
	add("label_removable", true, "removes outside never_remove")
	return add("label_budget", ctx.Counters.LabelOpsTodayEntity < e.cfg.RateLimits.LabelOpsPerEntityPerDay,
		fmt.Sprintf("%d/%d today on entity", ctx.Counters.LabelOpsTodayEntity, e.cfg.RateLimits.LabelOpsPerEntityPerDay))
}

func (ctx Context) entityLabels() []string {
	if ctx.PR != nil {
		return ctx.PR.Labels
	}
	if ctx.Issue != nil {
		return ctx.Issue.Labels
	}
	return nil
}

// glob matching: supports ** via path.Match on segments + prefix fallback.
func globMatch(pattern, file string) bool {
	if pattern == file {
		return true
	}
	if strings.HasSuffix(pattern, "/**") {
		return strings.HasPrefix(file, strings.TrimSuffix(pattern, "**"))
	}
	if strings.HasPrefix(pattern, "**/") {
		base := strings.TrimPrefix(pattern, "**/")
		ok, _ := path.Match(base, path.Base(file))
		return ok || strings.HasSuffix(file, "/"+base)
	}
	ok, _ := path.Match(pattern, file)
	return ok
}

func matchesAny(s string, patterns []string) bool {
	for _, p := range patterns {
		if globMatch(p, s) {
			return true
		}
	}
	return false
}

func firstMatch(files, patterns []string) string {
	for _, f := range files {
		if matchesAny(f, patterns) {
			return f
		}
	}
	return ""
}

func firstNotMatching(files, patterns []string) string {
	for _, f := range files {
		if !matchesAny(f, patterns) {
			return f
		}
	}
	return ""
}

func intersect(a, b []string) []string {
	var out []string
	for _, x := range a {
		if slices.Contains(b, x) {
			out = append(out, x)
		}
	}
	return out
}
