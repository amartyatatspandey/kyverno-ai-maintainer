// Command recall-eval measures W3 selection recall via the path-exercise
// proxy in internal/intel (see that package's doc comment). It is a
// separate binary from cmd/eval because sandbox execution is expensive
// and the cheap `go run ./cmd/eval` demo path must stay fast. Results
// cache in eval/w3_ground_truth.json; cmd/eval prints the cached block.
//
//	go run ./cmd/recall-eval --pilot 5
//	go run ./cmd/recall-eval --recompute
//	go run ./cmd/recall-eval --sandbox --repo-dir /path/to/kyverno
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/amartyatatspandey/kyverno-ai-maintainer/internal/intel"
	"github.com/amartyatatspandey/kyverno-ai-maintainer/internal/sandbox"
)

type w3Case struct {
	Number int      `json:"number"`
	Title  string   `json:"title"`
	Files  []string `json:"files"`
}

func main() {
	w3 := flag.String("w3", "../eval/w3_selection_cases.json", "W3 cases")
	mapPath := flag.String("map", "config/test-map.yaml", "test map")
	out := flag.String("out", "../eval/w3_ground_truth.json", "cache path")
	pilot := flag.Int("pilot", 0, "score only the first N cases (0 = all)")
	recompute := flag.Bool("recompute", false, "ignore existing cache and rewrite")
	useSandbox := flag.Bool("sandbox", false, "run selected unit tests in Docker against --repo-dir")
	useChainsaw := flag.Bool("chainsaw", false, "also run comparison-set chainsaw suites (requires --sandbox; skipped on FullFallback)")
	repoDir := flag.String("repo-dir", "", "Kyverno checkout (suite catalog + sandbox mount)")
	image := flag.String("image", "golang:1.25", "sandbox image")
	unitTimeout := flag.Duration("unit-timeout", 5*time.Minute, "per-package go test timeout")
	chainsawTimeout := flag.Duration("chainsaw-timeout", 10*time.Minute, "per-suite chainsaw timeout")
	flag.Parse()

	if !*recompute && !*useSandbox {
		if gt, err := loadGroundTruth(*out); err == nil {
			fmt.Print(intel.FormatGroundTruth(gt))
			fmt.Fprintf(os.Stderr, "cached %s (pass --recompute to rewrite)\n", *out)
			return
		}
	}

	if *useSandbox {
		if *repoDir == "" {
			fatal("--sandbox requires --repo-dir pointing at a Kyverno checkout")
		}
		if !sandbox.Available() {
			fatal("docker daemon not available")
		}
	}

	tmap, err := intel.LoadMap(*mapPath)
	must(err)
	var cases []w3Case
	must(readJSON(*w3, &cases))
	if *pilot > 0 && *pilot < len(cases) {
		cases = cases[:*pilot]
	}

	catalog := intel.ListSuiteCatalog(*repoDir)
	runner := &sandbox.Runner{
		Image: *image, RepoDir: *repoDir, Enabled: *useSandbox,
	}

	start := time.Now()
	var scores []intel.CaseScore
	for _, c := range cases {
		var extra []string
		overlay := (*intel.SandboxOverlay)(nil)
		if *useSandbox {
			overlay, extra = runSandbox(runner, tmap, c.Files, catalog, *useChainsaw, *unitTimeout, *chainsawTimeout)
		}
		cs := intel.ScoreRecall(tmap, c.Number, c.Files, catalog, extra)
		cs.Sandbox = overlay
		scores = append(scores, cs)
	}
	gt := intel.SummarizeRecall(scores, time.Since(start), *pilot)
	gt.SandboxUsed = *useSandbox
	gt.ChainsawUsed = *useChainsaw
	gt.RepoDir = *repoDir

	raw, err := json.MarshalIndent(gt, "", "  ")
	must(err)
	must(os.WriteFile(*out, append(raw, '\n'), 0o644))
	fmt.Print(intel.FormatGroundTruth(gt))
	fmt.Fprintf(os.Stderr, "wrote %s\n", *out)
}

func runSandbox(r *sandbox.Runner, tmap *intel.TestMap, files, catalog []string, chainsaw bool, unitTO, csTO time.Duration) (*intel.SandboxOverlay, []string) {
	sel := intel.Select(tmap, files, r.RepoDir)
	ov := &intel.SandboxOverlay{Ran: true, Kind: "unit"}
	ctx := context.Background()
	if len(sel.UnitPackages) > 0 {
		ov.Targets = append(ov.Targets, sel.UnitPackages...)
		res, err := r.RunUnitTests(ctx, sel.UnitPackages, unitTO)
		if err != nil {
			ov.Error = err.Error()
		}
		for _, s := range res {
			if !s.Passed && !infraFailure(s) {
				ov.Failed = append(ov.Failed, s.Target)
			}
		}
	}

	var extra []string
	scored := intel.ScoreRecall(tmap, 0, files, catalog, nil)
	if chainsaw && !scored.FullFallback && len(scored.Comparison) > 0 {
		ov.Kind = "unit+chainsaw"
		ov.Targets = append(ov.Targets, scored.Comparison...)
		res, err := r.RunChainsawSuites(ctx, scored.Comparison, csTO)
		if err != nil && ov.Error == "" {
			ov.Error = err.Error()
		}
		for _, s := range res {
			if s.Passed || infraFailure(s) {
				continue
			}
			ov.Failed = append(ov.Failed, s.Target)
			extra = append(extra, s.Target)
		}
	}
	return ov, extra
}

// infraFailure is a missing binary / image, not a test assertion. Overlaying
// these as needed-set hits would inflate recall with "docker had no chainsaw".
func infraFailure(s sandbox.StageResult) bool {
	log := strings.ToLower(s.LogTail)
	if s.ExitCode == 127 {
		return true
	}
	return strings.Contains(log, "executable file not found") ||
		strings.Contains(log, "chainsaw: not found") ||
		strings.Contains(log, "go: not found")
}

func loadGroundTruth(path string) (*intel.GroundTruth, error) {
	var gt intel.GroundTruth
	if err := readJSON(path, &gt); err != nil {
		return nil, err
	}
	if gt.Methodology == "" {
		return nil, fmt.Errorf("empty cache")
	}
	return &gt, nil
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func must(err error) {
	if err != nil {
		fatal(err.Error())
	}
}

func fatal(msg string) {
	fmt.Fprintln(os.Stderr, "error:", msg)
	os.Exit(1)
}
