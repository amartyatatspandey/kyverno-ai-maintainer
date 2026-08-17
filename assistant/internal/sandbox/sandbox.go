// Package sandbox executes templated commands in ephemeral, credential-free
// Docker containers (SANDBOX.md). No free-form shell: callers pick a template.
package sandbox

import (
	"context"
	"fmt"
	"os/exec"
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

func Available() bool {
	return exec.Command("docker", "info").Run() == nil
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
		cctx, cancel := context.WithTimeout(ctx, timeout)
		// Fixed template — the only shape a unit-test command can take.
		args := []string{
			"run", "--rm",
			"--network=none",
			"--cpus=4", "--memory=8g", "--memory-swap=8g", "--pids-limit=2048",
			"--read-only", "--tmpfs", "/tmp:size=1g", "--tmpfs", "/gocache:size=4g",
			"-e", "GOCACHE=/gocache/build", "-e", "GOMODCACHE=/gocache/mod", "-e", "GOFLAGS=-mod=mod",
			"-v", r.RepoDir + ":/workspace:ro",
			"-w", "/workspace",
			"--label", "ai-maintainer-run=true",
			r.Image,
			"go", "test", "-count=1", "-timeout", timeout.String(), pkg,
		}
		out, err := exec.CommandContext(cctx, "docker", args...).CombinedOutput()
		cancel()
		res := StageResult{
			Target: pkg, Duration: time.Since(start).Round(time.Second),
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
		results = append(results, res)
	}
	return results, nil
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
