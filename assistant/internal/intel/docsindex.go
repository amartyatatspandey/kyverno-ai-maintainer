package intel

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

// DocSnippet is a retrieved passage from a local documentation file.
// Paths are relative to the index root. Scores are deterministic TF-IDF-ish
// keyword overlap — not an embedding similarity.
type DocSnippet struct {
	Path  string
	Text  string
	Score float64
}

type indexedDoc struct {
	path   string
	text   string
	tf     map[string]float64
	tokens []string
}

// DocsIndex is a bag-of-words index over local markdown (and optional cached
// kyverno.io scrape files dropped in the same tree). No vector DB, no network.
type DocsIndex struct {
	root string
	docs []indexedDoc
	idf  map[string]float64
}

var docsStop = map[string]bool{
	"a": true, "an": true, "the": true, "is": true, "are": true, "was": true, "were": true,
	"be": true, "been": true, "being": true, "have": true, "has": true, "had": true,
	"do": true, "does": true, "did": true, "how": true, "i": true, "in": true, "of": true,
	"and": true, "to": true, "for": true, "with": true, "on": true, "at": true, "from": true,
	"by": true, "or": true, "not": true, "this": true, "that": true, "it": true, "as": true,
	"md": true, "see": true, "http": true, "https": true, "www": true, "com": true,
}

// BuildDocsIndex walks docsDir for .md/.txt/.html files (developer docs plus
// a locally-cached kyverno.io scrape if the operator copies pages into the
// same directory). It does not fetch the network.
func BuildDocsIndex(docsDir string) (*DocsIndex, error) {
	st, err := os.Stat(docsDir)
	if err != nil {
		return nil, fmt.Errorf("docs index: %w", err)
	}
	if !st.IsDir() {
		return nil, fmt.Errorf("docs index: %s is not a directory", docsDir)
	}
	idx := &DocsIndex{root: docsDir, idf: map[string]float64{}}
	df := map[string]int{}
	err = filepath.WalkDir(docsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".md" && ext != ".txt" && ext != ".html" {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(docsDir, path)
		if err != nil {
			rel = path
		}
		rel = filepath.ToSlash(rel)
		toks := tokenize(string(b))
		tf := map[string]float64{}
		for _, t := range toks {
			tf[t]++
		}
		n := float64(len(toks))
		if n > 0 {
			for t, c := range tf {
				tf[t] = c / n
			}
		}
		seen := map[string]bool{}
		for t := range tf {
			if !seen[t] {
				df[t]++
				seen[t] = true
			}
		}
		idx.docs = append(idx.docs, indexedDoc{path: rel, text: string(b), tf: tf, tokens: toks})
		return nil
	})
	if err != nil {
		return nil, err
	}
	n := float64(len(idx.docs))
	if n == 0 {
		return idx, nil
	}
	for term, d := range df {
		idx.idf[term] = math.Log((n+1)/(float64(d)+1)) + 1
	}
	return idx, nil
}

// TopMatches ranks indexed files by TF-IDF overlap with query. Query text is
// untrusted (discussion body): it is tokenized as keywords only and never
// inserted into the corpus.
func (idx *DocsIndex) TopMatches(query string, k int) ([]DocSnippet, error) {
	if idx == nil {
		return nil, fmt.Errorf("nil docs index")
	}
	if k <= 0 {
		k = 3
	}
	qtf := map[string]float64{}
	for _, t := range tokenize(query) {
		qtf[t]++
	}
	type scored struct {
		doc   indexedDoc
		score float64
	}
	var ranked []scored
	for _, d := range idx.docs {
		var num, den float64
		for t, qf := range qtf {
			w := idx.idf[t]
			if w == 0 {
				w = 1
			}
			den += qf * w
			if d.tf[t] > 0 {
				num += qf * w * (1 + d.tf[t])
			}
		}
		var s float64
		if den > 0 {
			s = num / den
		}
		if s > 0 {
			ranked = append(ranked, scored{d, s})
		}
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		return ranked[i].doc.path < ranked[j].doc.path
	})
	if len(ranked) > k {
		ranked = ranked[:k]
	}
	out := make([]DocSnippet, 0, len(ranked))
	for _, r := range ranked {
		out = append(out, DocSnippet{
			Path:  r.doc.path,
			Text:  snippetWindow(r.doc.text, 500),
			Score: r.score,
		})
	}
	return out, nil
}

func tokenize(s string) []string {
	var b strings.Builder
	var out []string
	flush := func() {
		t := strings.ToLower(b.String())
		b.Reset()
		if t == "" || docsStop[t] || len(t) < 2 {
			return
		}
		out = append(out, t)
	}
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return out
}

func snippetWindow(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
