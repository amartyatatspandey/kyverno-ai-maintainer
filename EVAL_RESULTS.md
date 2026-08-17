# EVAL_RESULTS.md — measured results (Phase 16 run of the Phase 11 harness)

Command: `go run ./cmd/eval` in `assistant/`. Inputs: 50 real Dependabot PRs, 30 real merged code PRs from kyverno/kyverno history (`eval/*.json`). Policy config: `assistant/config/ai-maintainer.yaml`.

## W1 — dependency-PR policy replay (n=50)

| Metric | Result |
|---|---|
| Update-type classification | 25 patch, 16 minor, 4 major, 5 unknown (grouped/multi-dep titles → `unknown` → human) |
| Policy ALLOW / DENY | 21 / 29 |
| **Unsafe merges (ALLOW on a PR rejected on merit)** | **0 of 50** |
| Duplicate-work merges (see finding below) | 1 (#16768) |
| Deny reasons | `no_protected_paths` 20, `update_type_allowed` 9 |
| Automation coverage | 42% of dependency PRs decided without a human |

Evaluation is deliberately **charitable to the assistant**: historical CI state isn't recoverable, so every case is replayed as if checks were green. Any ALLOW that shouldn't have happened is therefore a genuine finding, never an artifact of missing data.

### Finding 1 — the ground-truth rule from Phase 11 was wrong (corrected)
EVAL_HARNESS.md originally said "CLOSED ⇒ humans rejected ⇒ expected DENY." Measuring exposed that of 9 closed Dependabot PRs:
- **4 were auto-closed by Kyverno's own `pr-rate-limiter.yaml`** (>8 open PRs per author — the workflow found in Phase 1),
- **3 were closed by Dependabot itself** (superseded by a newer version),
- **2 were closed by a maintainer.**

Only the last category is a quality judgment. Scoring against the naive rule reported 6 "false positives"; against the true rule it is 1. The cases file now carries a `closure_cause` field per case, and the harness scores accordingly.

The 4 rate-limiter closures are themselves evidence *for* the assistant: they were mergeable patch/minor bumps discarded because the queue backed up, which Dependabot then re-opens later — repeat work created purely by latency.

### Finding 2 — the one real miss is a duplicate-work gap, not a safety gap
#16768 (cel-go 0.29.2→0.30.0) was closed by @realshuting because a **human PR #16782 was landing the same bump as a CVE fix**. The assistant would have merged a correct change that did in fact ship — no unsafe outcome, but wasted/conflicting effort.

**Consequence:** the policy engine needs a `no_competing_pr` rule (deny when another open PR modifies the same dependency). Logged as a roadmap item and a new RISKS row; not silently patched to make the number look better.

## W3 — scoped test selection (n=30 merged code PRs)

| Metric | Result |
|---|---|
| Avg conformance suites selected | 20.1 of 52 |
| **Conformance compute reduction** | **61%** (vs. 3,068 job-minutes / 342 jobs baseline) |
| Full-suite fallbacks (unmapped paths) | 11 of 30 (37%) |
| PRs needing zero conformance suites | 5 |

Meets the ≥60% BASELINE.md target, but the honest read is that **37% fallback rate is the headline weakness**: the path→suite map covers the common areas and nothing else, and every unmapped path forces a full run by design (fail-safe). The measured 61% is therefore a floor — map coverage is the single highest-leverage improvement, and the fallback rate is the metric that tracks it.

Selection recall against historical failing suites (the T1 metric) is **not yet measured** — it needs per-commit check-run archaeology (EVAL_HARNESS.md pilot). Until it is, scoped selection stays advisory-only, exactly as specified.

## Injection pass (Phase 14): see INJECTION_RESULTS.md — 7/7 vectors contained.
