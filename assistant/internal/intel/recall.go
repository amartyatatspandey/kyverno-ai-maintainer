package intel

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Path-exercise recall (T1 proxy) — methodology
//
// The original EVAL_HARNESS plan mined GitHub check-runs on intermediate PR
// commits for failing conformance jobs. That protocol is invalid: since
// kyverno/kyverno#15658, conformance does not run on pull requests (only
// post-merge on main/release-* or via /conformance). Current eval cases
// therefore have empty check-run history; implementing archaeology would
// silently report 0, which is worse than not measuring.
//
// Replacement metric, computed by ScoreRecall:
//
//	needed  = suites that independently exercise the same package paths as
//	          the PR's changed files (name-segment ∩ catalog, plus a small
//	          conventional-area table for packages whose suite name does
//	          not appear in the path). Sandbox comparison-run failures, if
//	          any, overlay onto needed. needed is NOT intel.Select's output
//	          — using the test-map as both selector and oracle would make
//	          recall tautological.
//	selected = intel.Select suites, or the full catalog on FullFallback
//	          (fail-safe is scored as covering everything).
//	comparison = selected ∪ needed. This is a documented widening, not a
//	          silent sample of the 52-suite matrix. Full KinD × 52 × 30
//	          is not practical locally (30m timeout per suite).
//	suite_recall = |needed ∩ selected| / |needed| over scored cases
//	          (needed empty ⇒ unscored: charts/CI/docs have no
//	          path-exercise signal).
//
// W3 stays advisory: this is a proxy, not historical-failure recall.

// DefaultSuiteCatalog is test/conformance/chainsaw/ directory names from
// kyverno/kyverno (enumerated 2026-08-26). "_step-templates" is excluded —
// it is shared scaffolding, not a CI shard. Prefer ListSuiteCatalog when a
// checkout is available so the catalog cannot drift.
var DefaultSuiteCatalog = []string{
	"assert", "autogen", "background-only", "cel", "cleanup", "cli", "configs",
	"custom-sigstore", "deferred", "deleting-policies", "events", "exceptions",
	"filter", "flags", "force-failure-policy-ignore", "generate",
	"generate-mutating-admission-policy", "generate-mutating-admission-policy-alpha",
	"generate-mutating-admission-policy-v1", "generate-validating-admission-policy",
	"generating-policies", "globalcontext", "image-validating-policies", "lease",
	"mutate", "mutating-admission-policy-reports", "mutating-admission-policy-reports-alpha",
	"mutating-admission-policy-reports-v1", "mutating-policies",
	"namespaced-deleting-policies", "namespaced-generating-policies",
	"namespaced-image-validating-policies", "namespaced-mutating-policies",
	"namespaced-validating-policies", "openreports", "policy-exceptions-disabled",
	"policy-validation", "rangeoperators", "rbac", "reports", "reports-exclude-result",
	"sigstore-custom-tuf", "tls-certificates", "ttl", "validate",
	"validating-admission-policy-reports", "validating-policies", "verify-images",
	"verify-manifests", "webhook-configurations", "webhooks",
}

// conventionalAreas maps changed-path prefixes to suites that exercise those
// packages when the suite name is NOT a path segment (so name-matching cannot
// fire). Longest prefix wins. This table is hand-maintained and is deliberately
// not generated from test-map.yaml — drift is the T1 signal.
var conventionalAreas = []struct {
	prefix string
	suites []string
}{
	{"pkg/controllers/cleanup/", []string{"cleanup"}},
	{"pkg/controllers/webhook/", []string{"webhook-configurations"}},
	{"pkg/engine/", []string{"assert", "mutate", "generate"}},
	{"pkg/cel/", []string{"cel"}},
	{"pkg/autogen/", []string{"autogen"}},
	{"pkg/exceptions/", []string{"exceptions"}},
	{"pkg/globalcontext/", []string{"globalcontext"}},
	{"pkg/event/", []string{"events"}},
	{"pkg/policy/", []string{"policy-validation"}},
	{"pkg/validation/", []string{"policy-validation", "validate"}},
	{"pkg/background/", []string{"background-only", "generate", "generating-policies"}},
	{"pkg/image/", []string{"verify-images", "image-validating-policies"}},
	{"cmd/cli/", []string{"cli"}},
	{"pkg/cli/", []string{"cli"}},
	{"api/", []string{"policy-validation", "assert"}},
	{"go.mod", []string{"assert"}},
	{"go.sum", []string{"assert"}},
}

