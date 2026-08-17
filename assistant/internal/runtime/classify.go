package runtime

import (
	"regexp"
	"strconv"
	"strings"
)

// bumpRe matches Dependabot titles like
//
//	"chore(deps): bump k8s.io/client-go from 0.34.1 to 0.34.2"
//	"chore(deps): bump the kubernetes group with 5 updates"
var bumpRe = regexp.MustCompile(`from v?(\d+)\.(\d+)(?:\.(\d+))?\S* to v?(\d+)\.(\d+)(?:\.(\d+))?`)

// ClassifyUpdateType is DETERMINISTIC parsing of the PR title — never the LLM,
// never the PR body (injection vector I4). Unparseable => "unknown" => policy
// denies auto-merge => human review. Grouped bumps without a single version
// range are "unknown" by design (conservative).
func ClassifyUpdateType(title string) string {
	m := bumpRe.FindStringSubmatch(title)
	if m == nil {
		return "unknown"
	}
	fromMaj, _ := strconv.Atoi(m[1])
	toMaj, _ := strconv.Atoi(m[4])
	fromMin, _ := strconv.Atoi(m[2])
	toMin, _ := strconv.Atoi(m[5])
	switch {
	case toMaj > fromMaj:
		return "major"
	case toMaj < fromMaj:
		return "unknown" // downgrade — never auto-merge
	case toMin != fromMin:
		return "minor"
	default:
		return "patch"
	}
}

// DependencyGroup extracts the Dependabot group when present (k8s/sigstore/otel
// groups configured in .github/dependabot.yml).
func DependencyGroup(title string) string {
	if i := strings.Index(title, " the "); i >= 0 {
		rest := title[i+5:]
		if j := strings.Index(rest, " group"); j > 0 {
			return rest[:j]
		}
	}
	return ""
}
