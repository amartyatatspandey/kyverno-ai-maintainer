package intel

import (
	"slices"
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

func TestDocsOnlyChangeSelectsNothing(t *testing.T) {
	sel := Select(loadTestMap(t), []string{"docs/dev/logging/logging.md"}, "")
	if len(sel.Suites) != 0 || sel.FullFallback {
		t.Fatalf("docs-only should select no suites and not fall back: %+v", sel)
	}
}
