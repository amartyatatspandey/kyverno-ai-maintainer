package runtime

import (
	"strings"
	"testing"

	"github.com/amartyatatspandey/kyverno-ai-maintainer/internal/policy"
)

func TestRenderChangelog_GroupsAndEscapes(t *testing.T) {
	prs := []policy.PRFacts{
		{Number: 10, Title: "add CEL bind", AuthorLogin: "alice", Labels: []string{"kind/feature", "area/cel"}},
		{Number: 11, Title: "fix nil panic", AuthorLogin: "bob", Labels: []string{"kind/bug", "area/engine"}},
		{Number: 12, Title: "chore(deps): bump foo", AuthorLogin: "app/dependabot", Labels: []string{"kind/dependencies"}},
		{Number: 13, Title: "docs: expand AGENTS.md", AuthorLogin: "carol", Labels: []string{"kind/documentation"}},
		{Number: 14, Title: "unlabeled refactor", AuthorLogin: "dave"},
		{Number: 15, Title: "<script>alert(1)</script> @evil `rm`", AuthorLogin: "mallory", Labels: []string{"kind/cleanup"}},
	}
	out := renderChangelog(prs)

	assertSectionContains(t, out, "Features", "#10")
	assertSectionContains(t, out, "Bug Fixes", "#11")
	assertSectionContains(t, out, "Dependencies", "#12")
	assertSectionContains(t, out, "Documentation", "#13")
	assertSectionContains(t, out, "Other", "#14")
	assertSectionContains(t, out, "Cleanup", "#15")

	if !strings.Contains(out, "- add CEL bind (#10) by @alice") {
		t.Fatalf("feature bullet: %s", out)
	}
	if strings.Contains(out, "<script>") || strings.Contains(out, "@evil") || strings.Contains(out, "`rm`") {
		t.Fatalf("title must go through escapeParam: %s", out)
	}
	if !strings.Contains(out, "&lt;script&gt;") {
		t.Fatal("escaped title must still be present")
	}
}

func assertSectionContains(t *testing.T, doc, heading, needle string) {
	t.Helper()
	idx := strings.Index(doc, "### "+heading)
	if idx < 0 {
		t.Fatalf("missing section %q in:\n%s", heading, doc)
	}
	rest := doc[idx+4:]
	if next := strings.Index(rest, "\n### "); next >= 0 {
		rest = rest[:next]
	}
	if !strings.Contains(rest, needle) {
		t.Fatalf("section %q missing %q:\n%s", heading, needle, rest)
	}
}
