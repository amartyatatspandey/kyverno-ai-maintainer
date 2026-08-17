// Package sandbox executes templated commands in ephemeral, credential-free
// Docker containers (SANDBOX.md). No free-form shell: callers pick a template.
package sandbox

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"
)

type Runner struct {
	Image   string // pinned digest in production; tag for POC
	RepoDir string // read-only bind mount of the pinned checkout
	Enabled bool   // --sandbox flag; requires docker daemon
}

type StageResult struct {
	Target   string        `json:"target"`
	ExitCode int           `json:"exit_code"`
	Passed   bool          `json:"passed"`
	Duration time.Duration `json:"duration"`
	LogTail  string        `json:"log_tail"` // bounded before leaving the sandbox
}

// LintResult is the policy-lint stage result; same shape as unit-test stages.
type LintResult = StageResult

func Available() bool {
	return exec.Command("docker", "info").Run() == nil
}

// PolicyLintImage is a kyverno-cli-equipped image. Runner.Image (golang:1.25)
// has no kyverno binary. This POC has no Dockerfile; adding the CLI to the
// default sandbox image is the production follow-up. Until then, policy lint
// uses the official CLI image with the same isolation flags as RunUnitTests.
const PolicyLintImage = "ghcr.io/kyverno/kyverno-cli:latest"

func (r *Runner) isolationArgs(image string, extraEnv, extraTmpfs []string) []string {
	args := []string{
		"run", "--rm",
		"--network=none",
		"--cpus=4", "--memory=8g", "--memory-swap=8g", "--pids-limit=2048",
		"--read-only", "--tmpfs", "/tmp:size=1g",
	}
	args = append(args, extraTmpfs...)
	args = append(args, extraEnv...)
	args = append(args,
		"-v", r.RepoDir+":/workspace:ro",
		"-w", "/workspace",
		"--label", "ai-maintainer-run=true",
		image,
	)
	return args
}

func (r *Runner) runContainer(ctx context.Context, timeout time.Duration, image string, extraEnv, extraTmpfs, cmd []string) ([]byte, error) {
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	args := append(r.isolationArgs(image, extraEnv, extraTmpfs), cmd...)
	return exec.CommandContext(cctx, "docker", args...).CombinedOutput()
}

// RunUnitTests: unprivileged, network=none (deps must be vendored/warm — for
// the POC we allow module downloads via GOFLAGS=-mod=mod + network for
// simplicity IF SandboxNet is set; default is hermetic).
func (r *Runner) RunUnitTests(ctx context.Context, packages []string, timeout time.Duration) ([]StageResult, error) {
	if !r.Enabled {
		return nil, fmt.Errorf("sandbox disabled (run with --sandbox and a running docker daemon)")
	}
	var results []StageResult
	for _, pkg := range packages {
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		default:
		}
		start := time.Now()
		extraEnv := []string{
			"-e", "GOCACHE=/gocache/build", "-e", "GOMODCACHE=/gocache/mod", "-e", "GOFLAGS=-mod=mod",
		}
		extraTmpfs := []string{"--tmpfs", "/gocache:size=4g"}
		out, err := r.runContainer(ctx, timeout, r.Image, extraEnv, extraTmpfs,
			[]string{"go", "test", "-count=1", "-timeout", timeout.String(), pkg})
		results = append(results, stageFromCmd(pkg, start, out, err))
	}
	return results, nil
}

type lintTarget struct {
	Target string
	Cmd    []string
}

// RunPolicyLint runs `kyverno test <dir>` when a kyverno-test manifest is in
// play, otherwise `kyverno apply <policy>.yaml --resource <resource>.yaml`.
func (r *Runner) RunPolicyLint(ctx context.Context, changedYAMLFiles []string, timeout time.Duration) ([]LintResult, error) {
	if !r.Enabled {
		return nil, fmt.Errorf("sandbox disabled (run with --sandbox and a running docker daemon)")
	}
	if timeout == 0 {
		timeout = 2 * time.Minute
	}
	var results []LintResult
	for _, t := range planLintTargets(r.RepoDir, changedYAMLFiles) {
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		default:
		}
		start := time.Now()
		out, err := r.runContainer(ctx, timeout, PolicyLintImage, nil, nil, t.Cmd)
		results = append(results, stageFromCmd(t.Target, start, out, err))
	}
	return results, nil
}

func planLintTargets(repoDir string, files []string) []lintTarget {
	tested := map[string]bool{}
	var out []lintTarget
	for _, f := range files {
		f = path.Clean(filepath.ToSlash(f))
		dir := path.Dir(f)
		if isKyvernoTestManifest(f) || dirHasKyvernoTest(repoDir, dir) {
			if tested[dir] {
				continue
			}
			tested[dir] = true
			out = append(out, lintTarget{Target: dir, Cmd: []string{"kyverno", "test", dir}})
			continue
		}
		if tested[dir] {
			continue
		}
		out = append(out, lintTarget{Target: f, Cmd: lintCommand(repoDir, f)})
	}
	return out
}

func lintCommand(repoDir, file string) []string {
	file = path.Clean(filepath.ToSlash(file))
	dir := path.Dir(file)
	if isKyvernoTestManifest(file) || dirHasKyvernoTest(repoDir, dir) {
		return []string{"kyverno", "test", dir}
	}
	for _, res := range []string{"resource.yaml", "resources.yaml", "resource.yml", "resources.yml"} {
		candidate := path.Join(dir, res)
		if repoDir == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(repoDir, filepath.FromSlash(candidate))); err == nil {
			return []string{"kyverno", "apply", file, "--resource", candidate}
		}
	}
	return []string{"kyverno", "apply", file}
}

func isKyvernoTestManifest(file string) bool {
	b := strings.ToLower(path.Base(file))
	return b == "kyverno-test.yaml" || b == "kyverno-test.yml"
}

func dirHasKyvernoTest(repoDir, dir string) bool {
	if repoDir == "" {
		return false
	}
	for _, name := range []string{"kyverno-test.yaml", "kyverno-test.yml"} {
		if _, err := os.Stat(filepath.Join(repoDir, filepath.FromSlash(dir), name)); err == nil {
			return true
		}
	}
	return false
}

func stageFromCmd(target string, start time.Time, out []byte, err error) StageResult {
	res := StageResult{
		Target: target, Duration: time.Since(start).Round(time.Second),
		LogTail: tail(string(out), 2048),
	}
	if err == nil {
		res.Passed, res.ExitCode = true, 0
	} else if ee, ok := err.(*exec.ExitError); ok {
		res.ExitCode = ee.ExitCode()
	} else {
		res.ExitCode = -1
		res.LogTail = err.Error() + "\n" + res.LogTail
	}
	return res
}

// KillAll: the manual kill path (`assistant stop` / OVERRIDE.md §5).
func KillAll() error {
	out, _ := exec.Command("docker", "ps", "-q", "--filter", "label=ai-maintainer-run=true").Output()
	ids := strings.Fields(string(out))
	if len(ids) == 0 {
		return nil
	}
	return exec.Command("docker", append([]string{"kill"}, ids...)...).Run()
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}
