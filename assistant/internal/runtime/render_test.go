package runtime

import (
	"strings"
	"testing"

	"github.com/amartyatatspandey/kyverno-ai-maintainer/internal/policy"
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