var genericPathSegments = map[string]bool{
	"pkg": true, "cmd": true, "api": true, "test": true, "tests": true,
	"internal": true, "charts": true, "github": true, "workflows": true,
	"docs": true, "hack": true, "scripts": true, "config": true, "source": true,
	"utils": true, "common": true, "conformance": true, "kyverno": true,
	"kubectl-kyverno": true, "handlers": true, "chainsaw": true,
}

const RecallMethodology = "sandboxed-path-exercise-2026-08-26"

// CaseScore is one W3 case's path-exercise recall.
type CaseScore struct {
	Number        int             `json:"number"`
	Selected      []string        `json:"selected"`
	Needed        []string        `json:"needed"`
	Comparison    []string        `json:"comparison"`
	FullFallback  bool            `json:"full_fallback"`
	Scored        bool            `json:"scored"`
	Hit           bool            `json:"hit"`
	Missed        []string        `json:"missed,omitempty"`
	NeededCovered int             `json:"needed_covered"`
	NeededTotal   int             `json:"needed_total"`
	UnitPackages  []string        `json:"unit_packages,omitempty"`
	Sandbox       *SandboxOverlay `json:"sandbox,omitempty"`
}

// SandboxOverlay records optional execution against a provided checkout.
// Failures do not rewrite Needed unless OverlayFailures is applied by the
// caller after distinguishing infrastructure errors from test failures.
type SandboxOverlay struct {
	Ran     bool     `json:"ran"`
	Kind    string   `json:"kind"` // unit | chainsaw
	Targets []string `json:"targets,omitempty"`
	Failed  []string `json:"failed,omitempty"`
	Error   string   `json:"error,omitempty"`
}

// GroundTruth is the cached W3 recall report (eval/w3_ground_truth.json).
type GroundTruth struct {
	Methodology            string      `json:"methodology"`
	ComputedAt             string      `json:"computed_at"`
	Pilot                  int         `json:"pilot,omitempty"`
	SandboxUsed            bool        `json:"sandbox_used"`
	ChainsawUsed           bool        `json:"chainsaw_used"`
	RepoDir                string      `json:"repo_dir,omitempty"`
	WallClockMS            int64       `json:"wall_clock_ms"`
	Notes                  string      `json:"notes"`
	SuiteRecall            float64     `json:"suite_recall"`
	CaseCoverage           float64     `json:"case_coverage"`
	SuiteRecallMappedOnly  float64     `json:"suite_recall_mapped_only"`
	CaseCoverageMappedOnly float64     `json:"case_coverage_mapped_only"`
	ScoredCases            int         `json:"scored_cases"`
	UnscoredCases          int         `json:"unscored_cases"`
	MappedScoredCases      int         `json:"mapped_scored_cases"`
	Cases                  []CaseScore `json:"cases"`
}

// ListSuiteCatalog returns chainsaw suite dirs from a Kyverno checkout, or
// DefaultSuiteCatalog if repoDir is empty / has no suite tree.
func ListSuiteCatalog(repoDir string) []string {
	if repoDir == "" {
		return append([]string(nil), DefaultSuiteCatalog...)
	}
	entries, err := os.ReadDir(filepath.Join(repoDir, "test", "conformance", "chainsaw"))
	if err != nil {
		return append([]string(nil), DefaultSuiteCatalog...)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), "_") || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		names = append(names, e.Name())
	}
	if len(names) == 0 {
		return append([]string(nil), DefaultSuiteCatalog...)
	}
	return keys(sliceSet(names))
}

