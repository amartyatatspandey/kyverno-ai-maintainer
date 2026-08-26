package intel

import (
	"slices"
	"strings"
	"testing"
)

func TestPathRelevantSuites_NameSegment(t *testing.T) {
	cat := DefaultSuiteCatalog
	got := PathRelevantSuites([]string{
		"cmd/cli/kubectl-kyverno/utils/common/fetch.go",
	}, cat)
	if !slices.Equal(got, []string{"cli"}) {
		t.Fatalf("cli path: got %v want [cli]", got)
	}
	got = PathRelevantSuites([]string{"pkg/webhooks/handlers/trace.go"}, cat)
	if !slices.Equal(got, []string{"webhooks"}) {
		t.Fatalf("webhooks path: got %v want [webhooks] (independent of the test-map)", got)
	}
	got = PathRelevantSuites([]string{"pkg/controllers/ttl/controller.go"}, cat)
	if !slices.Equal(got, []string{"ttl"}) {
		t.Fatalf("ttl path: got %v want [ttl] (ttl is a real chainsaw suite)", got)
	}
	got = PathRelevantSuites([]string{"pkg/engine/apicall/executor.go"}, cat)
	if len(got) != 0 {
		t.Fatalf("engine is not a suite name, got %v", got)
	}
}

func TestPathRelevantSuites_ChainsawSelf(t *testing.T) {
	got := PathRelevantSuites([]string{
		"test/conformance/chainsaw/cleanup/foo/chainsaw-test.yaml",
	}, DefaultSuiteCatalog)
	if !slices.Equal(got, []string{"cleanup"}) {
		t.Fatalf("got %v want [cleanup]", got)
	}
}

