package runtime

import (
	"math"
	"testing"
	"time"

	"github.com/amartyatatspandey/kyverno-ai-maintainer/internal/policy"
)

func TestMedianAgeDays(t *testing.T) {
	now := time.Date(2026, 8, 17, 0, 0, 0, 0, time.UTC)
	prs := []policy.PRFacts{
		{CreatedAt: now.Add(-2 * 24 * time.Hour)},
		{CreatedAt: now.Add(-4 * 24 * time.Hour)},
		{CreatedAt: now.Add(-10 * 24 * time.Hour)},
	}
	got := medianAgeDays(prs, now)
	if math.Abs(got-4) > 0.01 {
		t.Fatalf("median=%v want 4", got)
	}
	if medianAgeDays(nil, now) != 0 {
		t.Fatal("empty set median is 0")
	}
}

func TestISOWeekTarget(t *testing.T) {
	// 2026-08-17 is ISO week 34
	got := isoWeekTarget(time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC))
	if got != "digest/2026-W34" {
		t.Fatalf("got %q want digest/2026-W34", got)
	}
}

func TestTopFailureRates(t *testing.T) {
	got := topFailureRates(map[string]float64{
		"a": 0.1, "b": 0.9, "c": 0.5, "d": 0.8,
	}, 3)
	if len(got) != 3 || got[0].Name != "b" || got[1].Name != "d" || got[2].Name != "c" {
		t.Fatalf("got %+v", got)
	}
}

func TestCountChecksNotGreen(t *testing.T) {
	prs := []policy.PRFacts{
		{ChecksGreen: true},
		{ChecksGreen: false},
		{ChecksGreen: false, ChecksPending: true},
	}
	if n := countChecksNotGreen(prs); n != 2 {
		t.Fatalf("got %d want 2", n)
	}
}