// PathRelevantSuites returns catalog suites whose names appear as a
// non-generic path segment of a changed file, plus the suite directory when
// the file lives under test/conformance/chainsaw/<suite>/.
func PathRelevantSuites(files, catalog []string) []string {
	cat := sliceSet(catalog)
	seen := map[string]bool{}
	for _, f := range files {
		f = strings.TrimPrefix(filepath.ToSlash(f), "./")
		if rest, ok := strings.CutPrefix(f, "test/conformance/chainsaw/"); ok {
			suite, _, _ := strings.Cut(rest, "/")
			if cat[suite] {
				seen[suite] = true
			}
		}
		for _, seg := range strings.Split(f, "/") {
			if genericPathSegments[seg] || !cat[seg] {
				continue
			}
			seen[seg] = true
		}
	}
	return keys(seen)
}

// ConventionalSuites returns suites from the independent area table for the
// longest matching prefix of each file.
func ConventionalSuites(files []string) []string {
	seen := map[string]bool{}
	for _, f := range files {
		f = strings.TrimPrefix(filepath.ToSlash(f), "./")
		best, bestLen := []string(nil), -1
		for _, a := range conventionalAreas {
			if !fileMatchesPrefix(f, a.prefix) {
				continue
			}
			if len(a.prefix) > bestLen {
				best, bestLen = a.suites, len(a.prefix)
			}
		}
		for _, s := range best {
			seen[s] = true
		}
	}
	return keys(seen)
}

func fileMatchesPrefix(file, prefix string) bool {
	if file == prefix {
		return true
	}
	if strings.HasSuffix(prefix, "/") {
		return strings.HasPrefix(file, prefix)
	}
	return false
}

// ScoreRecall scores one case. extraNeeded is an optional sandbox-failure
// overlay already filtered to real test failures (not "binary not found").
func ScoreRecall(m *TestMap, number int, files, catalog, extraNeeded []string) CaseScore {
	sel := Select(m, files, "")
	needed := intersectCatalog(unionSorted(
		PathRelevantSuites(files, catalog),
		ConventionalSuites(files),
		extraNeeded,
	), catalog)
	effective := sel.Suites
	if sel.FullFallback {
		effective = append([]string(nil), catalog...)
	}
	cs := CaseScore{
		Number:       number,
		Selected:     append([]string(nil), effective...),
		Needed:       needed,
		Comparison:   unionSorted(effective, needed),
		FullFallback: sel.FullFallback,
		UnitPackages: sel.UnitPackages,
		NeededTotal:  len(needed),
	}
	if len(needed) == 0 {
		return cs
	}
	cs.Scored = true
	cs.Missed = missingFrom(effective, needed)
	cs.NeededCovered = len(needed) - len(cs.Missed)
	cs.Hit = len(cs.Missed) == 0
	return cs
}

// SummarizeRecall aggregates case scores into a GroundTruth document.
func SummarizeRecall(scores []CaseScore, wall time.Duration, pilot int) *GroundTruth {
	gt := &GroundTruth{
		Methodology: RecallMethodology,
		ComputedAt:  time.Now().UTC().Format(time.RFC3339),
		Pilot:       pilot,
		WallClockMS: elapsedMS(wall),
		Cases:       scores,
		Notes: "Path-exercise recall: of suites that name-match a changed-file " +
			"segment or conventional area (independent of test-map.yaml longest-prefix " +
			"Select), what fraction are in the deterministic selection? FullFallback " +
			"counts as selecting the whole catalog. Unscored = empty needed (no " +
			"path-exercise signal). Comparison set = selected ∪ needed (not a silent " +
			"sample of 52). Full 52-suite KinD × 30 PRs is not practical locally; " +
			"optional --sandbox runs unit tests on selected packages, --chainsaw runs " +
			"the comparison set (capped by that union). Historical check-run " +
			"archaeology was abandoned because conformance no longer runs on PRs " +
			"(kyverno/kyverno#15658).",
	}
	var needN, coverN, casesN, hitsN int
	var mapNeedN, mapCoverN, mapCasesN, mapHitsN int
	for _, c := range scores {
		if !c.Scored {
			gt.UnscoredCases++
			continue
		}
		gt.ScoredCases++
		needN += c.NeededTotal
		coverN += c.NeededCovered
		casesN++
		if c.Hit {
			hitsN++
		}
		if c.FullFallback {
			continue
		}
		gt.MappedScoredCases++
		mapNeedN += c.NeededTotal
		mapCoverN += c.NeededCovered
		mapCasesN++
		if c.Hit {
			mapHitsN++
		}
	}
	gt.SuiteRecall = ratio(coverN, needN)
	gt.CaseCoverage = ratio(hitsN, casesN)
	gt.SuiteRecallMappedOnly = ratio(mapCoverN, mapNeedN)
	gt.CaseCoverageMappedOnly = ratio(mapHitsN, mapCasesN)
	return gt
}

