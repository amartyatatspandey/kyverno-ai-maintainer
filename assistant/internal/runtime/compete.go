package runtime

import (
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/amartyatatspandey/kyverno-ai-maintainer/internal/policy"
)

// bumpModRe captures the module path from a Dependabot-style title
// ("chore(deps): bump github.com/google/cel-go from 0.29.2 to 0.30.0").
var bumpModRe = regexp.MustCompile(`(?i)bump\s+(\S+)\s+from\s+`)

// competingPRNumbers returns other currently-open PR numbers whose changed
// files overlap self's. For go.mod/go.sum-only diffs (the common Dependabot
// shape) file overlap is near-universal, so we additionally require the same
// module path from the title — that's the #16768/#16782 case, not "any other
// bump is open". Non-lockfile overlap is enough on its own (a human CVE fix
// that also touches go.mod still competes).
func competingPRNumbers(self *policy.PRFacts, open []policy.PRFacts) []int {
	if self == nil {
		return nil
	}
	selfFiles := map[string]struct{}{}
	for _, f := range self.ChangedFiles {
		selfFiles[f] = struct{}{}
	}
	selfMod := moduleFromTitle(self.Title)
	var out []int
	for _, o := range open {
		if o.Number == self.Number {
			continue
		}
		if !filesOverlap(selfFiles, o.ChangedFiles) {
			continue
		}
		if onlyGoLockfiles(self.ChangedFiles) && onlyGoLockfiles(o.ChangedFiles) {
			other := moduleFromTitle(o.Title)
			if selfMod == "" || other == "" || selfMod != other {
				continue
			}
		}
		out = append(out, o.Number)
	}
	slices.Sort(out)
	return out
}

func filesOverlap(self map[string]struct{}, other []string) bool {
	for _, f := range other {
		if _, ok := self[f]; ok {
			return true
		}
	}
	return false
}

func onlyGoLockfiles(files []string) bool {
	if len(files) == 0 {
		return false
	}
	for _, f := range files {
		base := path.Base(f)
		if base != "go.mod" && base != "go.sum" {
			return false
		}
	}
	return true
}

func moduleFromTitle(title string) string {
	m := bumpModRe.FindStringSubmatch(title)
	if m == nil {
		return ""
	}
	mod := m[1]
	// Grouped titles ("bump the kubernetes group") capture "the"; require a
	// module-shaped token.
	if !strings.ContainsAny(mod, "/.") {
		return ""
	}
	return mod
}
