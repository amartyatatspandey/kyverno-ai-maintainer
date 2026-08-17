package intel

import (
	"sort"
	"strings"

	"github.com/amartyatatspandey/kyverno-ai-maintainer/internal/ghx"
)

const defaultFlakeThreshold = 0.15

type FlakyCandidate struct {
	Suite             string
	FailureRate       float64
	TotalRuns         int
	RecentFailureSHAs []string
}

// DetectFlaky flags suites whose failure rate exceeds threshold and whose
// history is a true flake (pass after fail on the same SHA, or interleaved
// fail/pass) rather than a monotonic broken-then-fixed streak.
func DetectFlaky(records []ghx.CheckRunRecord, threshold float64) ([]FlakyCandidate, error) {
	if threshold <= 0 {
		threshold = defaultFlakeThreshold
	}
	byJob := map[string][]ghx.CheckRunRecord{}
	for _, r := range records {
		if SuiteFromJobName(r.JobName) == "" {
			continue
		}
		byJob[r.JobName] = append(byJob[r.JobName], r)
	}

	type acc struct {
		fail, total int
		flake       bool
		failSHAs    []string
		seenSHA     map[string]bool
	}
	bySuite := map[string]*acc{}
	for job, recs := range byJob {
		sort.Slice(recs, func(i, j int) bool { return recs[i].CreatedAt.Before(recs[j].CreatedAt) })
		suite := SuiteFromJobName(job)
		a := bySuite[suite]
		if a == nil {
			a = &acc{seenSHA: map[string]bool{}}
			bySuite[suite] = a
		}
		if jobFlakeSignal(recs) {
			a.flake = true
		}
		for _, r := range recs {
			c := strings.ToLower(r.Conclusion)
			if c != "success" && c != "failure" {
				continue
			}
			a.total++
			if c == "failure" {
				a.fail++
				if r.SHA != "" && !a.seenSHA[r.SHA] {
					a.seenSHA[r.SHA] = true
					a.failSHAs = append(a.failSHAs, r.SHA)
				}
			}
		}
	}

	var out []FlakyCandidate
	for suite, a := range bySuite {
		if !a.flake || a.total == 0 {
			continue
		}
		rate := float64(a.fail) / float64(a.total)
		if rate <= threshold {
			continue
		}
		shas := a.failSHAs
		if len(shas) > 8 {
			shas = shas[len(shas)-8:]
		}
		out = append(out, FlakyCandidate{
			Suite: suite, FailureRate: rate, TotalRuns: a.total,
			RecentFailureSHAs: shas,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Suite < out[j].Suite })
	return out, nil
}

func jobFlakeSignal(recs []ghx.CheckRunRecord) bool {
	type shaPair struct{ fail, pass bool }
	bySHA := map[string]*shaPair{}
	seenFail := false
	seenPassAfterFail := false
	failAfterPass := false
	for _, r := range recs {
		c := strings.ToLower(r.Conclusion)
		if c != "success" && c != "failure" {
			continue
		}
		fail := c == "failure"
		if r.SHA != "" {
			p := bySHA[r.SHA]
			if p == nil {
				p = &shaPair{}
				bySHA[r.SHA] = p
			}
			if fail {
				p.fail = true
			} else {
				p.pass = true
			}
		}
		if fail {
			seenFail = true
			if seenPassAfterFail {
				failAfterPass = true
			}
		} else if seenFail {
			seenPassAfterFail = true
		}
	}
	for _, p := range bySHA {
		if p.fail && p.pass {
			return true // same unchanged SHA failed and later passed
		}
	}
	if !seenPassAfterFail {
		return false
	}
	return failAfterPass // interleaved; monotonic fail-then-pass is a fix
}
