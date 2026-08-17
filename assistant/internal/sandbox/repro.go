package sandbox

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/amartyatatspandey/kyverno-ai-maintainer/internal/repro"
)

// ReproResult is the observation from a scripted KinD (or CLI-engine) run.
type ReproResult struct {
	ActualBehavior string `json:"actual_behavior"` // admitted | blocked | error
	Success        bool   `json:"success"`         // actual matched expected when expected is parseable
	Logs           string `json:"logs"`
}

// Pinned kyverno-cli images corresponding to the issue-template version
// dropdown. Never interpolate a user-supplied version string into an image ref.
var pinnedKyvernoCLI = map[string]string{
	"1.16.4": "ghcr.io/kyverno/kyverno-cli:v1.16.4",
	"1.17.2": "ghcr.io/kyverno/kyverno-cli:v1.17.2",
	"1.18.0": "ghcr.io/kyverno/kyverno-cli:v1.18.0",
}

// reproRunScript is trusted (ours). User YAML is only ever files under /repro.
// KinD is used when `kind`+`kubectl` exist in the image; otherwise the pinned
// kyverno CLI engine answers the same admit/block question without docker.sock
// (RISKS S1 — nested KinD wants privileged/dind; this POC stays unprivileged).
const reproRunScript = `#!/bin/sh
set -eu
POLICY=/repro/policy.yaml
RESOURCE=/repro/resource.yaml

if [ -d /image-cache ]; then
  if [ -z "$(ls -A /image-cache 2>/dev/null || true)" ]; then
    echo "image cache mounted but empty; refusing to pull" >&2
    echo "ACTUAL_BEHAVIOR=error"
    exit 2
  fi
  if command -v docker >/dev/null 2>&1; then
    for tar in /image-cache/*.tar; do
      [ -f "$tar" ] || continue
      docker load -i "$tar" >/dev/null
    done
  fi
fi

cleanup_kind() {
  if command -v kind >/dev/null 2>&1; then
    kind delete cluster --name ai-repro >/dev/null 2>&1 || true
  fi
}

if command -v kind >/dev/null 2>&1 && command -v kubectl >/dev/null 2>&1; then
  trap cleanup_kind EXIT INT TERM
  kind create cluster --name ai-repro --wait 90s
  kubectl apply -f "$POLICY"
  set +e
  kubectl apply -f "$RESOURCE" >/tmp/apply.out 2>&1
  rc=$?
  set -e
  cat /tmp/apply.out
  if [ "$rc" -eq 0 ]; then
    echo "ACTUAL_BEHAVIOR=admitted"
  else
    echo "ACTUAL_BEHAVIOR=blocked"
  fi
  exit 0
fi

if ! command -v kyverno >/dev/null 2>&1; then
  echo "neither kind nor kyverno CLI present in sandbox image" >&2
  echo "ACTUAL_BEHAVIOR=error"
  exit 2
fi
set +e
kyverno apply "$POLICY" --resource "$RESOURCE" >/tmp/apply.out 2>&1
rc=$?
set -e
cat /tmp/apply.out
if [ "$rc" -eq 0 ]; then
  echo "ACTUAL_BEHAVIOR=admitted"
else
  echo "ACTUAL_BEHAVIOR=blocked"
fi
`

// RunRepro applies a validated bundle inside container isolation.
// Invalid bundles are refused here as well (defense in depth). The cluster
// (or CLI process) is torn down via docker --rm plus the script's EXIT trap.
func (r *Runner) RunRepro(bundle *repro.ReproBundle) (*ReproResult, error) {
	return r.runRepro(context.Background(), bundle)
}

func (r *Runner) runRepro(ctx context.Context, bundle *repro.ReproBundle) (*ReproResult, error) {
	if !r.Enabled {
		return nil, fmt.Errorf("sandbox disabled (run with --sandbox and a running docker daemon)")
	}
	ok, reason := repro.ValidateReproBundle(bundle)
	if !ok {
		return nil, fmt.Errorf("refusing invalid repro bundle: %s", reason)
	}
	ver := strings.TrimPrefix(strings.TrimSpace(bundle.KyvernoVersion), "v")
	image, pinned := pinnedKyvernoCLI[ver]
	if !pinned {
		return nil, fmt.Errorf("unsupported version %s", ver)
	}

	timeout := r.ReproTimeout
	if timeout == 0 {
		timeout = 300 * time.Second
	}

	work, err := os.MkdirTemp("", "ai-repro-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(work)

	if err := os.WriteFile(filepath.Join(work, "policy.yaml"), []byte(bundle.PolicyYAML), 0o600); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(work, "resource.yaml"), []byte(bundle.ResourceYAML), 0o600); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(work, "run.sh"), []byte(reproRunScript), 0o700); err != nil {
		return nil, err
	}

	args := r.reproDockerArgs(image, work)
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	out, err := exec.CommandContext(cctx, "docker", args...).CombinedOutput()
	logs := tail(string(out), 4096)
	actual := parseActualBehavior(string(out))
	if cctx.Err() == context.DeadlineExceeded {
		return &ReproResult{ActualBehavior: "error", Success: false, Logs: "timeout\n" + logs}, fmt.Errorf("repro exceeded %s", timeout)
	}
	if err != nil && actual == "error" {
		return &ReproResult{ActualBehavior: "error", Success: false, Logs: err.Error() + "\n" + logs}, err
	}
	success := expectedMatches(bundle.ExpectedBehavior, actual)
	return &ReproResult{ActualBehavior: actual, Success: success, Logs: logs}, nil
}

func (r *Runner) reproDockerArgs(image, workDir string) []string {
	// KinD needs to talk to a local docker/cache; --network=none is not viable
	// for cluster bring-up. We still never reach the public internet: the
	// script refuses to pull, and the image cache is mounted read-only.
	// When no cache is configured, fall back to --network=none + kyverno CLI
	// (image must already be on the host; docker cannot pull).
	args := []string{"run", "--rm"}
	if r.ImageCache == "" {
		args = append(args, "--network=none")
	}
	args = append(args,
		"--cpus=4", "--memory=8g", "--memory-swap=8g", "--pids-limit=2048",
		"--read-only", "--tmpfs", "/tmp:size=2g",
		"-v", workDir+":/repro:ro",
		"-w", "/repro",
		"--label", "ai-maintainer-run=true",
	)
	if r.ImageCache != "" {
		args = append(args, "-v", r.ImageCache+":/image-cache:ro")
	}
	args = append(args, image, "/bin/sh", "/repro/run.sh")
	return args
}

func parseActualBehavior(out string) string {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "ACTUAL_BEHAVIOR=") {
			return strings.TrimPrefix(line, "ACTUAL_BEHAVIOR=")
		}
	}
	return "error"
}

func expectedMatches(expected, actual string) bool {
	if actual != "admitted" && actual != "blocked" {
		return false
	}
	e := strings.ToLower(expected)
	wantBlock := containsAny(e, "den", "block", "reject", "not admit", "fail", "violat")
	wantAdmit := containsAny(e, "allow", "admit", "creat", "pass", "succeed")
	if wantBlock && !wantAdmit {
		return actual == "blocked"
	}
	if wantAdmit && !wantBlock {
		return actual == "admitted"
	}
	return true
}

func containsAny(s string, needles ...string) bool {
	for _, n := range needles {
		if strings.Contains(s, n) {
			return true
		}
	}
	return false
}
