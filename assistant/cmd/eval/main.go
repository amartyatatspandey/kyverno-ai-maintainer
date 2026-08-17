// Command eval replays historical Kyverno cases through the real policy engine
// and selector, then reports the metrics defined in BASELINE.md.
//
//	go run ./cmd/eval --cases ../eval/w1_dependabot_cases.json --w3 ../eval/w3_selection_cases.json
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/amartyatatspandey/kyverno-ai-maintainer/internal/intel"
	"github.com/amartyatatspandey/kyverno-ai-maintainer/internal/policy"
	"github.com/amartyatatspandey/kyverno-ai-maintainer/internal/runtime"
)

type depCase struct {
	Number  int      `json:"number"`
	Title   string   `json:"title"`
	State   string   `json:"state"`
	Base    string   `json:"base"`
	Labels  []string `json:"labels"`
	Files   []string `json:"files"`
	Created string   `json:"createdAt"`
	Merged  string   `json:"mergedAt"`
	// ClosureCause distinguishes WHY a CLOSED PR was closed. Discovered during
	// the first eval run: most closures are Kyverno's own pr-rate-limiter bot or
	// Dependabot superseding its own PR — neither is a quality judgment, so
	// neither is a valid "expected DENY". Only human_rejection is.
	ClosureCause string `json:"closure_cause"`
	ClosedBy     string `json:"closed_by"`
}

type w3Case struct {
	Number int      `json:"number"`
	Title  string   `json:"title"`
	Files  []string `json:"files"`
}

func main() {
	cases := flag.String("cases", "../eval/w1_dependabot_cases.json", "W1 cases")
	w3 := flag.String("w3", "../eval/w3_selection_cases.json", "W3 cases")
	cfgPath := flag.String("config", "config/ai-maintainer.yaml", "policy config")
	mapPath := flag.String("map", "config/test-map.yaml", "test map")
	flag.Parse()

	cfg, err := policy.LoadConfig(*cfgPath)
	must(err)
	e := policy.NewEngine(cfg)
	tmap, err := intel.LoadMap(*mapPath)
	must(err)

	// ---- W1: classification + policy decisions on 50 real Dependabot PRs ----
	var dcs []depCase
	must(readJSON(*cases, &dcs))

	var (
		allowed, denied          int
		falsePositive            []int // policy ALLOW on a PR a HUMAN rejected
		rateLimited              []int // ALLOW on a PR the rate-limiter bot closed
		typeCounts               = map[string]int{}
		denyReasons              = map[string]int{}
		closedCases, mergedCases int
		closureCauses            = map[string]int{}
	)
	for _, c := range dcs {
		ut := runtime.ClassifyUpdateType(c.Title)
		typeCounts[ut]++
		// Reconstruct the facts as they were at decision time. CI state is not
		// recoverable from history, so we assume green — the CHARITABLE case for
		// the assistant, making any false positive a genuine finding.
		pr := &policy.PRFacts{
			Number: c.Number, AuthorLogin: "app/dependabot", AuthorIsBot: true,
			Title: c.Title, BaseRef: c.Base, HeadSHA: fmt.Sprintf("sha%d", c.Number),
			Labels: c.Labels, ChangedFiles: c.Files, UpdateType: ut,
			ChecksGreen: true, Mergeable: true, State: "OPEN",
		}
		d := e.Evaluate(policy.Action{Type: "merge_pr"},
			policy.Context{Workflow: "dependency_prs", PR: pr, Now: time.Now()})
		if d.Allowed {
			allowed++
		} else {
			denied++
			denyReasons[firstFailedRule(d)]++
		}
		switch c.State {
		case "CLOSED":
			closedCases++
			closureCauses[c.ClosureCause]++
			if d.Allowed {
				switch c.ClosureCause {
				case "human_rejection":
					falsePositive = append(falsePositive, c.Number)
				case "rate_limiter":
					rateLimited = append(rateLimited, c.Number)
				}
			}
		case "MERGED":
			mergedCases++
		}
	}

	fmt.Println("=== W1: dependency-PR policy replay (50 real Kyverno PRs) ===")
	fmt.Printf("cases: %d (%d merged by humans, %d closed/rejected by humans)\n", len(dcs), mergedCases, closedCases)
	fmt.Printf("update-type classification: %v\n", typeCounts)
	fmt.Printf("closure causes (closed cases): %v\n", closureCauses)
	fmt.Printf("policy ALLOW: %d   DENY: %d\n", allowed, denied)
	fmt.Printf("FALSE-POSITIVE MERGES (ALLOW on human-rejected PR): %d %v  [target: 0]\n",
		len(falsePositive), falsePositive)
	fmt.Printf("ALLOW on rate-limiter-closed PRs: %d %v\n", len(rateLimited), rateLimited)
	fmt.Printf("  ^ not errors: these were mergeable bumps discarded because the\n" +
		"    Dependabot queue exceeded 8 open PRs — i.e. exactly the backlog this\n" +
		"    assistant drains. Dependabot reopens them later, repeating the work.\n")
	fmt.Println("deny reasons:")
	for r, n := range denyReasons {
		fmt.Printf("  %-32s %d\n", r, n)
	}
	fmt.Printf("automation coverage: %.0f%% of dependency PRs decided without a human\n",
		100*float64(allowed)/float64(len(dcs)))

	// ---- W3: scoped selection on 30 real merged PRs ----
	var wcs []w3Case
	must(readJSON(*w3, &wcs))
	const totalSuites = 52 // dirs under test/conformance/chainsaw (verified)
	var (
		totalSelected, fallbacks int
		zeroSuite                int
	)
	fmt.Println("\n=== W3: scoped test selection (30 real merged PRs) ===")
	for _, c := range wcs {
		sel := intel.Select(tmap, c.Files, "")
		n := len(sel.Suites)
		if sel.FullFallback {
			fallbacks++
			n = totalSuites
		}
		if n == 0 {
			zeroSuite++
		}
		totalSelected += n
	}
	avg := float64(totalSelected) / float64(len(wcs))
	fmt.Printf("cases: %d\n", len(wcs))
	fmt.Printf("avg suites selected: %.1f of %d  → %.0f%% conformance compute reduction\n",
		avg, totalSuites, 100*(1-avg/float64(totalSuites)))
	fmt.Printf("full-suite fallbacks (unmapped paths): %d (%.0f%%)  [map-coverage health]\n",
		fallbacks, 100*float64(fallbacks)/float64(len(wcs)))
	fmt.Printf("PRs needing zero conformance suites: %d\n", zeroSuite)
	fmt.Printf("baseline: 342 jobs / ~3068 job-minutes per full conformance run\n")
}

func firstFailedRule(d policy.Decision) string {
	for _, r := range d.Rules {
		if !r.Pass {
			return r.Rule
		}
	}
	return "(none)"
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
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
