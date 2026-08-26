package repro

import (
	"fmt"
	"io"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Limits is the config-driven allowlist for ValidateReproBundle.
// Defaults match config/ai-maintainer.yaml (repro.max_yaml_bytes, allowed_versions).
type Limits struct {
	MaxYAMLBytes    int
	AllowedVersions []string
}

const defaultMaxYAMLBytes = 64 * 1024 // 64KB

// DefaultAllowedVersions is the issue-template dropdown
// (bug-webhook.yaml / bug-cli.yaml). The sandbox only has images for these.
var DefaultAllowedVersions = []string{"1.16.4", "1.17.2", "1.18.0"}

// DefaultLimits matches the issue-template dropdown. Unknown versions are
// rejected — the sandbox never interpolates a user string into an image ref.
func DefaultLimits() Limits {
	return Limits{MaxYAMLBytes: defaultMaxYAMLBytes, AllowedVersions: append([]string{}, DefaultAllowedVersions...)}
}

// Policy kinds allowed as the first artifact. api/kyverno/v1 registers
// ClusterPolicy and Policy only. ValidatingPolicy and ImageValidatingPolicy
// exist as policies.kyverno.io CRDs (used in-tree); they are included because
// the W5 spec lists them. CleanupPolicy / PolicyException are not.
var allowedPolicyKinds = map[string]bool{
	"ClusterPolicy":         true,
	"Policy":                true,
	"ValidatingPolicy":      true,
	"ImageValidatingPolicy": true,
}

// Small explicit core-Kubernetes resource allowlist. Not "anything with a kind".
var allowedResourceKinds = map[string]bool{
	"Pod":                   true,
	"Deployment":            true,
	"ReplicaSet":            true,
	"StatefulSet":           true,
	"DaemonSet":             true,
	"Job":                   true,
	"CronJob":               true,
	"ConfigMap":             true,
	"Service":               true,
	"PersistentVolumeClaim": true,
	"LimitRange":            true,
	"ResourceQuota":         true,
	"Namespace":             true,
}

// Privilege / host-access field names. Rejected as keys anywhere in either
// document (case-insensitive). This is the most important check in W5.
var forbiddenFields = map[string]bool{
	"exec":               true,
	"command":            true,
	"hostpath":           true,
	"hostnetwork":        true,
	"privileged":         true,
	"serviceaccountname": true,
}

var semverRe = regexp.MustCompile(`^v?\d+\.\d+\.\d+$`)

// ValidateReproBundle is the hard allowlist gate. All checks must pass.
// Unknown Kyverno versions are rejected with "unsupported version" — never fetched.
func ValidateReproBundle(b *ReproBundle) (ok bool, reason string) {
	return ValidateReproBundleWith(b, DefaultLimits())
}

// ValidateReproBundleWith applies config-driven size/version caps.
func ValidateReproBundleWith(b *ReproBundle, lim Limits) (ok bool, reason string) {
	if b == nil {
		return false, "nil repro bundle"
	}
	if lim.MaxYAMLBytes <= 0 {
		lim.MaxYAMLBytes = defaultMaxYAMLBytes
	}
	if len(lim.AllowedVersions) == 0 {
		lim.AllowedVersions = DefaultAllowedVersions
	}

	combined := len(b.PolicyYAML) + len(b.ResourceYAML)
	if combined == 0 {
		return false, "empty policy/resource yaml"
	}
	if combined > lim.MaxYAMLBytes {
		return false, fmt.Sprintf("combined yaml size %d exceeds cap %d", combined, lim.MaxYAMLBytes)
	}

	ver := strings.TrimSpace(b.KyvernoVersion)
	if !semverRe.MatchString(ver) {
		return false, "kyverno version is not semver"
	}
	verNorm := strings.TrimPrefix(ver, "v")
	if !containsFold(lim.AllowedVersions, verNorm) {
		return false, fmt.Sprintf("unsupported version %s", verNorm)
	}

	policyDocs, err := parseYAMLDocs(b.PolicyYAML)
	if err != nil {
		return false, "policy yaml parse error: " + err.Error()
	}
	resourceDocs, err := parseYAMLDocs(b.ResourceYAML)
	if err != nil {
		return false, "resource yaml parse error: " + err.Error()
	}

	for _, doc := range policyDocs {
		kind := docKind(doc)
		if !allowedPolicyKinds[kind] {
			return false, fmt.Sprintf("policy kind %q is not in the allowlist", kind)
		}
	}
	for _, doc := range resourceDocs {
		kind := docKind(doc)
		if !allowedResourceKinds[kind] {
			return false, fmt.Sprintf("resource kind %q is not in the allowlist", kind)
		}
	}

	if field := firstForbiddenField(policyDocs); field != "" {
		return false, "forbidden field: " + field
	}
	if field := firstForbiddenField(resourceDocs); field != "" {
		return false, "forbidden field: " + field
	}
	return true, ""
}

func containsFold(list []string, want string) bool {
	for _, s := range list {
		if strings.EqualFold(strings.TrimPrefix(s, "v"), want) {
			return true
		}
	}
	return false
}

func parseYAMLDocs(s string) ([]any, error) {
	dec := yaml.NewDecoder(strings.NewReader(s))
	var docs []any
	for {
		var doc any
		err := dec.Decode(&doc)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if doc == nil {
			continue
		}
		docs = append(docs, doc)
	}
	if len(docs) == 0 {
		return nil, fmt.Errorf("no yaml documents")
	}
	return docs, nil
}

func docKind(doc any) string {
	m := asStringMap(doc)
	if m == nil {
		return ""
	}
	k, _ := m["kind"].(string)
	return k
}

func asStringMap(v any) map[string]any {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	return m
}

func firstForbiddenField(docs []any) string {
	for _, doc := range docs {
		if f := walkForbidden(doc); f != "" {
			return f
		}
	}
	return ""
}

func walkForbidden(v any) string {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			if forbiddenFields[strings.ToLower(k)] {
				return k
			}
			if f := walkForbidden(val); f != "" {
				return f
			}
		}
	case []any:
		for _, item := range t {
			if f := walkForbidden(item); f != "" {
				return f
			}
		}
	}
	return ""
}
