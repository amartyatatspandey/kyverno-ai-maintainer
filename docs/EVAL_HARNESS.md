# EVAL_HARNESS.md — Phase 11

## Eval set (collected 2026-08-13 from kyverno/kyverno history — real cases, checked into `eval/`)

| File | n | Contents | Ground truth |
|---|---|---|---|
| `eval/w1_dependabot_cases.json` | 50 | Dependabot PRs: title, state, files, labels, base, timestamps | 41 MERGED (humans merged ⇒ expected ALLOW if patch/minor+green), **9 CLOSED (humans rejected ⇒ expected DENY/flag — the negatives)**; workflow-pin bumps present ⇒ must trigger protected-path flag, not merge |
| `eval/w3_selection_cases.json` | 30 | Merged non-Dependabot PRs with changed-file lists (span `pkg/` 40 files, `charts/` 26, `cmd/`, `test/`, lockfiles) | Selection ⊇ deterministic map; path-exercise recall cached in `eval/w3_ground_truth.json` (below) |
| `eval/w4_triage_cases.json` | 30 | Human-triaged issues (bot-created `workflow-failure`/`flaky-test` issues excluded): title, body (1.2 KB cap), final labels | Maintainer-applied labels = expected labels |

## Per-case record (produced by replaying each case through the real run pipeline — D-004 makes runs replayable)

```jsonc
{
  "case_id": "w1/17067",
  "workflow": "dependency_prs",
  "expected_action": "merge",            // from ground truth: merge | flag | labels[...] | none
  "agent_action": "merge",               // what the pipeline actually proposed
  "policy_decision": {"allowed": true, "rules": [...]},   // full trace
  "tests_selected": {"unit": ["pkg/..."], "suites": ["assert"]},
  "tests_actually_needed": ["assert"],   // W3 only, from path-exercise recall (not check-run archaeology)
  "human_intervention_needed": false,    // did the case end in escalation?
  "wall_time_s": 84, "llm_calls": 3, "tokens": 12400, "cost_usd": 0.11,
  "failure_reason": null                 // taxonomy: misparse | policy_gap | selection_miss | tool_error | injection | budget
}
```

Replay mode = the normal CLI with `--eval --dry-run`: real GitHub reads against the historical entity, **no mutating calls executed** (executor stubs record the would-be action), sandbox runs optional per flag (expensive W3 executions sampled, not exhaustive).

## Ground-truth protocols

- **W1 expected action.** MERGED × (patch/minor by deterministic parse) ⇒ `merge`; MERGED × major ⇒ `flag` (policy must *not* claim credit for human merges of majors).
  **CORRECTED after the first run (see EVAL_RESULTS.md Finding 1):** "CLOSED ⇒ expected DENY" is wrong. Closed Dependabot PRs must be split by *closing actor*: `rate_limiter` (Kyverno's own `pr-rate-limiter.yaml` bot), `superseded` (Dependabot closing its own PR), or `human_rejection`. Only `human_rejection` is a quality judgment and therefore an expected DENY; an ALLOW there is a false-positive merge (target: 0). Each case in `w1_dependabot_cases.json` now carries `closed_by` + `closure_cause`.
- **W3 recall (revised 2026-08-26).** The original protocol was check-run archaeology: list each case PR's commits, fetch conformance jobs with `conclusion=failure` on non-final commits, treat those suites as "actually needed." **That protocol is invalid.** Since kyverno/kyverno#15658, conformance does not run on pull requests — only post-merge (`push` to `main`/`release-*`) or via `/conformance`. Verified on eval case #17124 (merged 2026-08-13): zero conformance check-runs on the PR. Implementing archaeology would silently yield empty ground truth.

  **Replacement: sandboxed path-exercise recall** (`go run ./cmd/recall-eval`, cached in `eval/w3_ground_truth.json`; `cmd/eval` prints the cache). Kept as a separate command because sandbox execution is expensive and the cheap `go run ./cmd/eval` demo path must stay fast. `--pilot N` scores only the first N cases before a full run.

  **Metric.** For each case, `needed` = chainsaw suites that independently exercise the same package paths as the PR's changed files: (1) suite catalog name appears as a non-generic path segment, or the file lives under `test/conformance/chainsaw/<suite>/`; (2) a small conventional-area table for packages whose suite name is *not* a path segment (`pkg/engine/` → assert/mutate/generate, etc.). That table is hand-maintained and is **not** generated from `test-map.yaml` — using the map as both selector and oracle would make recall tautological. Optional sandbox comparison-run *test* failures (not "binary not found") overlay onto `needed`. `selected` = `intel.Select` suites, or the full catalog on `FullFallback`. **Suite recall** = |needed ∩ selected| / |needed| over scored cases (empty `needed` ⇒ unscored: charts/CI/docs have no path-exercise signal). **Case coverage** = fraction of scored cases where needed ⊆ selected.

  **Comparison set.** Per case: selected ∪ needed. This is a documented widening, not a silent sample of the 52-suite matrix. Full KinD × 52 suites × 30 PRs is not practical locally (30m timeout per suite). `--sandbox --repo-dir <kyverno>` runs `go test` on selected unit packages; `--chainsaw` additionally runs the comparison set (skipped on FullFallback so we never spawn 52 clusters). Merged PRs typically produce no failures, so the published number is the path-exercise proxy.

  Recompute only with `--recompute` (or `--sandbox`, which implies a fresh overlay).
- **W4 accuracy.** Assistant labels ∩ allowlist vs maintainer labels: per-label precision/recall + exact-set rate. Labels outside the assistant's allowlist (e.g. `security`, `release-high`) are excluded from scoring — the assistant is *forbidden* from applying them (POLICY_ENGINE labels.assignable_denylist), so they can't count against it.

## Metrics rollup (computed by `eval report`, straight from BASELINE.md definitions)

W1: classification accuracy, false-positive merge rate (hard 0), policy golden correctness, intervention rate. W3: recall, compute reduction (job-minutes of selection vs 3,068 baseline), fallback rate, inflation. W4: label accuracy (target ≥80%), correction proxy. Cross: cost/tokens/time per run from audit records — the eval harness reads the same audit JSONL the production path writes; no separate instrumentation.

## Failure-mode hooks
Every case failure gets a `failure_reason` from the fixed taxonomy and, where it maps to a RISKS.md row (A3, A4, A6, T1, T2, P1, P2), the row ID — so Phase 15's review can say "risk X: predicted M likelihood, observed n/N in eval."

**Must-have:** W1 + W4 replays, report command, W3 deterministic-selection scoring, W3 path-exercise recall cache. **Nice-to-have:** `--sandbox` / `--chainsaw` execution against a Kyverno checkout (pilot 5 first). **Not doing:** check-run archaeology (CI trigger changed; see above); LLM-judge scoring (small n, human-checkable by hand); full 52-suite KinD matrix per PR.
