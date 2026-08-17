package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/amartyatatspandey/kyverno-ai-maintainer/internal/repro"
)

func TestLintCommandKyvernoTestManifest(t *testing.T) {
	cmd := lintCommand("", "test/cli/test/simple/kyverno-test.yaml")
	if len(cmd) < 3 || cmd[0] != "kyverno" || cmd[1] != "test" || cmd[2] != "test/cli/test/simple" {
		t.Fatalf("want kyverno test <dir>, got %v", cmd)
	}
}

func TestLintCommandApplyWithResource(t *testing.T) {
	dir := t.TempDir()
	policy := filepath.Join("charts", "kyverno-policies", "foo.yaml")
	mustWrite(t, dir, policy, "apiVersion: kyverno.io/v1\n")
	mustWrite(t, dir, filepath.Join("charts", "kyverno-policies", "resource.yaml"), "apiVersion: v1\n")
	cmd := lintCommand(dir, filepath.ToSlash(policy))
	if len(cmd) < 5 || cmd[1] != "apply" || cmd[3] != "--resource" {
		t.Fatalf("want kyverno apply --resource, got %v", cmd)
	}
}

func TestPlanLintTargetsDedupesTestDir(t *testing.T) {
	files := []string{
		"test/cli/test/simple/kyverno-test.yaml",
		"test/cli/test/simple/policy.yaml",
	}
	got := planLintTargets("", files)
	if len(got) != 1 || got[0].Target != "test/cli/test/simple" {
		t.Fatalf("one kyverno test per dir, got %+v", got)
	}
}

func TestRunPolicyLintDisabled(t *testing.T) {
	r := &Runner{Enabled: false}
	_, err := r.RunPolicyLint(context.Background(), []string{"a.yaml"}, time.Minute)
	if err == nil {
		t.Fatal("disabled sandbox must refuse")
	}
}

func TestRunPolicyLintDockerGuard(t *testing.T) {
	if Available() {
		t.Skip("docker present — isolation flags are shared with RunUnitTests; no extra live run")
	}
	r := &Runner{Enabled: true, RepoDir: t.TempDir(), Image: "unused"}
	res, err := r.RunPolicyLint(context.Background(), []string{"charts/kyverno-policies/x.yaml"}, time.Second)
	if err == nil && len(res) == 1 && res[0].Passed {
		t.Fatal("absent docker must not report a passing lint")
	}
}

func TestRunReproDisabled(t *testing.T) {
	r := &Runner{Enabled: false}
	_, err := r.RunRepro(&repro.ReproBundle{
		PolicyYAML:     "kind: ClusterPolicy\n",
		ResourceYAML:   "kind: Pod\n",
		KyvernoVersion: "1.18.0",
	})
	if err == nil {
		t.Fatal("disabled sandbox must refuse RunRepro")
	}
}

func TestRunReproRefusesInvalidBundle(t *testing.T) {
	r := &Runner{Enabled: true}
	_, err := r.RunRepro(&repro.ReproBundle{
		PolicyYAML: `apiVersion: kyverno.io/v1
kind: ClusterPolicy
metadata:
  name: x
spec: {}
`,
		ResourceYAML: `apiVersion: v1
kind: Pod
metadata:
  name: x
spec:
  hostNetwork: true
  containers:
  - name: pause
    image: pause
`,
		KyvernoVersion: "1.18.0",
	})
	if err == nil {
		t.Fatal("hostNetwork bundle must never reach docker")
	}
	if !strings.Contains(err.Error(), "invalid repro bundle") {
		t.Fatalf("want invalid-bundle refusal, got %v", err)
	}
}

func TestReproDockerArgs_NoCacheIsNetworkNone(t *testing.T) {
	r := &Runner{}
	args := r.reproDockerArgs("ghcr.io/kyverno/kyverno-cli:v1.18.0", "/tmp/work")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--network=none") {
		t.Fatalf("CLI path must be hermetic: %v", args)
	}
	if !strings.Contains(joined, "--label ai-maintainer-run=true") && !containsPair(args, "--label", "ai-maintainer-run=true") {
		t.Fatalf("missing sandbox label: %v", args)
	}
	if strings.Contains(joined, "policy.yaml") && strings.Contains(joined, "apiVersion") {
		t.Fatal("user YAML must not appear on the docker argv")
	}
}

func TestReproDockerArgs_CacheDropsNetworkNone(t *testing.T) {
	r := &Runner{ImageCache: "/var/cache/ai-images"}
	args := r.reproDockerArgs("ghcr.io/kyverno/kyverno-cli:v1.18.0", "/tmp/work")
	joined := strings.Join(args, " ")
	if strings.Contains(joined, "--network=none") {
		t.Fatal("KinD path cannot use network=none (needs local cache/docker)")
	}
	if !strings.Contains(joined, "/var/cache/ai-images:/image-cache:ro") {
		t.Fatalf("cache must be mounted read-only: %v", args)
	}
}

func TestRunReproDockerGuard(t *testing.T) {
	if Available() {
		t.Skip("docker present — not starting a live KinD cluster in unit tests")
	}
	r := &Runner{Enabled: true}
	b := &repro.ReproBundle{
		PolicyYAML: `apiVersion: kyverno.io/v1
kind: ClusterPolicy
metadata:
  name: require-app-label
spec:
  rules: []
`,
		ResourceYAML: `apiVersion: v1
kind: Pod
metadata:
  name: demo
spec:
  containers:
  - name: pause
    image: registry.k8s.io/pause:3.9
`,
		KyvernoVersion: "1.18.0",
	}
	res, err := r.RunRepro(b)
	if err == nil && res != nil && res.Success {
		t.Fatal("absent docker must not report a successful repro")
	}
}

func containsPair(args []string, k, v string) bool {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == k && args[i+1] == v {
			return true
		}
	}
	return false
}

func mustWrite(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
