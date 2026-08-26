package runtime

import (
	"fmt"
	"strings"

	"github.com/amartyatatspandey/kyverno-ai-maintainer/internal/intel"
	"github.com/amartyatatspandey/kyverno-ai-maintainer/internal/policy"
	"github.com/amartyatatspandey/kyverno-ai-maintainer/internal/sandbox"
)

// GitHub comment templates. Untrusted strings (titles, LLM text, log tails)
// pass through escapeParam (A2/I9) before interpolation — the model cannot
// break out of the template into a second GitHub action.

// escapeParam neutralizes markdown/HTML injection from untrusted text that
// reaches a comment template (RISKS A2 / injection case I9).
func escapeParam(s string, max int) string {
	s = strings.NewReplacer("<", "&lt;", ">", "&gt;", "@", "&#64;", "`", "'").Replace(s)
	if len(s) > max {
		s = s[:max] + "…[truncated]"
	}
	return s
}

// renderPRComment is the W1 maintainer-facing comment. The LLM summary is
// labeled advisory; the policy table is the decision. mode != autonomous
// holds an ALLOW so humans see eligibility without a merge.
func renderPRComment(runID string, pr *policy.PRFacts, summary string, sel intel.Selection,
	tests []sandbox.StageResult, d policy.Decision, mode string) string {
	var b strings.Builder
	w := func(f string, a ...any) { fmt.Fprintf(&b, f+"\n", a...) }

	verdict := "🚫 **NOT eligible for automated merge**"
	if d.Allowed {
		verdict = "✅ **Eligible for automated merge**"
		if mode != "autonomous" {
			verdict += fmt.Sprintf(" — held for human approval (mode: `%s`)", mode)
		}
	}
	w("### Kyverno AI Maintainer Assistant")
	w("")
	w("%s", verdict)
	w("")
	w("| | |")
	w("|---|---|")
	w("| Update type | `%s` (parsed from PR title deterministically) |", pr.UpdateType)
	w("| Changed files | %d |", len(pr.ChangedFiles))
	w("| Head SHA | `%s` |", short(pr.HeadSHA))
	w("")
	w("**Risk summary** _(from the language model — advisory only, not used in the decision)_:")
	w("> %s", escapeParam(summary, 1500))
	w("")
	w("**Scoped tests selected** _(deterministic: path map + import closure)_:")
	if len(sel.Suites) == 0 && len(sel.UnitPackages) == 0 {
		w("- none required for these paths")
	} else {
		w("- conformance suites: %s", codeList(sel.Suites))
		w("- unit packages: %d", len(sel.UnitPackages))
	}
	if sel.FullFallback {
		w("- ⚠️ unmapped paths present → full-suite fallback recommended: %s", codeList(sel.UnmappedFiles))
	}
	if len(tests) > 0 {
		w("")
		w("**Sandbox results:**")
		for _, t := range tests {
			mark := "✅"
			if !t.Passed {
				mark = "❌"
			}
			w("- %s `%s` (%s)", mark, t.Target, t.Duration)
		}
	}
	w("")
	w("**Policy decision** _(deterministic engine — this, not the model, authorizes actions)_:")
	w("```")
	for _, r := range d.Rules {
		mark := "✓"
		if !r.Pass {
			mark = "✗"
		}
		w("%s %-28s %s", mark, r.Rule, r.Reason)
	}
	w("```")
	w("")
	w("_Run `%s`. To stop the assistant: add the `ai-hold` label, or set repo variable `AI_MAINTAINER_PAUSED=true`._", runID)
	return b.String()
}

// renderTriageComment never claims the model applied a privileged label —
// policy already stripped the denylist; the template just says so.
func renderTriageComment(runID string, c classification, labelDecision policy.Decision) string {
	var b strings.Builder
	w := func(f string, a ...any) { fmt.Fprintf(&b, f+"\n", a...) }
	w("### Kyverno AI Maintainer Assistant — triage")
	w("")
	w("**Proposed classification** _(model output, advisory)_: `%s`", escapeParam(c.Type, 40))
	if len(c.MissingInfo) > 0 {
		w("")
		w("**Possibly missing information:**")
		for _, m := range c.MissingInfo {
			w("- %s", escapeParam(m, 200))
		}
	}
	if c.Rationale != "" {
		w("")
		w("> %s", escapeParam(c.Rationale, 800))
	}
	w("")
	w("**Labels applied**: %v _(filtered by policy allowlist; privileged labels such as `security` are structurally unavailable to the assistant)_", labelDecision.Allowed)
	w("")
	w("_The `triage` label is only removed by a human. Run `%s`._", runID)
	return b.String()
}

