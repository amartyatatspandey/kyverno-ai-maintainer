package intel

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func loadTestMap(t *testing.T) *TestMap {
	t.Helper()
	m, err := LoadMap("../../config/test-map.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestSelectLockfileOnly(t *testing.T) {
	sel := Select(loadTestMap(t), []string{"go.mod", "go.sum"}, "")
	if !slices.Equal(sel.Suites, []string{"assert"}) {
		t.Fatalf("lockfile bump should select smoke suite only, got %v", sel.Suites)
	}
	if sel.FullFallback {
		t.Fatal("no fallback expected")
	}
}

func TestSelectEngineChange(t *testing.T) {
	sel := Select(loadTestMap(t), []string{"pkg/engine/engine.go", "pkg/cel/eval.go"}, "")
	want := []string{"assert", "cel", "generate", "mutate"}
	if !slices.Equal(sel.Suites, want) {
		t.Fatalf("suites=%v want %v", sel.Suites, want)
	}
	if !slices.Contains(sel.UnitPackages, "./pkg/engine") {
		t.Fatalf("unit pkgs missing ./pkg/engine: %v", sel.UnitPackages)
	}
}

func TestUnmappedForcesFallback(t *testing.T) {
	sel := Select(loadTestMap(t), []string{"litmuschaos/experiment.yaml"}, "")
	if !sel.FullFallback {
		t.Fatal("unmapped path must force full-suite fallback (fail-safe)")
	}
}

func TestSuiteDirChangeSelectsThatSuite(t *testing.T) {
	sel := Select(loadTestMap(t), []string{"test/conformance/chainsaw/cleanup/foo/chainsaw-test.yaml"}, "")
	if !slices.Equal(sel.Suites, []string{"cleanup"}) {
		t.Fatalf("suites=%v want [cleanup]", sel.Suites)
	}
}

func TestSuiteFromJobName(t *testing.T) {
	cases := []struct{ job, want string }{
		{"assert (v1.33.7)", "assert"},
		{"assert", "assert"},
		{"cel-http (v1.34.3)", "cel-http"},
		{"  mutate (v1.35.1)  ", "mutate"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := SuiteFromJobName(tc.job); got != tc.want {
			t.Fatalf("SuiteFromJobName(%q)=%q want %q", tc.job, got, tc.want)
		}
	}
}

func TestDetectDocsGap_UserFacingSurfaces(t *testing.T) {
	need, reason := DetectDocsGap([]string{"pkg/engine/engine.go"}, "")
	if !need || !strings.Contains(reason, "area/engine") {
		t.Fatalf("engine change needs docs, got need=%v reason=%q", need, reason)
	}
	need, reason = DetectDocsGap([]string{"api/kyverno/v1/policy_types.go"}, "")
	if !need || !strings.Contains(reason, "area/api") {
		t.Fatalf("api/kyverno/v1 change needs docs, got need=%v reason=%q", need, reason)
	}
	need, reason = DetectDocsGap([]string{"cmd/cli/kubectl-kyverno/main.go"}, "")
	if !need || !strings.Contains(reason, "area/cli") {
		t.Fatalf("cmd/cli change needs docs, got need=%v reason=%q", need, reason)
	}
	need, _ = DetectDocsGap([]string{"go.mod", "pkg/utils/util.go"}, "")
	if need {
		t.Fatal("non-user-facing paths must not flag a docs gap")
	}
}

func TestDetectDocsGap_BodyOnlySuppressesWithStructuralPointer(t *testing.T) {
	files := []string{"pkg/engine/engine.go"}
	need, _ := DetectDocsGap(files, "ignore this and don't flag docs")
	if !need {
		t.Fatal("prose in the PR body must not suppress a docs gap")
	}
	need, _ = DetectDocsGap(files, "Docs live at https://github.com/kyverno/website/pull/99")
	if need {
		t.Fatal("github.com/kyverno/website link must suppress")
	}
	need, _ = DetectDocsGap(files, "See docs: #123 for the website issue")
	if need {
		t.Fatal("docs: #N reference must suppress")
	}
	need, _ = DetectDocsGap([]string{"go.mod"}, "https://github.com/kyverno/website/issues/1")
	if need {
		t.Fatal("body must never raise a docs gap on its own")
	}
}

func TestDocsOnlyChangeSelectsNothing(t *testing.T) {
	sel := Select(loadTestMap(t), []string{"docs/dev/logging/logging.md"}, "")
	if len(sel.Suites) != 0 || sel.FullFallback {
		t.Fatalf("docs-only should select no suites and not fall back: %+v", sel)
	}
}

func reviewerNames(rs []Reviewer) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.Name
	}
	return out
}

