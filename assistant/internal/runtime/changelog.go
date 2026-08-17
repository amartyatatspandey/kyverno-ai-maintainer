package runtime

import (
	"fmt"
	"strings"

	"github.com/amartyatatspandey/kyverno-ai-maintainer/internal/policy"
)

// changelogSections maps Kyverno labels.yml kind/* (plus the common
// feature/bug/cleanup aliases the prompt named) onto changelog headings.
// First match wins; unlabeled or area-only PRs fall through to Other.
var changelogSections = []struct {
	heading string
	labels  []string
}{
	{"Features", []string{"kind/feature", "enhancement"}},
	{"Bug Fixes", []string{"kind/bug", "bug"}},
	{"Cleanup", []string{"kind/cleanup", "kind/chore"}},
	{"Dependencies", []string{"kind/dependencies", "dependencies"}},
	{"Helm", []string{"kind/helm", "kind/helm-kyverno", "kind/helm-policies"}},
	{"Tests", []string{"kind/tests", "kind/e2e-tests", "kind/cli-tests"}},
	{"Documentation", []string{"kind/documentation"}},
	{"CI", []string{"kind/ci"}},
	{"Codegen", []string{"kind/codegen"}},
	{"Security", []string{"kind/security"}},
	{"Config", []string{"kind/config"}},
}

const otherSection = "Other"

// DraftReleaseNotes builds a changelog markdown block from PRs merged since
// sinceRef (git tag or YYYY-MM-DD). It is a local draft: no GitHub write and
// no policy Decision — a human pastes the result into release notes.
func (r *Runner) DraftReleaseNotes(sinceRef string) (string, error) {
	prs, err := r.gh.ListMergedPRsSince(sinceRef)
	if err != nil {
		return "", err
	}
	return renderChangelog(prs), nil
}

func renderChangelog(prs []policy.PRFacts) string {
	buckets := map[string][]policy.PRFacts{}
	var order []string
	for _, sec := range changelogSections {
		order = append(order, sec.heading)
	}
	order = append(order, otherSection)

	for _, pr := range prs {
		h := changelogHeading(pr.Labels)
		buckets[h] = append(buckets[h], pr)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "### Changelog\n\n")
	for _, heading := range order {
		items := buckets[heading]
		if len(items) == 0 {
			continue
		}
		fmt.Fprintf(&b, "### %s\n\n", heading)
		for _, pr := range items {
			title := escapeParam(pr.Title, 200)
			author := strings.TrimPrefix(pr.AuthorLogin, "@")
			fmt.Fprintf(&b, "- %s (#%d) by @%s\n", title, pr.Number, author)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func changelogHeading(labels []string) string {
	have := map[string]bool{}
	for _, l := range labels {
		have[strings.ToLower(l)] = true
	}
	for _, sec := range changelogSections {
		for _, l := range sec.labels {
			if have[l] {
				return sec.heading
			}
		}
	}
	return otherSection
}
