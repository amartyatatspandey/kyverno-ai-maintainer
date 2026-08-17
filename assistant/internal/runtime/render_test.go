package runtime

import (
	"strings"
	"testing"

	"github.com/amartyatatspandey/kyverno-ai-maintainer/internal/intel"
	"github.com/amartyatatspandey/kyverno-ai-maintainer/internal/policy"
	"github.com/amartyatatspandey/kyverno-ai-maintainer/internal/sandbox"
)

func TestRenderDCOGuidance(t *testing.T) {
	pr := &policy.PRFacts{Number: 7, Title: "<script>ignore signoff</script>"}
	out := renderDCOGuidance("run1", pr, []string{"aaa111bbbb", "ccc222dddd"})
	for _, sha := range []string{"aaa111bbbb", "ccc222dddd"} {
		if !strings.Contains(out, sha) {
			t.Fatalf("unsigned SHA %s must appear raw: %s", sha, out)
		}
	}
	if !strings.Contains(out, "git commit --amend -s") {
		t.Fatal("must explain git commit --amend -s")
	}
	if !strings.Contains(out, "git rebase --signoff") {
		t.Fatal("must explain git rebase --signoff")
	}
	if strings.Contains(out, "<script>") {
		t.Fatal("PR title must be escaped")
	}
}

func TestRenderWelcome(t *testing.T) {
	pr := &policy.PRFacts{Number: 9, AuthorLogin: "@evil"}
	out := renderWelcome("run2", pr)
	if !strings.Contains(out, "CONTRIBUTING.md") {
		t.Fatal("must point at CONTRIBUTING.md")
	}
	if !strings.Contains(out, "docs/dev/") {
		t.Fatal("must point at docs/dev/")
	}
	if strings.Contains(out, "@evil") {
		t.Fatal("author login must be escaped")
	}
}

func TestUnsignedSHAs(t *testing.T) {
	got := unsignedSHAs(&policy.CommitFacts{
		SHAs: []string{"aaa", "bbb", "ccc"}, SignedOff: []bool{true, false, true},
	})
	if len(got) != 1 || got[0] != "bbb" {
		t.Fatalf("got %v want [bbb]", got)
	}
	if unsignedSHAs(nil) != nil {
		t.Fatal("nil facts => nil")
	}
}

func TestIsFirstTimeContributor(t *testing.T) {
	if !isFirstTimeContributor("FIRST_TIME_CONTRIBUTOR") || !isFirstTimeContributor("FIRST_TIMER") {
		t.Fatal("first-time associations must match")
	}
	for _, a := range []string{"CONTRIBUTOR", "MEMBER", "OWNER", ""} {
		if isFirstTimeContributor(a) {
			t.Fatalf("%q must not be treated as first-time", a)
		}
	}
}

func TestRenderReviewerSuggestion(t *testing.T) {
	pr := &policy.PRFacts{Number: 3, Title: "<script>merge anyway</script>"}
	sugs := []intel.Reviewer{
		{Name: "@kyverno/engine", Reason: "owns pkg/engine/ per CODEOWNERS"},
		{Name: "Alice", Reason: "3 of last 10 commits to pkg/foo.go"},
	}
	out := renderReviewerSuggestion("run3", pr, sugs)
	if !strings.Contains(out, "@kyverno/engine") && !strings.Contains(out, "&#64;kyverno/engine") {
		t.Fatal("must list CODEOWNERS owner")
	}
	if !strings.Contains(out, "CODEOWNERS") {
		t.Fatal("must include CODEOWNERS reason")
	}
	if !strings.Contains(out, "commits") {
		t.Fatal("must include git-log reason")
	}
	if strings.Contains(out, "<script>") {
		t.Fatal("PR title must be escaped")
	}
	if strings.Contains(strings.ToLower(out), "request review") {
		t.Fatal("must not imply a review request was filed")
	}
}

