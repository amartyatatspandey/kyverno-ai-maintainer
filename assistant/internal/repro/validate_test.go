package repro

import (
	"strings"
	"testing"
)

func validPolicyYAML() string {
	return `apiVersion: kyverno.io/v1
kind: ClusterPolicy
metadata:
  name: require-app-label
spec:
  validationFailureAction: Enforce
  rules:
  - name: check-label
    match:
      any:
      - resources:
          kinds: ["Pod"]
    validate:
      message: label required
      pattern:
        metadata:
          labels:
            app: "?*"
`
}

func validResourceYAML() string {
	return `apiVersion: v1
kind: Pod
metadata:
  name: demo
  labels:
    app: demo
spec:
  containers:
  - name: pause
    image: registry.k8s.io/pause:3.9
`
}

func validBundle() *ReproBundle {
	return &ReproBundle{
		PolicyYAML:     validPolicyYAML(),
		ResourceYAML:   validResourceYAML(),
		KyvernoVersion: "1.18.0",
	}
}

func mustOK(t *testing.T, b *ReproBundle) {
	t.Helper()
	ok, reason := ValidateReproBundle(b)
	if !ok {
		t.Fatalf("want pass, got reject: %s", reason)
	}
}

func mustFail(t *testing.T, b *ReproBundle, substr string) {
	t.Helper()
	ok, reason := ValidateReproBundle(b)
	if ok {
		t.Fatal("want reject, got pass")
	}
	if substr != "" && !strings.Contains(strings.ToLower(reason), strings.ToLower(substr)) {
		t.Fatalf("reason %q does not contain %q", reason, substr)
	}
}

// --- YAML parse ---

func TestValidate_ParsePass(t *testing.T) {
	mustOK(t, validBundle())
}

func TestValidate_ParseFailPolicy(t *testing.T) {
	b := validBundle()
	b.PolicyYAML = "kind: ClusterPolicy\n{{ this is not yaml"
	mustFail(t, b, "parse")
}

func TestValidate_ParseFailResource(t *testing.T) {
	b := validBundle()
	b.ResourceYAML = ":\n  - not: valid: yaml: ["
	mustFail(t, b, "parse")
}

func TestValidate_ParseFailEmptyDocs(t *testing.T) {
	b := validBundle()
	b.PolicyYAML = "# just a comment\n"
	mustFail(t, b, "parse")
}

func TestValidate_NilBundle(t *testing.T) {
	mustFail(t, nil, "nil")
}

// --- Policy kind allowlist (api/kyverno/v1: ClusterPolicy, Policy) ---

func TestValidate_PolicyKindPass_ClusterPolicy(t *testing.T) {
	mustOK(t, validBundle())
}

func TestValidate_PolicyKindPass_Policy(t *testing.T) {
	b := validBundle()
	b.PolicyYAML = strings.Replace(validPolicyYAML(), "kind: ClusterPolicy", "kind: Policy", 1)
	mustOK(t, b)
}

func TestValidate_PolicyKindPass_ValidatingPolicy(t *testing.T) {
	b := validBundle()
	b.PolicyYAML = `apiVersion: policies.kyverno.io/v1beta1
kind: ValidatingPolicy
metadata:
  name: demo
spec:
  matchConstraints:
    resourceRules:
    - apiGroups: [""]
      apiVersions: ["v1"]
      resources: ["pods"]
      operations: ["CREATE"]
  validations:
  - message: need label
    expression: has(object.metadata.labels.app)
`
	mustOK(t, b)
}

func TestValidate_PolicyKindPass_ImageValidatingPolicy(t *testing.T) {
	b := validBundle()
	b.PolicyYAML = `apiVersion: policies.kyverno.io/v1beta1
kind: ImageValidatingPolicy
metadata:
  name: demo
spec: {}
`
	mustOK(t, b)
}

func TestValidate_PolicyKindFail_NotAllowlisted(t *testing.T) {
	b := validBundle()
	b.PolicyYAML = `apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: evil
`
	mustFail(t, b, "kind")
}

func TestValidate_PolicyKindFail_CleanupPolicy(t *testing.T) {
	b := validBundle()
	b.PolicyYAML = `apiVersion: kyverno.io/v2
kind: CleanupPolicy
metadata:
  name: no
spec: {}
`
	mustFail(t, b, "kind")
}

// --- Resource kind allowlist ---

func TestValidate_ResourceKindPass_Pod(t *testing.T) {
	mustOK(t, validBundle())
}

func TestValidate_ResourceKindPass_ConfigMap(t *testing.T) {
	b := validBundle()
	b.ResourceYAML = `apiVersion: v1
kind: ConfigMap
metadata:
  name: demo
data:
  k: v
`
	mustOK(t, b)
}

func TestValidate_ResourceKindFail_Node(t *testing.T) {
	b := validBundle()
	b.ResourceYAML = `apiVersion: v1
kind: Node
metadata:
  name: hijack
`
	mustFail(t, b, "kind")
}

func TestValidate_ResourceKindFail_Secret(t *testing.T) {
	b := validBundle()
	b.ResourceYAML = `apiVersion: v1
kind: Secret
metadata:
  name: steal
`
	mustFail(t, b, "kind")
}

func TestValidate_ResourceKindFail_ServiceAccount(t *testing.T) {
	b := validBundle()
	b.ResourceYAML = `apiVersion: v1
kind: ServiceAccount
metadata:
  name: default
`
	mustFail(t, b, "kind")
}

// --- Forbidden privilege / host-access fields (most important check) ---

func TestValidate_ForbiddenFieldsPass_CleanManifests(t *testing.T) {
	mustOK(t, validBundle())
}