func ratio(num, den int) float64 {
	if den == 0 {
		return 0
	}
	return float64(num) / float64(den)
}

func elapsedMS(d time.Duration) int64 {
	ms := d.Milliseconds()
	if ms == 0 && d > 0 {
		return 1
	}
	return ms
}

// FormatGroundTruth is the shared W3 recall report block for cmd/eval and
// cmd/recall-eval.
func FormatGroundTruth(gt *GroundTruth) string {
	if gt == nil {
		return "W3 selection recall: not cached (run go run ./cmd/recall-eval)\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "W3 selection recall (%s)\n", gt.Methodology)
	fmt.Fprintf(&b, "  suite recall (needed ∩ selected / needed): %.0f%%  [%d scored cases, %d unscored]\n",
		100*gt.SuiteRecall, gt.ScoredCases, gt.UnscoredCases)
	fmt.Fprintf(&b, "  case coverage (needed ⊆ selected):         %.0f%%\n", 100*gt.CaseCoverage)
	fmt.Fprintf(&b, "  mapped-only suite recall (excl. fallback): %.0f%%  [%d cases]\n",
		100*gt.SuiteRecallMappedOnly, gt.MappedScoredCases)
	fmt.Fprintf(&b, "  mapped-only case coverage:                 %.0f%%\n", 100*gt.CaseCoverageMappedOnly)
	fmt.Fprintf(&b, "  wall clock: %d ms  sandbox=%v chainsaw=%v\n", gt.WallClockMS, gt.SandboxUsed, gt.ChainsawUsed)
	if gt.Pilot > 0 {
		fmt.Fprintf(&b, "  pilot: first %d cases\n", gt.Pilot)
	}
	var misses []string
	for _, c := range gt.Cases {
		if c.Scored && !c.Hit {
			misses = append(misses, fmt.Sprintf("#%d missing %s", c.Number, strings.Join(c.Missed, ",")))
		}
	}
	if len(misses) > 0 {
		fmt.Fprintf(&b, "  misses:\n")
		for _, m := range misses {
			fmt.Fprintf(&b, "    %s\n", m)
		}
	}
	return b.String()
}

func sliceSet(in []string) map[string]bool {
	m := map[string]bool{}
	for _, s := range in {
		if s != "" {
			m[s] = true
		}
	}
	return m
}

func unionSorted(sets ...[]string) []string {
	seen := map[string]bool{}
	for _, s := range sets {
		for _, x := range s {
			if x != "" {
				seen[x] = true
			}
		}
	}
	return keys(seen)
}

func intersectCatalog(in, catalog []string) []string {
	cat := sliceSet(catalog)
	seen := map[string]bool{}
	for _, s := range in {
		if cat[s] {
			seen[s] = true
		}
	}
	return keys(seen)
}

func missingFrom(have, need []string) []string {
	set := sliceSet(have)
	var miss []string
	for _, n := range need {
		if !set[n] {
			miss = append(miss, n)
		}
	}
	return miss
}