func TestConventionalSuites_EngineNotNameMatch(t *testing.T) {
	got := ConventionalSuites([]string{"pkg/engine/jmespath/functions.go"})
	want := []string{"assert", "generate", "mutate"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestScoreRecall_CLIHit(t *testing.T) {
	cs := ScoreRecall(loadTestMap(t), 17124, []string{
		"cmd/cli/kubectl-kyverno/utils/common/fetch.go",
	}, DefaultSuiteCatalog, nil)
	if !cs.Scored || !cs.Hit {
		t.Fatalf("cli should hit, got %+v", cs)
	}
	if !slices.Contains(cs.Needed, "cli") || !slices.Contains(cs.Selected, "cli") {
		t.Fatalf("needed/selected should include cli: %+v", cs)
	}
}

func TestScoreRecall_TTLMissesTTLSuite(t *testing.T) {
	cs := ScoreRecall(loadTestMap(t), 17104, []string{
		"pkg/controllers/ttl/controller.go",
	}, DefaultSuiteCatalog, nil)
	if !cs.Scored || cs.Hit {
		t.Fatalf("ttl suite is path-relevant but not in the map's longest-prefix (pkg/controllers/ → assert,generate); got %+v", cs)
	}
	if !slices.Contains(cs.Missed, "ttl") {
		t.Fatalf("missed=%v want ttl", cs.Missed)
	}
	if slices.Contains(cs.Selected, "ttl") {
		t.Fatal("Select must not already include ttl — that would make this test tautological")
	}
}

func TestScoreRecall_WebhooksNameSegmentMiss(t *testing.T) {
	cs := ScoreRecall(loadTestMap(t), 17087, []string{
		"pkg/webhooks/handlers/trace.go",
	}, DefaultSuiteCatalog, nil)
	if !cs.Scored || cs.Hit {
		t.Fatalf("webhooks suite is path-relevant; map selects assert/mutate/policy-validation, not webhooks: %+v", cs)
	}
	if !slices.Contains(cs.Missed, "webhooks") {
		t.Fatalf("missed=%v want webhooks", cs.Missed)
	}
}

func TestScoreRecall_ChartsUnscored(t *testing.T) {
	cs := ScoreRecall(loadTestMap(t), 17094, []string{
		"charts/kyverno/Chart.yaml",
	}, DefaultSuiteCatalog, nil)
	if cs.Scored {
		t.Fatalf("charts have no path-exercise suite signal, got %+v", cs)
	}
}

func TestScoreRecall_UnmappedImageFallbackCoversNeeded(t *testing.T) {
	cs := ScoreRecall(loadTestMap(t), 17078, []string{
		"pkg/image/verifiers/cpol/notary/registry.go",
	}, DefaultSuiteCatalog, nil)
	if !cs.FullFallback {
		t.Fatal("pkg/image is unmapped")
	}
	if !cs.Scored || !cs.Hit {
		t.Fatalf("fallback must cover conventional verify-images/image-validating-policies: %+v", cs)
	}
	if !slices.Contains(cs.Needed, "verify-images") {
		t.Fatalf("needed=%v", cs.Needed)
	}
}

func TestScoreRecall_ComparisonIsUnionNotSample(t *testing.T) {
	cs := ScoreRecall(loadTestMap(t), 17116, []string{
		"pkg/engine/apicall/executor.go",
	}, DefaultSuiteCatalog, nil)
	if !cs.Hit {
		t.Fatalf("engine conventional should match the map, got %+v", cs)
	}
	for _, s := range cs.Selected {
		if !slices.Contains(cs.Comparison, s) {
			t.Fatalf("comparison must be a superset of selected, missing %s", s)
		}
	}
	for _, s := range cs.Needed {
		if !slices.Contains(cs.Comparison, s) {
			t.Fatalf("comparison must be a superset of needed, missing %s", s)
		}
	}
}

func TestScoreRecall_SandboxOverlayAddsNeeded(t *testing.T) {
	cs := ScoreRecall(loadTestMap(t), 1, []string{"docs/README.md"}, DefaultSuiteCatalog, []string{"assert"})
	if !cs.Scored || !slices.Contains(cs.Needed, "assert") {
		t.Fatalf("extraNeeded overlay should score docs-only as needing assert: %+v", cs)
	}
}

func TestSummarizeRecall_Averages(t *testing.T) {
	m := loadTestMap(t)
	var scores []CaseScore
	for i, files := range [][]string{
		{"cmd/cli/kubectl-kyverno/main.go"},
		{"pkg/controllers/ttl/controller.go"},
		{"charts/kyverno/Chart.yaml"},
	} {
		scores = append(scores, ScoreRecall(m, i, files, DefaultSuiteCatalog, nil))
	}
	gt := SummarizeRecall(scores, 0, 0)
	if gt.ScoredCases != 2 || gt.UnscoredCases != 1 {
		t.Fatalf("scored=%d unscored=%d", gt.ScoredCases, gt.UnscoredCases)
	}
	if gt.SuiteRecall >= 1 {
		t.Fatalf("ttl miss should pull suite recall below 1, got %v", gt.SuiteRecall)
	}
	if !strings.Contains(FormatGroundTruth(gt), "missing ttl") {
		t.Fatalf("report should name the ttl miss:\n%s", FormatGroundTruth(gt))
	}
}

func TestListSuiteCatalog_FromCheckout(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, "test/conformance/chainsaw/assert/.keep", "")
	mustWrite(t, dir, "test/conformance/chainsaw/_step-templates/.keep", "")
	got := ListSuiteCatalog(dir)
	if !slices.Equal(got, []string{"assert"}) {
		t.Fatalf("got %v want [assert] (_step-templates skipped)", got)
	}
}

func TestDefaultSuiteCatalogHasIndependentSignals(t *testing.T) {
	for _, s := range []string{"ttl", "webhooks", "cli", "assert"} {
		if !slices.Contains(DefaultSuiteCatalog, s) {
			t.Fatalf("catalog missing %s", s)
		}
	}
	if slices.Contains(DefaultSuiteCatalog, "_step-templates") {
		t.Fatal("_step-templates is not a CI shard")
	}
}
