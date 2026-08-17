package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
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
