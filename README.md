# Kyverno AI Maintainer Assistant — POC

A model-agnostic, sandboxed, policy-constrained maintainer assistant for [kyverno/kyverno](https://github.com/kyverno/kyverno).

> **The LLM proposes. The policy engine authorizes. The sandbox executes. The audit log records. Humans retain control.**
> The model is never the security boundary.

## Quick start

```bash
cd assistant && go test ./...
```

```bash
cd assistant && go run ./cmd/assistant run --pr 17118 --repo kyverno/kyverno --dry-run
```

```bash
cd assistant && go run ./cmd/eval
```

Model provider is pluggable (BYOM): `AI_PROVIDER=anthropic|openai|stub`, `AI_MODEL=...`; `openai` also covers vLLM/Ollama/OpenRouter via `OPENAI_BASE_URL`. Default is a deterministic stub so tests and CI never need a key.

## What it does

**Primary flow** — Dependabot PR → deterministic update classification → advisory LLM risk summary → scoped test selection (path map + Go import closure) → sandboxed test run → policy gate → merge or flag-for-human → audit record.
**Secondary flow** — new issue → classify → allowlisted labels + one templated comment → audit record.

## Measured results

| | |
|---|---|
| Unsafe merges across 50 real historical Dependabot PRs | **0** |
| Conformance compute reduction across 30 real merged PRs | **61%** (vs 342 jobs / ~3,068 job-minutes) |
| Injection vectors contained | **7/7**, all asserted on policy decisions, not model refusals |
| Policy golden cases | 19, green |

Full detail — including a metric I had to correct and one genuine miss — in [EVAL_RESULTS.md](EVAL_RESULTS.md).

## Repository map

| Document | What it is |
|---|---|
| [NOTES.md](NOTES.md) | Phase 1 discovery: what the Kyverno repo actually contains, with citations |
| [PROBLEM_MAP.md](PROBLEM_MAP.md) | Per-workflow specs and what was dropped (and why) |
| [RISKS.md](RISKS.md) | 26 failure modes with likelihood/impact/detection/mitigation/residual |
| [TRUST_MODEL.md](TRUST_MODEL.md) | Trust zones, boundary-crossing rules, actor table |
| [DECISIONS.md](DECISIONS.md) | Architecture decisions + rejected alternatives (D-001…D-007) |
| [MCP_TOOLS.md](MCP_TOOLS.md) | The 13-tool capability surface, and what is deliberately absent |
| [POLICY_ENGINE.md](POLICY_ENGINE.md) | Authorization design + config schema |
| [SANDBOX.md](SANDBOX.md) | Isolation design, limits, and the hostile-input hot path |
| [POC_SCOPE.md](POC_SCOPE.md) | Final scope commitment |
| [BASELINE.md](BASELINE.md) | Current-state measurements + success metrics |
| [EVAL_HARNESS.md](EVAL_HARNESS.md) / [EVAL_RESULTS.md](EVAL_RESULTS.md) | Evaluation design and measured results |
| [AUDIT.md](AUDIT.md) / [OVERRIDE.md](OVERRIDE.md) | Audit records; the seven human-control mechanisms |
| [INJECTION_TESTS.md](INJECTION_TESTS.md) / [INJECTION_RESULTS.md](INJECTION_RESULTS.md) | Prompt-injection plan and results |
| [ARCHITECTURE.md](ARCHITECTURE.md) | Component/flow/sequence diagrams, permission model, self-critique |
| [ASSUMPTIONS.md](ASSUMPTIONS.md) / [QUESTIONS.md](QUESTIONS.md) | What's unverified; what needs a maintainer's answer |
| [DEMO_SCRIPT.md](DEMO_SCRIPT.md) / [VIDEO_CHECKLIST.md](VIDEO_CHECKLIST.md) | Demo narrative and submission checklist |

## Code

```
assistant/
  cmd/assistant/     CLI: run --pr N | --issue N, audit show|why|list, stop
  cmd/eval/          replays historical cases, prints the metrics
  internal/policy/   deterministic authorization (deny-by-default) + golden tests
  internal/intel/    path→suite map + reverse import closure
  internal/ghx/      the only credential-holding code; mutations require a Decision
  internal/sandbox/  ephemeral, credential-free containers
  internal/audit/    write-ahead JSONL + maintainer-readable renderer
  internal/llm/      BYOM provider abstraction (advisory outputs only)
  internal/runtime/  the run loop + comment templates + injection tests
  config/            ai-maintainer.yaml (policy), test-map.yaml (repo intelligence)
eval/                50 Dependabot PRs, 30 code PRs, 30 issues — real history
```

## Status

POC. Not deployed against upstream kyverno/kyverno; designed to run against a fork with a least-privilege GitHub App. Scoped test selection is advisory-only until selection recall is measured. Not built (designed only): sandboxed issue reproduction, webhook ingestion, MCP-over-protocol serving.
# kyverno-ai-maintainer
