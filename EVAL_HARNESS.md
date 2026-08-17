# EVAL_HARNESS.md — Phase 11

## Eval set (collected 2026-08-13 from kyverno/kyverno history — real cases, checked into `eval/`)

| File | n | Contents | Ground truth |
|---|---|---|---|
| `eval/w1_dependabot_cases.json` | 50 | Dependabot PRs: title, state, files, labels, base, timestamps | 41 MERGED (humans merged ⇒ expected ALLOW if patch/minor+green), **9 CLOSED (humans rejected ⇒ expected DENY/flag — the negatives)**; workflow-pin bumps present ⇒ must trigger protected-path flag, not merge |
| `eval/w3_selection_cases.json` | 30 | Merged non-Dependabot PRs with changed-file lists (span `pkg/` 40 files, `charts/` 26, `cmd/`, `test/`, lockfiles) | Selection ⊇ deterministic map; recall ground truth from historical check-runs (below) |
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
  "tests_actually_needed": ["assert"],   // W3 only, from check-run ground truth
  "human_intervention_needed": false,    // did the case end in escalation?
  "wall_time_s": 84, "llm_calls": 3, "tokens": 12400, "cost_usd": 0.11,
  "failure_reason": null                 // taxonomy: misparse | policy_gap | selection_miss | tool_error | injection | budget
}
```

Replay mode = the normal CLI with `--eval --dry-run`: real GitHub reads against the historical entity, **no mutating calls executed** (executor stubs record the would-be action), sandbox runs optional per flag (expensive W3 executions sampled, not exhaustive).

## Ground-truth protocols

- **W1 expected action.** MERGED × (patch/minor by deterministic parse) ⇒ `merge`; MERGED × major ⇒ `flag` (policy must *not* claim credit for human merges of majors).
  **CORRECTED after the first run (see EVAL_RESULTS.md Finding 1):** "CLOSED ⇒ expected DENY" is wrong. Closed Dependabot PRs must be split by *closing actor*: `rate_limiter` (Kyverno's own `pr-rate-limiter.yaml` bot), `superseded` (Dependabot closing its own PR), or `human_rejection`. Only `human_rejection` is a quality judgment and therefore an expected DENY; an ALLOW there is a false-positive merge (target: 0). Each case in `w1_dependabot_cases.json` now carries `closed_by` + `closure_cause`.
- **W3 recall.** For each case PR: list its commits; for each non-final commit fetch check-runs; collect conformance job names with `conclusion=failure` whose name maps to a suite (job names embed `tests-path`). Failing suites on intermediate commits = "suites that were actually needed." Recall = cases where selection ⊇ ≥1 such suite / cases with any such suite. API-heavy ⇒ computed once in Phase 16 and cached in `eval/w3_ground_truth.json`; pilot on 5 PRs first to validate job-name→suite parsing.
- **W4 accuracy.** Assistant labels ∩ allowlist vs maintainer labels: per-label precision/recall + exact-set rate. Labels outside the assistant's allowlist (e.g. `security`, `release-high`) are excluded from scoring — the assistant is *forbidden* from applying them (POLICY_ENGINE labels.assignable_denylist), so they can't count against it.

## Metrics rollup (computed by `eval report`, straight from BASELINE.md definitions)

W1: classification accuracy, false-positive merge rate (hard 0), policy golden correctness, intervention rate. W3: recall, compute reduction (job-minutes of selection vs 3,068 baseline), fallback rate, inflation. W4: label accuracy (target ≥80%), correction proxy. Cross: cost/tokens/time per run from audit records — the eval harness reads the same audit JSONL the production path writes; no separate instrumentation.

## Failure-mode hooks
Every case failure gets a `failure_reason` from the fixed taxonomy and, where it maps to a RISKS.md row (A3, A4, A6, T1, T2, P1, P2), the row ID — so Phase 15's review can say "risk X: predicted M likelihood, observed n/N in eval."

**Must-have:** W1 + W4 replays, report command, W3 deterministic-selection scoring. **Nice-to-have:** W3 sandbox-executed sampling, full 30-case check-run ground truth (pilot 5 first). **Not doing:** LLM-judge scoring (small n, human-checkable by hand).
