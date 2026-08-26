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
cd assistant && go run ./cmd/assistant run --pr 17118 --workflow dco_check --repo kyverno/kyverno --dry-run
```

```bash
cd assistant && go run ./cmd/eval
```

`go run ./cmd/assistant` also exposes `digest`, `flaky-report`, and `draft-release-notes` as standalone subcommands (no `--pr`/`--issue` target), and `run --issue N --repro` / `run --discussion N` for the two other non-PR-shaped workflows — see `assistant run -h` or the [Code](#code) tree below for the full list.

Model provider is pluggable (BYOM): `AI_PROVIDER=anthropic|openai|stub`, `AI_MODEL=...`; `openai` also covers vLLM/Ollama/OpenRouter via `OPENAI_BASE_URL`. Default is a deterministic stub so tests and CI never need a key.

## What it does

**Primary flow** — Dependabot PR → deterministic update classification → advisory LLM risk summary → scoped test selection (path map + Go import closure) → sandboxed test run → policy gate → merge or flag-for-human → audit record.
**Secondary flow** — new issue → classify → allowlisted labels + one templated comment → audit record.

Thirteen workflows total, all running through the same policy/audit/ghx substrate — nothing below is a separate codebase, it's the same architecture applied to a different action:

| Workflow | What it does | Github ops |
|---|---|---|
| `dependency_prs` | Dependabot PR → classify → scoped tests → merge or flag | comment, label, **merge** |
| `scoped_tests` | Path-map + import-closure test selection (advisory) | comment, label |
| `issue_triage` | Classify new issues, apply allowlisted labels | comment, label |
| `dco_check` | Flag commits missing a matching `Signed-off-by` trailer | comment |
| `welcome_bot` | Point first-time contributors at CONTRIBUTING.md | comment |
| `reviewer_suggest` | Suggest reviewers from CODEOWNERS + git-log frequency | comment |
| `maintainer_digest` | Weekly PR-aging / triage-backlog / CI-health dashboard | comment |
| `release_notes_draft` | Draft a changelog from merged PR titles/labels (local file, no GitHub write) | — |
| `policy_lint` | Run `kyverno apply`/`test` in-sandbox on community policy YAML | comment, label |
| `flaky_detection` | Flag flaky (not just failing) chainsaw suites; never auto-quarantines | comment |
| `docs_gap_detection` | Flag user-facing changes missing a website-repo docs pointer | comment |
| `discussion_qa` | Answer GitHub Discussions from a local docs index, dual-gated on model confidence *and* deterministic retrieval score | comment |
| `issue_repro` | Extract + validate issue-body YAML, then run a scripted sandbox reproduction | comment, label |

Only `dependency_prs` can merge. Everything else is comment/label only — the merge path is exactly as narrow as it was in the original POC; growth happened around it, not to it. Two workflows are deliberately **designed but not built** — see [SECURITY_ADVISORY_TRIAGE.md](docs/SECURITY_ADVISORY_TRIAGE.md) and [AUTO_BACKPORT.md](docs/AUTO_BACKPORT.md) for why (severity judgment and release-branch writes are higher-consequence surfaces than this pass's risk budget covers).

## Measured results

| | |
|---|---|
| Unsafe merges across 50 real historical Dependabot PRs | **0** |
| Conformance compute reduction across 30 real merged PRs | **61%** (vs 342 jobs / ~3,068 job-minutes) |
| Injection vectors contained | **7/7** (original pass) **+ 10** (W5/W6/W8 additions), all asserted on policy decisions, not model refusals |
| Automated test cases | **204**, green (`go test ./...`), including 84 policy-layer golden/injection cases across all 13 workflows |

Full detail — including a metric I had to correct and one genuine miss — in [EVAL_RESULTS.md](docs/EVAL_RESULTS.md).

## Repository map

Design, eval, and discovery docs live in [`docs/`](docs/).

| Document | What it is |
|---|---|
| [problem_description.md](docs/problem_description.md) | Original problem statement and proposed scope |
| [NOTES.md](docs/NOTES.md) | Phase 1 discovery: what the Kyverno repo actually contains, with citations |
| [PROBLEM_MAP.md](docs/PROBLEM_MAP.md) | Per-workflow specs and what was dropped (and why) |
| [RISKS.md](docs/RISKS.md) | 26 failure modes with likelihood/impact/detection/mitigation/residual |
| [TRUST_MODEL.md](docs/TRUST_MODEL.md) | Trust zones, boundary-crossing rules, actor table |
| [DECISIONS.md](docs/DECISIONS.md) | Architecture decisions + rejected alternatives (D-001…D-007) |
| [MCP_TOOLS.md](docs/MCP_TOOLS.md) | The 13-tool capability surface, and what is deliberately absent |
| [POLICY_ENGINE.md](docs/POLICY_ENGINE.md) | Authorization design + config schema |
| [SANDBOX.md](docs/SANDBOX.md) | Isolation design, limits, and the hostile-input hot path |
| [POC_SCOPE.md](docs/POC_SCOPE.md) | Final scope commitment |
| [BASELINE.md](docs/BASELINE.md) | Current-state measurements + success metrics |
| [EVAL_HARNESS.md](docs/EVAL_HARNESS.md) / [EVAL_RESULTS.md](docs/EVAL_RESULTS.md) | Evaluation design and measured results |
| [AUDIT.md](docs/AUDIT.md) / [OVERRIDE.md](docs/OVERRIDE.md) | Audit records; the seven human-control mechanisms |
| [INJECTION_TESTS.md](docs/INJECTION_TESTS.md) / [INJECTION_RESULTS.md](docs/INJECTION_RESULTS.md) | Prompt-injection plan and results |
| [ARCHITECTURE.md](docs/ARCHITECTURE.md) | Component/flow/sequence diagrams, permission model, self-critique |
| [ASSUMPTIONS.md](docs/ASSUMPTIONS.md) / [QUESTIONS.md](docs/QUESTIONS.md) | What's unverified; what needs a maintainer's answer |
| [DEMO_SCRIPT.md](docs/DEMO_SCRIPT.md) | Demo narrative |
| [SECURITY_ADVISORY_TRIAGE.md](docs/SECURITY_ADVISORY_TRIAGE.md) / [AUTO_BACKPORT.md](docs/AUTO_BACKPORT.md) | Design-only specs — why these two stay unbuilt this pass |

## Code

```
assistant/
  cmd/assistant/     CLI: run --pr N | --issue N [--repro] | --discussion N, digest, flaky-report,
                     draft-release-notes, serve (webhook), audit show|why|list, stop
  cmd/eval/          replays historical cases, prints the metrics
  internal/policy/   deterministic authorization (deny-by-default) + golden tests, 13 workflows
  internal/intel/    path→suite map, reverse import closure, CODEOWNERS/git-log reviewer suggest,
                     flaky-suite detection, docs-gap detection, local TF-IDF docs index
  internal/ghx/      the only credential-holding code; mutations require a Decision (REST + Discussions GraphQL)
  internal/sandbox/  ephemeral, credential-free containers (unit tests, policy lint, KinD/CLI repro)
  internal/repro/    W5's security boundary — untrusted issue YAML extraction + allowlist validation
  internal/audit/    write-ahead JSONL + maintainer-readable renderer
  internal/llm/      BYOM provider abstraction (advisory outputs only; incl. dual-gated Q&A grounding)
  internal/runtime/  the run loop + comment templates + injection tests
  internal/webhook/  GitHub webhook adapter (HMAC-SHA256, delivery-ID dedupe) → same run entrypoint
  config/            ai-maintainer.yaml (policy), test-map.yaml (repo intelligence)
eval/                50 Dependabot PRs, 30 code PRs, 30 issues — real history
```

## Status

POC. Not deployed against upstream kyverno/kyverno; designed to run against a fork with a least-privilege GitHub App. Scoped test selection is advisory-only until selection recall is measured. Automated issue reproduction (W5) moved from designed-only to built and tested this pass — see [SECURITY_ADVISORY_TRIAGE.md](docs/SECURITY_ADVISORY_TRIAGE.md) / [AUTO_BACKPORT.md](docs/AUTO_BACKPORT.md) for the two that deliberately stayed design-only. Webhook ingestion ships as `assistant serve` (HMAC-verified adapter on the same run entrypoint as CLI/poll). Not built: MCP-over-protocol serving (in-process Go interfaces for now).