// renderDCOGuidance is comment-only. Unsigned SHAs come from git trailers
// (GetCommitFacts), never from the PR body, so a hostile description cannot
// fake a Signed-off-by.
func renderDCOGuidance(runID string, pr *policy.PRFacts, unsigned []string) string {
	var b strings.Builder
	w := func(f string, a ...any) { fmt.Fprintf(&b, f+"\n", a...) }
	w("### Kyverno AI Maintainer Assistant — DCO")
	w("")
	w("This pull request has commit(s) missing a `Signed-off-by:` trailer matching the commit author (Developer Certificate of Origin).")
	if pr != nil && pr.Title != "" {
		w("")
		w("PR: #%d — %s", pr.Number, escapeParam(pr.Title, 200))
	}
	w("")
	w("**Unsigned commits:**")
	for _, sha := range unsigned {
		w("- `%s`", sha)
	}
	w("")
	w("Please sign off and force-push:")
	w("- single commit: `git commit --amend -s`")
	w("- multiple commits: `git rebase --signoff`")
	w("")
	w("Then `git push --force-with-lease`.")
	w("")
	w("_Run `%s`. To stop the assistant: add the `ai-hold` label, or set repo variable `AI_MAINTAINER_PAUSED=true`._", runID)
	return b.String()
}

// renderWelcome is a first-timer pointer, not a merge. Author login is
// escaped so a crafted username cannot inject markdown (A2).
func renderWelcome(runID string, pr *policy.PRFacts) string {
	var b strings.Builder
	w := func(f string, a ...any) { fmt.Fprintf(&b, f+"\n", a...) }
	who := "there"
	if pr != nil && pr.AuthorLogin != "" {
		who = escapeParam(pr.AuthorLogin, 80)
	}
	w("### Kyverno AI Maintainer Assistant — welcome")
	w("")
	w("Hi %s — thanks for your first contribution to Kyverno!", who)
	w("")
	w("A few pointers that help reviews go smoothly:")
	w("- [CONTRIBUTING.md](CONTRIBUTING.md) — contribution process and DCO (`git commit -s`)")
	w("- [docs/dev/](docs/dev/) — developer docs, local build, and test layout")
	w("")
	w("_Run `%s`. To stop the assistant: add the `ai-hold` label, or set repo variable `AI_MAINTAINER_PAUSED=true`._", runID)
	return b.String()
}

// renderReviewerSuggestion never files a GitHub review request — that
// github_op is not in this workflow. The list is CODEOWNERS/git-log, not LLM.
func renderReviewerSuggestion(runID string, pr *policy.PRFacts, reviewers []intel.Reviewer) string {
	var b strings.Builder
	w := func(f string, a ...any) { fmt.Fprintf(&b, f+"\n", a...) }
	w("### Kyverno AI Maintainer Assistant — suggested reviewers")
	w("")
	w("These are **suggestions only**. The assistant never files a GitHub review request — a maintainer can act on this list manually.")
	if pr != nil && pr.Title != "" {
		w("")
		w("PR: #%d — %s", pr.Number, escapeParam(pr.Title, 200))
	}
	w("")
	for _, r := range reviewers {
		w("- %s — %s", escapeParam(r.Name, 80), escapeParam(r.Reason, 200))
	}
	w("")
	w("_Run `%s`. To stop the assistant: add the `ai-hold` label, or set repo variable `AI_MAINTAINER_PAUSED=true`._", runID)
	return b.String()
}

// renderPolicyLintResult posts sandbox observations. Log tails are escaped
// and truncated so a policy YAML comment cannot break out of the template (I9).
func renderPolicyLintResult(runID string, pr *policy.PRFacts, results []sandbox.LintResult) string {
	var b strings.Builder
	w := func(f string, a ...any) { fmt.Fprintf(&b, f+"\n", a...) }
	w("### Kyverno AI Maintainer Assistant — policy lint")
	w("")
	w("Ran `kyverno apply` / `kyverno test` in the credential-free sandbox against policy-library YAML.")
	if pr != nil && pr.Title != "" {
		w("")
		w("PR: #%d — %s", pr.Number, escapeParam(pr.Title, 200))
	}
	w("")
	if len(results) == 0 {
		w("_no policy files linted_")
	} else {
		for _, t := range results {
			mark := "✅ pass"
			if !t.Passed {
				mark = "❌ fail"
			}
			out := strings.Join(strings.Fields(t.LogTail), " ")
			if out == "" {
				w("- %s `%s`", mark, escapeParam(t.Target, 120))
				continue
			}
			w("- %s `%s` — %s", mark, escapeParam(t.Target, 120), escapeParam(out, 300))
		}
	}
	w("")
	w("_Run `%s`. To stop the assistant: add the `ai-hold` label, or set repo variable `AI_MAINTAINER_PAUSED=true`._", runID)
	return b.String()
}

