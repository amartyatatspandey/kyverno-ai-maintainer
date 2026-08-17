# POLICY_ENGINE.md — Phase 7

## Mechanism: deterministic Go package + YAML config (not OPA)

A single Go package (`policy`) with one entrypoint:

```go
// Evaluate is the only path to an ALLOW. Deny-by-default:
// zero-value Decision is a denial; unknown action types are denied.
func (e *Engine) Evaluate(a Action, ctx Context) Decision

type Action struct {
    Type   string         // "merge_pr" | "comment" | "set_labels" | "run_scoped_tests" | "reproduce_issue"
    Target string         // "pr/17123", "issue/16992"
    Params map[string]any // schema-validated per type
}

type Context struct { // assembled FRESH by the engine itself (fetch-fresh rule, RISKS P4)
    Repo         string
    PR           *PRFacts   // author, authorType, base, headSHA, labels, checks, changedFiles, updateType
    Issue        *IssueFacts
    RunID        string
    Counters     Counters   // today's merges/comments/labels per entity + global
    Config       Config     // parsed ai-maintainer.yaml
}

type Decision struct {
    Allowed   bool
    Rules     []RuleResult // every rule evaluated, pass/fail + reason — full trace, not just verdict
    BoundSHA  string       // ALLOW is valid only for this head SHA
    ExpiresAt time.Time    // short TTL; executor re-calls Evaluate if expired
}
```

**Why Go, not OPA/Rego (D-005):** the whole rule set is ~10 predicates over one context object. Rego would add a runtime, a second language for reviewers, and an input-marshalling layer — while the hard guarantees here (fetch-fresh context, SHA binding, fail-closed wiring into tools) live *outside* the policy language either way, in Go. OPA earns its keep when policies are numerous, hot-swapped, or authored by non-developers; none applies to a POC with 4 action types. The `Evaluate(action, ctx)` interface is OPA-shaped on purpose: a `RegoEngine` implementing the same interface is a drop-in later. Boring deterministic security code is the point.

**Enforcement invariants (code, not config):**
1. Mutating MCP tools require a non-expired Decision matching their action + SHA; the GitHub client is constructed only inside the executor, so no other code path holds credentials (RISKS P3).
2. Deny-by-default: unknown action, unknown workflow, missing config section, config parse error ⇒ DENY (and for config errors: global halt).
3. Kill switch is checked inside `Evaluate` *and* by the executor immediately before each side effect (P5).
4. Every Decision (ALLOW and DENY) is written to the audit log with its full rule trace before the action executes.
5. The policy package has its own table-driven test suite with golden cases per rule (P1); a CI meta-test enumerates registered MCP tools and asserts each mutating one maps to a known action type.

## Configuration: `.github/ai-maintainer.yaml`

Human-owned rules; changing them requires a commit (i.e., maintainer review). POC reads it from the target repo's default branch at run start (pinned per run).

