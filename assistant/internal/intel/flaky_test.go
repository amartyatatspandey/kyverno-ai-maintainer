package intel

import (
	"testing"
	"time"

	"github.com/amartyatatspandey/kyverno-ai-maintainer/internal/ghx"
)

func rec(suiteJob, sha, conclusion string, hour int) ghx.CheckRunRecord {
	return ghx.CheckRunRecord{
		JobName:    suiteJob,
		SHA:        sha,
		Conclusion: conclusion,
		CreatedAt:  time.Date(2026, 8, 1, hour, 0, 0, 0, time.UTC),
	}
}

func TestDetectFlaky_InterleavedSameSHA_Flagged(t *testing.T) {
	// Same SHA fail then pass = unchanged files, true flake.
	records := []ghx.CheckRunRecord{
		rec("assert (v1.33.7)", "aaa", "failure", 1),
		rec("assert (v1.33.7)", "aaa", "success", 2),
		rec("assert (v1.33.7)", "bbb", "failure", 3),
		rec("assert (v1.33.7)", "ccc", "success", 4),
	}
	got, err := DetectFlaky(records, 0.15)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Suite != "assert" {
		t.Fatalf("want assert flagged, got %+v", got)
	}
	if got[0].TotalRuns != 4 || got[0].FailureRate != 0.5 {
		t.Fatalf("rate/total %+v", got[0])
	}
	if len(got[0].RecentFailureSHAs) == 0 {
		t.Fatal("must list failing SHAs")
	}
}

func TestDetectFlaky_BrokenThenFixed_NotFlagged(t *testing.T) {
	// Consecutive failures, then consecutive successes on later SHAs:
	// the suite was broken, then fixed — not a flake.
	records := []ghx.CheckRunRecord{
		rec("mutate (v1.33.7)", "s1", "failure", 1),
		rec("mutate (v1.33.7)", "s2", "failure", 2),
		rec("mutate (v1.33.7)", "s3", "failure", 3),
		rec("mutate (v1.33.7)", "s4", "success", 4),
		rec("mutate (v1.33.7)", "s5", "success", 5),
		rec("mutate (v1.33.7)", "s6", "success", 6),
	}
	got, err := DetectFlaky(records, 0.15)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("broken-then-fixed must not be flagged, got %+v", got)
	}
}

func TestDetectFlaky_OnlyFlakeSuiteFlaggedWhenMixed(t *testing.T) {
	records := []ghx.CheckRunRecord{
		rec("assert (v1.33.7)", "aaa", "failure", 1),
		rec("assert (v1.33.7)", "aaa", "success", 2),
		rec("assert (v1.33.7)", "bbb", "failure", 3),
		rec("mutate (v1.33.7)", "s1", "failure", 1),
		rec("mutate (v1.33.7)", "s2", "failure", 2),
		rec("mutate (v1.33.7)", "s3", "success", 3),
		rec("mutate (v1.33.7)", "s4", "success", 4),
	}
	got, err := DetectFlaky(records, 0.15)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Suite != "assert" {
		t.Fatalf("only the flake suite, got %+v", got)
	}
}

func TestDetectFlaky_BelowThresholdNotFlagged(t *testing.T) {
	records := []ghx.CheckRunRecord{
		rec("cleanup (v1.33.7)", "x", "success", 1),
		rec("cleanup (v1.33.7)", "x", "failure", 2), // same SHA flake pattern
		rec("cleanup (v1.33.7)", "a", "success", 3),
		rec("cleanup (v1.33.7)", "b", "success", 4),
		rec("cleanup (v1.33.7)", "c", "success", 5),
		rec("cleanup (v1.33.7)", "d", "success", 6),
		rec("cleanup (v1.33.7)", "e", "success", 7),
		rec("cleanup (v1.33.7)", "f", "success", 8),
	}
	// 1/8 = 12.5% < 15%
	got, err := DetectFlaky(records, 0.15)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("below threshold must not flag, got %+v", got)
	}
}

func TestDetectFlaky_DefaultThreshold(t *testing.T) {
	records := []ghx.CheckRunRecord{
		rec("assert (v1.33.7)", "aaa", "failure", 1),
		rec("assert (v1.33.7)", "aaa", "success", 2),
	}
	got, err := DetectFlaky(records, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("zero threshold uses default 15%%; 50%% flake should flag, got %+v", got)
	}
}