func TestValidate_ForbiddenFieldsPass_StringMentionsCommand(t *testing.T) {
	b := validBundle()
	b.PolicyYAML = `apiVersion: kyverno.io/v1
kind: ClusterPolicy
metadata:
  name: require-app-label
spec:
  rules:
  - name: check
    match:
      any:
      - resources:
          kinds: ["Pod"]
    validate:
      message: "do not set command or hostPath on the pod"
      pattern:
        metadata:
          labels:
            app: "?*"
`
	mustOK(t, b)
}

func TestValidate_ForbiddenFail_hostNetwork(t *testing.T) {
	b := validBundle()
	b.ResourceYAML = `apiVersion: v1
kind: Pod
metadata:
  name: demo
spec:
  hostNetwork: true
  containers:
  - name: pause
    image: registry.k8s.io/pause:3.9
`
	mustFail(t, b, "hostNetwork")
}

func TestValidate_ForbiddenFail_hostPath(t *testing.T) {
	b := validBundle()
	b.ResourceYAML = `apiVersion: v1
kind: Pod
metadata:
  name: demo
spec:
  containers:
  - name: pause
    image: registry.k8s.io/pause:3.9
  volumes:
  - name: host
    hostPath:
      path: /etc
`
	mustFail(t, b, "hostPath")
}

func TestValidate_ForbiddenFail_privileged(t *testing.T) {
	b := validBundle()
	b.ResourceYAML = `apiVersion: v1
kind: Pod
metadata:
  name: demo
spec:
  containers:
  - name: pause
    image: registry.k8s.io/pause:3.9
    securityContext:
      privileged: true
`
	mustFail(t, b, "privileged")
}

func TestValidate_ForbiddenFail_serviceAccountName(t *testing.T) {
	b := validBundle()
	b.ResourceYAML = `apiVersion: v1
kind: Pod
metadata:
  name: demo
spec:
  serviceAccountName: default
  containers:
  - name: pause
    image: registry.k8s.io/pause:3.9
`
	mustFail(t, b, "serviceAccountName")
}

func TestValidate_ForbiddenFail_command(t *testing.T) {
	b := validBundle()
	b.ResourceYAML = `apiVersion: v1
kind: Pod
metadata:
  name: demo
spec:
  containers:
  - name: pause
    image: registry.k8s.io/pause:3.9
    command: ["/bin/sh", "-c", "curl http://evil.example"]
`
	mustFail(t, b, "command")
}

func TestValidate_ForbiddenFail_exec(t *testing.T) {
	b := validBundle()
	// Isolated `exec` key — no sibling `command` so map-iteration cannot
	// report a different forbidden field.
	b.ResourceYAML = `apiVersion: v1
kind: Pod
metadata:
  name: demo
spec:
  containers:
  - name: pause
    image: registry.k8s.io/pause:3.9
    livenessProbe:
      exec: {}
`
	mustFail(t, b, "exec")
}

func TestValidate_ForbiddenFail_NestedInPolicyYAML(t *testing.T) {
	b := validBundle()
	b.PolicyYAML = `apiVersion: kyverno.io/v1
kind: ClusterPolicy
metadata:
  name: require-app-label
spec:
  hostNetwork: false
  rules: []
`
	mustFail(t, b, "hostNetwork")
}

func TestValidate_ForbiddenFail_FalseStillRejected(t *testing.T) {
	b := validBundle()
	b.ResourceYAML = `apiVersion: v1
kind: Pod
metadata:
  name: demo
spec:
  privileged: false
  containers:
  - name: pause
    image: registry.k8s.io/pause:3.9
`
	mustFail(t, b, "privileged")
}

// --- Combined YAML size cap ---

func TestValidate_SizePass_UnderCap(t *testing.T) {
	mustOK(t, validBundle())
}

func TestValidate_SizeFail_OverCap(t *testing.T) {
	b := validBundle()
	b.ResourceYAML = `apiVersion: v1
kind: ConfigMap
metadata:
  name: demo
data:
  blob: ` + strings.Repeat("A", defaultMaxYAMLBytes)
	mustFail(t, b, "size")
}

func TestValidate_SizeFail_CustomCap(t *testing.T) {
	b := validBundle()
	ok, reason := ValidateReproBundleWith(b, Limits{MaxYAMLBytes: 32, AllowedVersions: DefaultAllowedVersions})
	if ok {
		t.Fatal("tiny cap must reject a normal bundle")
	}
	if !strings.Contains(reason, "size") {
		t.Fatalf("reason=%q", reason)
	}
}

// --- Kyverno version: semver + pinned allowlist ---

func TestValidate_VersionPass_Pinned(t *testing.T) {
	for _, v := range DefaultAllowedVersions {
		b := validBundle()
		b.KyvernoVersion = v
		mustOK(t, b)
	}
}

func TestValidate_VersionPass_VPrefix(t *testing.T) {
	b := validBundle()
	b.KyvernoVersion = "v1.18.0"
	mustOK(t, b)
}

func TestValidate_VersionFail_NotSemver(t *testing.T) {
	b := validBundle()
	b.KyvernoVersion = "latest"
	mustFail(t, b, "semver")
}

func TestValidate_VersionFail_Unsupported(t *testing.T) {
	b := validBundle()
	b.KyvernoVersion = "9.9.9"
	ok, reason := ValidateReproBundle(b)
	if ok {
		t.Fatal("unknown version must be rejected, never fetched")
	}
	if !strings.Contains(strings.ToLower(reason), "unsupported version") {
		t.Fatalf("want 'unsupported version' in reason, got %q", reason)
	}
}

func TestValidate_VersionFail_Empty(t *testing.T) {
	b := validBundle()
	b.KyvernoVersion = ""
	mustFail(t, b, "semver")
}
