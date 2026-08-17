// Package repro extracts and validates untrusted issue-body YAML for W5.
// Extraction is string-only: no interpretation, execution, or trust of content.
package repro

import (
	"fmt"
	"regexp"
	"strings"
)

// ReproBundle is the structured result of ExtractReproArtifacts.
// YAML fields are still untrusted until ValidateReproBundle says otherwise.
type ReproBundle struct {
	PolicyYAML       string
	ResourceYAML     string
	KyvernoVersion   string
	ExpectedBehavior string // prose from the issue template; never executed
}

// GitHub issue forms render template field labels as `### <label>`.
// Labels come from kyverno/.github/ISSUE_TEMPLATE/bug-webhook.yaml and bug-cli.yaml.
const (
	headingKyvernoVersion    = "Kyverno Version"
	headingKyvernoCLIVersion = "Kyverno CLI Version"
	headingSteps             = "Steps to reproduce"
	headingExpected          = "Expected behavior"
)

var (
	headingRe = regexp.MustCompile(`(?m)^###[ \t]+(.+?)\s*$`)
	// Only yaml/yml fences. Other languages and untagged fences are ignored.
	yamlFenceRe = regexp.MustCompile("(?is)```ya?ml[^\n]*\n(.*?)```")
)

// ExtractReproArtifacts pulls fenced YAML and version/expected prose from a
// bug-report issue body. It does not parse or trust the YAML.
func ExtractReproArtifacts(issueBody string) (*ReproBundle, error) {
	sections := splitSections(issueBody)

	version := firstLine(sections[headingKyvernoVersion])
	if version == "" {
		version = firstLine(sections[headingKyvernoCLIVersion])
	}

	steps, ok := sections[headingSteps]
	if !ok || strings.TrimSpace(steps) == "" {
		return nil, fmt.Errorf("missing %q heading from the bug-report template", headingSteps)
	}

	blocks := yamlFenceRe.FindAllStringSubmatch(steps, -1)
	if len(blocks) < 2 {
		return nil, fmt.Errorf("need two ```yaml fences under %q (policy, then resource)", headingSteps)
	}

	return &ReproBundle{
		PolicyYAML:       strings.TrimSpace(blocks[0][1]),
		ResourceYAML:     strings.TrimSpace(blocks[1][1]),
		KyvernoVersion:   strings.TrimSpace(version),
		ExpectedBehavior: strings.TrimSpace(sections[headingExpected]),
	}, nil
}

func splitSections(body string) map[string]string {
	idxs := headingRe.FindAllStringSubmatchIndex(body, -1)
	out := make(map[string]string, len(idxs))
	for i, loc := range idxs {
		name := strings.TrimSpace(body[loc[2]:loc[3]])
		start := loc[1]
		end := len(body)
		if i+1 < len(idxs) {
			end = idxs[i+1][0]
		}
		out[name] = strings.TrimSpace(body[start:end])
	}
	return out
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}