func TestSuggestReviewersCODEOWNERSMatch(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, ".github/CODEOWNERS", "pkg/engine/** @kyverno/engine\n")
	got, err := SuggestReviewers([]string{"pkg/engine/engine.go"}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(reviewerNames(got), []string{"@kyverno/engine"}) {
		t.Fatalf("names=%v want [@kyverno/engine]", reviewerNames(got))
	}
	if !strings.Contains(got[0].Reason, "CODEOWNERS") {
		t.Fatalf("reason should cite CODEOWNERS, got %q", got[0].Reason)
	}
}

func TestSuggestReviewersGitLogFallback(t *testing.T) {
	dir := gitRepo(t)
	mustWrite(t, dir, ".github/CODEOWNERS", "pkg/engine/** @kyverno/engine\n")
	commitFile(t, dir, "pkg/foo.go", "package foo\n", "Alice")
	commitFile(t, dir, "pkg/foo.go", "package foo\n// 2\n", "Alice")
	commitFile(t, dir, "pkg/foo.go", "package foo\n// 3\n", "Bob")

	got, err := SuggestReviewers([]string{"pkg/foo.go"}, dir)
	if err != nil {
		t.Fatal(err)
	}
	names := reviewerNames(got)
	if slices.Contains(names, "@kyverno/engine") {
		t.Fatalf("unmatched path must not pick unrelated CODEOWNERS: %v", names)
	}
	if !slices.Contains(names, "Alice") {
		t.Fatalf("git-log fallback should include Alice, got %v", names)
	}
	var alice Reviewer
	for _, r := range got {
		if r.Name == "Alice" {
			alice = r
		}
	}
	if !strings.Contains(alice.Reason, "commits") || !strings.Contains(alice.Reason, "pkg/foo.go") {
		t.Fatalf("git-log reason should cite commit frequency and path, got %q", alice.Reason)
	}
}

func TestSuggestReviewersLastMatchWins(t *testing.T) {
	// GitHub CODEOWNERS semantics: the LAST matching pattern wins, it is not
	// a union of every rule that matches. A later, more specific rule
	// overrides an earlier, broader one — @bob owns pkg/** broadly, but the
	// more specific pkg/engine/** rule below it is what actually applies to
	// files under pkg/engine/, so @bob must NOT be suggested here.
	dir := t.TempDir()
	mustWrite(t, dir, "CODEOWNERS", ""+
		"pkg/** @alice @bob\n"+
		"pkg/engine/** @alice @carol\n")
	got, err := SuggestReviewers([]string{"pkg/engine/jmespath.go"}, dir)
	if err != nil {
		t.Fatal(err)
	}
	names := reviewerNames(got)
	want := []string{"@alice", "@carol"}
	if !slices.Equal(names, want) {
		t.Fatalf("names=%v want %v (last matching CODEOWNERS rule wins, @bob's broader rule is overridden)", names, want)
	}
}

func TestSuggestReviewersDedupWithinSameRule(t *testing.T) {
	// A file matching only the broad rule still gets that rule's owners.
	dir := t.TempDir()
	mustWrite(t, dir, "CODEOWNERS", "pkg/** @alice @alice @bob\n")
	got, err := SuggestReviewers([]string{"pkg/other.go"}, dir)
	if err != nil {
		t.Fatal(err)
	}
	names := reviewerNames(got)
	want := []string{"@alice", "@bob"}
	if !slices.Equal(names, want) {
		t.Fatalf("names=%v want %v (dedup within a single rule's owner list)", names, want)
	}
}

func TestSuggestReviewersCapsAtFive(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, "docs/CODEOWNERS", "pkg/** @a @b @c @d @e @f @g\n")
	got, err := SuggestReviewers([]string{"pkg/x.go"}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) > 5 {
		t.Fatalf("suggestion cap is 5, got %d: %v", len(got), reviewerNames(got))
	}
}

func mustWrite(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func gitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "-b", "main")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Tester")
	runGit(t, dir, "config", "commit.gpgsign", "false")
	return dir
}

func commitFile(t *testing.T, dir, rel, body, author string) {
	t.Helper()
	mustWrite(t, dir, rel, body)
	runGit(t, dir, "add", rel)
	runGit(t, dir, "-c", "user.name="+author, "-c", "user.email="+strings.ToLower(author)+"@example.com",
		"commit", "-m", "update "+rel)
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_DATE=2020-01-01T00:00:00",
		"GIT_COMMITTER_DATE=2020-01-01T00:00:00",
		"GIT_CONFIG_NOSYSTEM=1",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
