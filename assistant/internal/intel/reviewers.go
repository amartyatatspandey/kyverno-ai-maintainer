package intel

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	maxReviewers     = 5
	gitLogWindow     = 10
	gitLogTopPerFile = 3
)

// Reviewer is one suggested reviewer plus a one-line structured reason.
type Reviewer struct {
	Name   string
	Reason string
}

type ownersRule struct {
	pattern string
	owners  []string
}

var codeownersLocations = []string{
	".github/CODEOWNERS",
	"CODEOWNERS",
	"docs/CODEOWNERS",
}

// SuggestReviewers maps changed files to a deduplicated, capped reviewer list
// from CODEOWNERS, falling back to recent git-log authors for unmapped paths.
func SuggestReviewers(changedFiles []string, repoRoot string) ([]Reviewer, error) {
	if repoRoot == "" || len(changedFiles) == 0 {
		return nil, nil
	}
	rules := loadCODEOWNERS(repoRoot)
	seen := map[string]bool{}
	var out []Reviewer

	add := func(name, reason string) {
		if name == "" || seen[name] || len(out) >= maxReviewers {
			return
		}
		seen[name] = true
		out = append(out, Reviewer{Name: name, Reason: reason})
	}

	unmapped := make([]string, 0, len(changedFiles))
	for _, f := range changedFiles {
		// CODEOWNERS semantics: the LAST matching pattern in the file wins,
		// not a union of every pattern that matches — a later, more specific
		// rule overrides an earlier, broader one (matches GitHub's own
		// CODEOWNERS resolution, see github.com CODEOWNERS docs).
		var last *ownersRule
		for i := range rules {
			if codeownersMatch(rules[i].pattern, f) {
				last = &rules[i]
			}
		}
		if last == nil {
			unmapped = append(unmapped, f)
			continue
		}
		reason := "owns " + last.pattern + " per CODEOWNERS"
		for _, o := range last.owners {
			add(o, reason)
		}
	}
	for _, f := range unmapped {
		for _, a := range gitLogTopAuthors(repoRoot, f) {
			add(a.name, fmt.Sprintf("%d of last %d commits to %s", a.count, a.window, f))
		}
	}
	return out, nil
}

func loadCODEOWNERS(repoRoot string) []ownersRule {
	for _, rel := range codeownersLocations {
		b, err := os.ReadFile(filepath.Join(repoRoot, rel))
		if err != nil {
			continue
		}
		return parseCODEOWNERS(string(b))
	}
	return nil
}

func parseCODEOWNERS(s string) []ownersRule {
	var rules []ownersRule
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		rules = append(rules, ownersRule{pattern: fields[0], owners: fields[1:]})
	}
	return rules
}

func codeownersMatch(pattern, file string) bool {
	p := strings.TrimPrefix(pattern, "/")
	if p == "*" || p == "**" {
		return true
	}
	if strings.HasSuffix(p, "/") {
		p += "**"
	}
	return globMatch(p, file)
}

type authorCount struct {
	name   string
	count  int
	window int
}

func gitLogTopAuthors(repoRoot, file string) []authorCount {
	cmd := exec.Command("git", "log", "-n", strconv.Itoa(gitLogWindow), "--format=%an", "--", file)
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	window := 0
	counts := map[string]int{}
	var order []string
	for _, line := range lines {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		window++
		if counts[name] == 0 {
			order = append(order, name)
		}
		counts[name]++
	}
	sort.SliceStable(order, func(i, j int) bool {
		return counts[order[i]] > counts[order[j]]
	})
	if len(order) > gitLogTopPerFile {
		order = order[:gitLogTopPerFile]
	}
	var res []authorCount
	for _, n := range order {
		res = append(res, authorCount{name: n, count: counts[n], window: window})
	}
	return res
}