```yaml
version: 1

# ---- global controls ----
enabled: true                    # master enable
kill_switch:
  label: "ai-hold"               # this label on any PR/issue freezes actions on it
  repo_variable: "AI_MAINTAINER_PAUSED"   # set to "true" → global halt (checked every run + before every action)

rate_limits:
  merges_per_day: 10
  comments_per_entity_per_day: 2
  label_ops_per_entity_per_day: 4
  sandbox_runs_per_day: 20

# ---- per-workflow enable/disable ----
workflows:
  dependency_prs:   { enabled: true }
  scoped_tests:     { enabled: true, advisory_only: true }   # advisory_only: results never gate anything
  issue_triage:     { enabled: true }
  reproduction:     { enabled: false }                        # stretch, off by default

# ---- branch & path protection ----
branches:
  merge_targets_allowed: ["main"]        # merge_pr may only target these
  push_allowed: []                        # no push capability exists in POC; empty by declaration

protected_paths:                          # any changed file matching ⇒ deny auto-merge, flag security-sensitive
  - "api/kyverno/v1/**"
  - "pkg/cosign/**"
  - "pkg/notary/**"
  - ".github/workflows/**"
  - "charts/**"
  - "SECURITY.md"
  - "DEPENDENCY-POLICY.md"

generated_paths:                          # informational classification (W3 comment); never hand-editable
  - "pkg/client/**"
  - "config/crds/**"
  - "**/zz_generated*.go"

# ---- W1: auto-merge conditions ----
auto_merge:
  allowed_authors: ["app/dependabot"]     # author *type* Bot verified in code, not just login string
  update_types: ["patch", "minor"]        # parsed deterministically from Dependabot metadata/title
  require:
    checks: all_required_green            # required-check list read from GitHub API at eval time
    mergeable: true
    base: "main"
  changed_files_must_match:               # diff ⊆ these globs, else deny
    - "go.mod"
    - "go.sum"
    - "hack/*/go.mod"
    - "hack/*/go.sum"
    - ".github/workflows/*.ya?ml"         # actions-pin bumps; still subject to protected_paths? NO —
                                          # see rule ordering below: protected_paths wins ⇒ workflow bumps
                                          # are flagged, not auto-merged. Deliberate.
  deny_labels: ["hold", "ai-hold", "security", "breaking-change", "do-not-merge"]
  min_age_hours: 0                        # optional supply-chain cooldown; 0 for demo, recommend 24 upstream
  method: squash

# ---- allowed GitHub operations per workflow (allowlist) ----
github_ops:
  dependency_prs: ["comment", "set_labels", "merge_pr"]
  scoped_tests:   ["comment", "set_labels"]
  issue_triage:   ["comment", "set_labels"]
  reproduction:   ["comment", "set_labels"]

labels:
  assignable_denylist: ["security", "good first issue", "cherry-pick-*", "milestone-*"]
  never_remove: ["hold", "ai-hold", "triage", "security"]

# ---- sandbox command policy ----
commands:                                  # exact templates; no free-form shell exists
  - id: unit_tests
    template: "go test -timeout {timeout} {packages}"
    max_timeout: "20m"
  - id: chainsaw_suite
    template: "chainsaw test --test-dir test/conformance/chainsaw/{suite}"
    max_timeout: "30m"
  - id: repro_script
    template: "scripts/repro.sh {kyverno_version} {k8s_version}"   # fixed script, args validated
    max_timeout: "20m"
sandbox:
  cpu: "4", memory: "8Gi", disk: "20Gi"
  network_egress: ["proxy.golang.org", "registry.k8s.io", "ghcr.io"]  # setup phase only; none during repro
```

## Rule ordering (evaluated in this order; first DENY wins, ALLOW requires full pass)

1. Config valid & version supported, else **global halt**
2. `enabled` ∧ workflow enabled ∧ kill switch off (variable + label)
3. Action type ∈ `github_ops[workflow]` (or command id ∈ `commands` for sandbox)
4. Branch rules (merge target allowlist)
5. **Protected paths** — overrides everything below it (so a workflow-pin bump is *flag for human*, never auto-merge, even though `changed_files_must_match` would admit it)
6. Action-specific conditions (auto-merge block; label allow/deny lists; comment template exists)
7. Security-sensitive detection: changed files ∩ protected_paths, or Dependabot group ∈ {sigstore} ⇒ attach `security-review` escalation regardless of other outcomes
8. Rate limits (per-entity, then global)
9. Freshness: context age < TTL, head SHA unchanged ⇒ bind Decision to SHA

## Human override surface (summary; Phase 13 implements)

| Control | Effect | Latency |
|---|---|---|
| Repo variable `AI_MAINTAINER_PAUSED=true` | Global halt (no new runs, in-flight runs stop before next side effect) | ≤ one action boundary |
| `ai-hold` / `hold` label on entity | Freezes all actions on that PR/issue | Next Evaluate call |
| `workflows.<name>.enabled: false` | Disables one workflow repo-wide | Next run (config re-read) |
| Suspend GitHub App installation | Hard platform-level stop (break-glass) | Immediate, out-of-band |
| `git revert <merge SHA from audit record>` | Undo any merge | Human speed |

**POC must-have vs demo framing:** must-have = engine + invariants 1–4 + auto_merge/labels/kill-switch/rate-limit rules + golden tests. Nice-to-have = min_age cooldown, per-entity counters persisted across restarts. Demo theater (harmless but not load-bearing): rendering the full rule trace in the PR comment — actually useful for the video's "policy visibly does work" requirement.