// renderDocsGap flags a missing website-repo pointer. reason is a structured
// area/path hit from DetectDocsGap, not model prose.
func renderDocsGap(runID string, pr *policy.PRFacts, reason string) string {
	var b strings.Builder
	w := func(f string, a ...any) { fmt.Fprintf(&b, f+"\n", a...) }
	w("### Kyverno AI Maintainer Assistant — docs gap")
	w("")
	w("This PR looks like a user-facing change (%s) without a linked documentation issue/PR on the website repo.", escapeParam(reason, 300))
	if pr != nil && pr.Title != "" {
		w("")
		w("PR: #%d — %s", pr.Number, escapeParam(pr.Title, 200))
	}
	w("")
	w("Please open (or link) a corresponding docs issue/PR on [kyverno/website](https://github.com/kyverno/website).")
	w("See [CONTRIBUTING.md](CONTRIBUTING.md) (docs-PR requirement) and [AGENTS.md](AGENTS.md) Pull Request Guidelines: new/changed functionality needs a corresponding documentation issue/PR on the website repo.")
	w("")
	w("_Run `%s`. To stop the assistant: add the `ai-hold` label, or set repo variable `AI_MAINTAINER_PAUSED=true`._", runID)
	return b.String()
}

// renderDiscussionAnswer posts a grounded draft. The answer is still escaped:
// a discussion body that jailbreaks the model (I4) must not inject markdown.
func renderDiscussionAnswer(runID, answer string, snips []intel.DocSnippet) string {
	var b strings.Builder
	w := func(f string, a ...any) { fmt.Fprintf(&b, f+"\n", a...) }
	w("### Kyverno AI Maintainer Assistant — discussion answer")
	w("")
	w("%s", escapeParam(answer, 4000))
	w("")
	var files []string
	for _, s := range snips {
		files = append(files, "`"+escapeParam(s.Path, 120)+"`")
	}
	if len(files) == 0 {
		w("_grounded in: _none_ (no local doc snippets)._")
	} else {
		w("_grounded in: %s._", strings.Join(files, ", "))
	}
	w("")
	w("_Run `%s`. To stop the assistant: add the `ai-hold` label, or set repo variable `AI_MAINTAINER_PAUSED=true`._", runID)
	return b.String()
}

// renderDiscussionEscalation is the dual-gate miss path: no local snippet
// overlap or low model confidence → human, never a guessed answer.
func renderDiscussionEscalation(runID string) string {
	var b strings.Builder
	w := func(f string, a ...any) { fmt.Fprintf(&b, f+"\n", a...) }
	w("### Kyverno AI Maintainer Assistant — discussion escalation")
	w("")
	w("I don't have a confident answer — flagging for a maintainer.")
	w("The assistant only answers from local documentation (keyword overlap + a grounded draft); it will not guess.")
	w("")
	w("_Run `%s`. To stop the assistant: add the `ai-hold` label, or set repo variable `AI_MAINTAINER_PAUSED=true`._", runID)
	return b.String()
}

// renderFlakyReport includes a YAML snippet a human can paste. The assistant
// never writes quarantined-tests itself (T2).
func renderFlakyReport(runID string, cands []intel.FlakyCandidate) string {
	var b strings.Builder
	w := func(f string, a ...any) { fmt.Fprintf(&b, f+"\n", a...) }
	w("### Kyverno AI Maintainer Assistant — flaky suite candidates")
	w("")
	w("Advisory only. The assistant **never quarantines a test** (that would edit skip-lists / workflow inputs, which stay in human hands).")
	w("Paste the snippet below yourself if you agree.")
	w("")
	var suites []string
	for _, c := range cands {
		suites = append(suites, escapeParam(c.Suite, 80))
		w("- `%s` — %.0f%% failure rate (%d runs); recent failing SHAs: %s",
			escapeParam(c.Suite, 80), c.FailureRate*100, c.TotalRuns,
			escapeParam(strings.Join(shortSHAs(c.RecentFailureSHAs), ", "), 200))
	}
	w("")
	w("**Suggested `chainsaw-tests` quarantine snippet** _(human paste only)_:")
	w("")
	w("```yaml")
	w("# .github/actions/tests/conformance/run inputs — do not apply automatically")
	w("with:")
	w("  chainsaw-tests: ''")
	w("  quarantined-tests: '%s'", strings.Join(suites, ","))
	w("```")
	w("")
	w("_Run `%s`. To stop the assistant: add the `ai-hold` label, or set repo variable `AI_MAINTAINER_PAUSED=true`._", runID)
	return b.String()
}

