package runtime

import (
	"fmt"
	"strings"

	"github.com/amartyatatspandey/kyverno-ai-maintainer/internal/intel"
	"github.com/amartyatatspandey/kyverno-ai-maintainer/internal/policy"
	"github.com/amartyatatspandey/kyverno-ai-maintainer/internal/sandbox"
)

// escapeParam neutralizes markdown/HTML injection from untrusted text that
// reaches a comment template (RISKS A2 / injection case I9).
func escapeParam(s string, max int) string {
	s = strings.NewReplacer("<", "&lt;", ">", "&gt;", "@", "&#64;", "`", "'").Replace(s)
	if len(s) > max {
		s = s[:max] + "…[truncated]"
	}
	return s
}

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
