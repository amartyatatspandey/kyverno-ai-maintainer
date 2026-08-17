// Package intel is the deterministic repo-intelligence layer: changed files in,
// unit packages + chainsaw suites out. The LLM may WIDEN the result, never narrow
// it (RISKS A6): see runtime.mergeSelections.
package intel

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type mapEntry struct {
	Suites []string `yaml:"suites"`
}

type TestMap struct {
	Version int                 `yaml:"version"`
	Paths   map[string]mapEntry `yaml:"paths"`
}

type Selection struct {
	UnitPackages  []string `json:"unit_packages"`
	Suites        []string `json:"suites"`
	UnmappedFiles []string `json:"unmapped_files"`
	FullFallback  bool     `json:"full_fallback"` // unmapped files => run everything
}

func LoadMap(path string) (*TestMap, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m TestMap
	if err := yaml.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// Select maps changed files to suites (longest-prefix match) and unit packages
// (package dirs of changed .go files; reverse import closure when repoDir != "").
func Select(m *TestMap, changed []string, repoDir string) Selection {
	sel := Selection{}
	suiteSet := map[string]bool{}
	pkgSet := map[string]bool{}

	for _, f := range changed {
		// Suite mapping: longest matching prefix wins.
		best, bestLen := "", -1
		for prefix := range m.Paths {
			if (strings.HasPrefix(f, prefix) || f == strings.TrimSuffix(prefix, "/")) && len(prefix) > bestLen {
				best, bestLen = prefix, len(prefix)
			}
		}
		if bestLen < 0 {
			sel.UnmappedFiles = append(sel.UnmappedFiles, f)
		} else {
			for _, s := range m.Paths[best].Suites {
				if s == "__self__" {
					// test/conformance/chainsaw/<suite>/... => that suite
					rest := strings.TrimPrefix(f, "test/conformance/chainsaw/")
					if i := strings.Index(rest, "/"); i > 0 {
						suiteSet[rest[:i]] = true
					}
					continue
				}
				suiteSet[s] = true
			}
		}
		// Unit package: directory of changed .go files.
		if strings.HasSuffix(f, ".go") && !strings.Contains(f, "zz_generated") {
			pkgSet["./"+path.Dir(f)] = true
		}
	}

	// Reverse import closure: packages that import the changed packages also
	// need their tests run. Requires a checkout; best-effort with graceful skip.
	if repoDir != "" && len(pkgSet) > 0 {
		for _, dep := range reverseClosure(repoDir, keys(pkgSet)) {
			pkgSet[dep] = true
		}
	}

	sel.Suites = keys(suiteSet)
	sel.UnitPackages = keys(pkgSet)
	sel.FullFallback = len(sel.UnmappedFiles) > 0
	sort.Strings(sel.Suites)
	sort.Strings(sel.UnitPackages)
	return sel
}

// reverseClosure: one `go list -json ./...` pass, build reverse import graph,
// BFS from changed packages. Cached per head SHA by the caller if needed.
func reverseClosure(repoDir string, changedPkgs []string) []string {
	cmd := exec.Command("go", "list", "-json=ImportPath,Imports,Dir", "./...")
	cmd.Dir = repoDir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "intel: go list failed (%v); selection stays direct-packages-only\n", err)
		return nil
	}
	type pkg struct {
		ImportPath string
		Imports    []string
		Dir        string
	}
	// Map module-relative dirs to import paths.
	dec := json.NewDecoder(&out)
	rev := map[string][]string{} // imported -> importers
	byDir := map[string]string{} // "./pkg/engine" -> import path
	for dec.More() {
		var p pkg
		if err := dec.Decode(&p); err != nil {
			break
		}
		rel, err := relDir(repoDir, p.Dir)
		if err == nil {
			byDir[rel] = p.ImportPath
		}
		for _, imp := range p.Imports {
			rev[imp] = append(rev[imp], p.ImportPath)
		}
	}
	seen := map[string]bool{}
	var queue []string
	for _, d := range changedPkgs {
		if ip, ok := byDir[d]; ok {
			queue = append(queue, ip)
			seen[ip] = true
		}
	}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, importer := range rev[cur] {
			if !seen[importer] {
				seen[importer] = true
				queue = append(queue, importer)
			}
		}
	}
	// Convert import paths back to ./dirs for `go test`.
	ipToDir := map[string]string{}
	for d, ip := range byDir {
		ipToDir[ip] = d
	}
	var res []string
	for ip := range seen {
		if d, ok := ipToDir[ip]; ok {
			res = append(res, d)
		}
	}
	return res
}

func relDir(root, dir string) (string, error) {
	if !strings.HasPrefix(dir, root) {
		return "", fmt.Errorf("outside root")
	}
	rel := strings.TrimPrefix(strings.TrimPrefix(dir, root), "/")
	if rel == "" {
		return "./.", nil
	}
	return "./" + rel, nil
}

// globMatch is the path matcher used by test-map prefixes' glob forms and by
// CODEOWNERS. Same rules as policy.globMatch: exact, /** prefix, **/ basename,
// then path.Match. CODEOWNERS reuses this rather than a second matcher.
func globMatch(pattern, file string) bool {
	if pattern == file {
		return true
	}
	if strings.HasSuffix(pattern, "/**") {
		return strings.HasPrefix(file, strings.TrimSuffix(pattern, "**"))
	}
	if strings.HasPrefix(pattern, "**/") {
		base := strings.TrimPrefix(pattern, "**/")
		ok, _ := path.Match(base, path.Base(file))
		return ok || strings.HasSuffix(file, "/"+base)
	}
	ok, _ := path.Match(pattern, file)
	return ok
}

func keys(m map[string]bool) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