func shortSHAs(shas []string) []string {
	out := make([]string, 0, len(shas))
	for _, s := range shas {
		out = append(out, short(s))
	}
	return out
}

// renderDigest is numbers-only. No LLM insertion point, so an issue body on
// the digest ticket cannot inject a second action into this comment.
func renderDigest(runID string, snap digestSnapshot) string {
	var b strings.Builder
	w := func(f string, a ...any) { fmt.Fprintf(&b, f+"\n", a...) }
	w("### Kyverno AI Maintainer Assistant — weekly digest (%s)", escapeParam(snap.Week, 16))
	w("")
	w("Repo dashboard. Computed from GitHub API facts; no model involved.")
	w("")
	w("| Metric | Value |")
	w("|---|---|")
	w("| Open PRs | %d |", snap.OpenPRs)
	w("| Median PR age | %.1f days |", snap.MedianAgeDays)
	w("| PRs with CI not green | %d |", snap.ChecksNotGreen)
	w("| Triage backlog | %d |", snap.TriageBacklog)
	w("")
	w("**Top workflow failure rates** _(last %d days, workflow-level)_:", digestFailureRateDays)
	w("")
	if len(snap.TopFailures) == 0 {
		w("_no recent workflow runs_")
	} else {
		w("| Workflow | Failure rate |")
		w("|---|---|")
		for _, f := range snap.TopFailures {
			w("| %s | %.0f%% |", escapeParam(f.Name, 80), f.Rate*100)
		}
	}
	w("")
	w("_Schedulable via cron, e.g. `0 9 * * 1 assistant digest`. Run `%s`._", runID)
	return b.String()
}

func codeList(xs []string) string {
	if len(xs) == 0 {
		return "_none_"
	}
	out := make([]string, 0, len(xs))
	for _, x := range xs {
		out = append(out, "`"+escapeParam(x, 80)+"`")
	}
	return strings.Join(out, ", ")
}

func short(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}

// renderReproRejected does not echo the rejected YAML (A2: hostile comments
// in the manifest must not round-trip into a GitHub comment).
func renderReproRejected(runID, reason string) string {
	var b strings.Builder
	w := func(f string, a ...any) { fmt.Fprintf(&b, f+"\n", a...) }
	w("### Kyverno AI Maintainer Assistant — repro")
	w("")
	w("Automated reproduction **did not run**. The issue body did not pass the YAML allowlist gate.")
	w("")
	w("**Reason:** %s", escapeParam(reason, 400))
	w("")
	w("Fix the bug-report template fields (`Kyverno Version` / `Kyverno CLI Version`, `Steps to reproduce` with two ```yaml fences, `Expected behavior`) and ensure the manifests contain no host-access or exec-style fields.")
	w("")
	w("_The rejected YAML is not echoed here._")
	w("")
	w("_Run `%s`. To stop the assistant: add the `ai-hold` label, or set repo variable `AI_MAINTAINER_PAUSED=true`._", runID)
	return b.String()
}

// renderReproResult is advisory evidence, not a "bug confirmed" verdict —
// env quirks (T4) are why the comment names the pinned versions.
func renderReproResult(runID, version, expected string, res *sandbox.ReproResult) string {
	var b strings.Builder
	w := func(f string, a ...any) { fmt.Fprintf(&b, f+"\n", a...) }
	w("### Kyverno AI Maintainer Assistant — repro")
	w("")
	if res != nil && res.Success {
		w("Sandbox observation **matched** the expected behavior (advisory evidence, not a verdict).")
	} else {
		w("Sandbox observation **did not match** the expected behavior, or the run did not complete cleanly (advisory evidence, not a verdict).")
	}
	w("")
	w("| | |")
	w("|---|---|")
	w("| Kyverno version | `%s` (pinned allowlist) |", escapeParam(version, 20))
	actual := ""
	logs := ""
	if res != nil {
		actual = res.ActualBehavior
		logs = res.Logs
	}
	w("| Actual | `%s` |", escapeParam(actual, 40))
	w("| Expected | %s |", escapeParam(expected, 200))
	w("")
	if logs != "" {
		w("**Logs** _(truncated)_:")
		w("```")
		w("%s", escapeParam(logs, 1500))
		w("```")
		w("")
	}
	w("_Run `%s`. To stop the assistant: add the `ai-hold` label, or set repo variable `AI_MAINTAINER_PAUSED=true`._", runID)
	return b.String()
}