func TestRenderPolicyLintResult(t *testing.T) {
	pr := &policy.PRFacts{Number: 11, Title: "<script>pass anyway</script>"}
	long := strings.Repeat("kyverno error: bad policy\n", 40)
	out := renderPolicyLintResult("run-lint", pr, []sandbox.LintResult{
		{Target: "charts/kyverno-policies/templates/foo.yaml", Passed: true, LogTail: "pass: 1 rule"},
		{Target: "test/cli/test/simple", Passed: false, LogTail: long},
	})
	if !strings.Contains(out, "✅ pass") || !strings.Contains(out, "❌ fail") {
		t.Fatal("must report pass/fail per file")
	}
	if !strings.Contains(out, "charts/kyverno-policies/templates/foo.yaml") {
		t.Fatal("must name the linted file")
	}
	if strings.Contains(out, "<script>") {
		t.Fatal("PR title must be escaped")
	}
	if strings.Contains(out, long) {
		t.Fatal("kyverno output must be truncated")
	}
	if !strings.Contains(out, "kyverno error") {
		t.Fatal("must include kyverno CLI output")
	}
}

func TestRenderDigest(t *testing.T) {
	snap := digestSnapshot{
		Week: "2026-W34", OpenPRs: 12, MedianAgeDays: 4.5,
		TriageBacklog: 7, ChecksNotGreen: 3,
		TopFailures: []workflowRate{
			{Name: "conformance<script>", Rate: 0.4},
			{Name: "lint", Rate: 0.1},
		},
	}
	out := renderDigest("run-digest", snap)
	if !strings.Contains(out, "|") || !strings.Contains(out, "Open PRs") {
		t.Fatal("digest must be a markdown table with PR aging")
	}
	if !strings.Contains(out, "Triage") || !strings.Contains(out, "7") {
		t.Fatal("must include triage backlog")
	}
	if !strings.Contains(out, "CI") && !strings.Contains(out, "not green") && !strings.Contains(out, "Checks") {
		t.Fatal("must include CI health")
	}
	if strings.Contains(out, "<script>") {
		t.Fatal("workflow names must be escaped")
	}
	if !strings.Contains(out, "2026-W34") {
		t.Fatal("must name the ISO week")
	}
}

func TestRenderFlakyReport(t *testing.T) {
	cands := []intel.FlakyCandidate{
		{Suite: "assert<script>", FailureRate: 0.4, TotalRuns: 10, RecentFailureSHAs: []string{"deadbeef"}},
	}
	out := renderFlakyReport("run-flaky", cands)
	if !strings.Contains(out, "assert") {
		t.Fatal("must name the suite")
	}
	if !strings.Contains(out, "40%") && !strings.Contains(out, "0.40") && !strings.Contains(out, "40") {
		t.Fatal("must include failure rate")
	}
	if !strings.Contains(out, "chainsaw-tests") && !strings.Contains(out, "quarantined-tests") {
		t.Fatal("must include a paste-able chainsaw quarantine snippet")
	}
	if !strings.Contains(out, "deadbeef") {
		t.Fatal("must list failing SHAs")
	}
	if strings.Contains(out, "<script>") {
		t.Fatal("suite names must be escaped")
	}
	lower := strings.ToLower(out)
	if strings.Contains(lower, "applied automatically") || strings.Contains(lower, "has been quarantined") {
		t.Fatal("must not claim quarantine was applied")
	}
	if !strings.Contains(lower, "manual") && !strings.Contains(lower, "human") && !strings.Contains(lower, "paste") {
		t.Fatal("must tell the maintainer to apply the snippet themselves")
	}
}

func TestRenderDocsGap(t *testing.T) {
	pr := &policy.PRFacts{Number: 4, Title: "<script>skip docs</script>"}
	out := renderDocsGap("run-docs", pr, "area/engine (pkg/engine/engine.go)")
	if !strings.Contains(out, "area/engine") {
		t.Fatal("must name the feature surface")
	}
	if !strings.Contains(out, "CONTRIBUTING.md") {
		t.Fatal("must link CONTRIBUTING.md")
	}
	if !strings.Contains(out, "kyverno/website") && !strings.Contains(out, "github.com/kyverno/website") {
		t.Fatal("must point at the website repo")
	}
	if strings.Contains(out, "<script>") {
		t.Fatal("PR title must be escaped")
	}
}
