package runtime

import (
	"slices"
	"testing"

	"github.com/amartyatatspandey/kyverno-ai-maintainer/internal/policy"
)

func TestCompetingPRNumbers_CelGoCase(t *testing.T) {
	self := &policy.PRFacts{
		Number: 16768, Title: "chore(deps): bump github.com/google/cel-go from 0.29.2 to 0.30.0",
		ChangedFiles: []string{"go.mod", "go.sum"},
	}
	open := []policy.PRFacts{
		{Number: 16768, Title: self.Title, ChangedFiles: self.ChangedFiles},
		{Number: 16782, Title: "fix: bump cel-go for CVE (github.com/google/cel-go from 0.29.2 to 0.30.0)",
			ChangedFiles: []string{"go.mod", "go.sum", "pkg/cel/cel.go"}},
		{Number: 17000, Title: "chore(deps): bump k8s.io/client-go from 0.34.1 to 0.34.2",
			ChangedFiles: []string{"go.mod", "go.sum"}},
	}
	got := competingPRNumbers(self, open)
	if !slices.Equal(got, []int{16782}) {
		t.Fatalf("got %v want [#16782]; other go.mod bumps must not compete", got)
	}
}

func TestCompetingPRNumbers_SameModuleLockfiles(t *testing.T) {
	self := &policy.PRFacts{
		Number: 1, Title: "chore(deps): bump github.com/foo/bar from 1.0.0 to 1.0.1",
		ChangedFiles: []string{"go.mod", "go.sum"},
	}
	open := []policy.PRFacts{
		{Number: 2, Title: "chore(deps): bump github.com/foo/bar from 1.0.0 to 1.0.2",
			ChangedFiles: []string{"go.mod", "go.sum"}},
	}
	got := competingPRNumbers(self, open)
	if !slices.Equal(got, []int{2}) {
		t.Fatalf("got %v want [2]", got)
	}
}

func TestCompetingPRNumbers_IgnoresSelf(t *testing.T) {
	self := &policy.PRFacts{
		Number: 9, Title: "chore(deps): bump github.com/foo/bar from 1.0.0 to 1.0.1",
		ChangedFiles: []string{"go.mod"},
	}
	got := competingPRNumbers(self, []policy.PRFacts{*self})
	if len(got) != 0 {
		t.Fatalf("self must not compete with itself, got %v", got)
	}
}
