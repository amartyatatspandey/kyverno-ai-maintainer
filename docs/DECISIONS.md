# DECISIONS.md — architecture decisions and rejected alternatives

## D-007 (Phase 9): POC scope = one composite primary flow (Dependabot PR → analysis → scoped tests → policy gate → merge/flag → audit) + triage-lite secondary; W5 repro dropped to roadmap
The composite flow alone exercises all seven pillars. W5 dropped despite stretch status: heaviest build, and its unique demo value (sandbox eats hostile input) is covered by sandboxed scoped tests + live injection test in triage. Full scope, demo plan, and deliverables in POC_SCOPE.md.

## D-006 (Phase 8): Sandbox = ephemeral Docker container per run; DinD-nested KinD for cluster runs; zero credentials inside
Host-socket KinD rejected (siblings ≠ sandbox); K8s Jobs and VM-per-run are production roadmap, not POC. Privileged DinD weakness stated openly, compensated by credential-free design + Docker Desktop VM interposition + unprivileged unit-test runs. Full design in SANDBOX.md.

## D-005 (Phase 7): Policy engine = deterministic Go package + `.github/ai-maintainer.yaml`, not OPA
~10 predicates over one context object don't justify a Rego runtime, a second language, and input marshalling — while the hard guarantees (fetch-fresh context, SHA-bound decisions, fail-closed tool wiring) live in Go regardless. `Evaluate(action, ctx) → Decision` is OPA-shaped so a RegoEngine is a drop-in if policy complexity grows. Full design in POLICY_ENGINE.md. Rejected: OPA/Rego now (premature), policy-in-system-prompt (thesis violation), hardcoded rules without YAML (maintainers couldn't tune without recompiling).

**Dependency exception:** `github.com/modelcontextprotocol/go-sdk` was added for `assistant mcp`. An SDK that implements a real, evolving external protocol is a legitimate exception to "boring Go, one dep" in a way an agent-orchestration framework would not be. Mutating MCP calls still terminate in `Evaluate`.

## D-004 (Phase 5): Single-agent, event-per-run, deterministic-pipeline-with-LLM-steps

**Decision.** One agent process. Each triggering event (Dependabot PR, target PR for scoped tests, new issue) starts one bounded, stateless **run**:

```
trigger → deterministic context assembly → agent loop (LLM ⇄ MCP tools, budgeted)
        → proposed actions → policy engine (ALLOW/DENY per action) → execute (GitHub / sandbox)
        → audit record
```

- **Trigger mechanism:** polling sweep + manual CLI trigger + webhook adapter on the same run entrypoint. `poll` mode lists open Dependabot PRs / new issues since last run; `run --pr N` triggers one run for the demo; `assistant serve` accepts GitHub `pull_request` / `issues` webhooks (HMAC-SHA256, delivery-ID dedupe) and calls those same run methods. The adapter swap D-004 designed for has shipped.
- **Stateless runs.** All state the run needs is refetched from GitHub at start (also the fix for stale-state, RISKS P4). The only cross-run state: audit log + dedupe/rate-limit counters in a small local store.
- **The LLM sits at fixed insertion points, not at the top.** For W1: summarize changelog risk, classify breaking-change signals. For W3: propose *additional* suites beyond the deterministic selection, write the rationale comment. For W4: classify + draft templated comment. The surrounding pipeline (fetch, parse semver, compute import closure, gate, execute) is ordinary code. Within an insertion point the LLM has a bounded tool loop (read-only tools; budget caps per RISKS A5).

**Why this is the simplest architecture that still demonstrates the thesis.** The thesis is *LLM proposes / policy authorizes / sandbox executes* — that is a statement about **layering**, not about agent topology. One agent with real multi-step tool use (read PR → fetch diff → inspect map → run tests → propose actions) is fully "agentic" in the sense the challenge means (autonomous multi-step interaction with tools), while every additional agent would only add coordination surface without adding a pillar.

**Rejected alternatives.**
- **Multi-agent (triage agent + test agent + merge agent + reviewer agent).** Rejected: no workflow in scope needs concurrent specialized reasoning; inter-agent messages would become a new untrusted channel to police; debugging/audit becomes N-way. Nothing in the seven pillars requires it. Revisit only if workflows grow truly parallel (e.g. Q&A assistant running alongside).
- **Planner/executor split (LLM plans, separate executor replays plan).** Rejected as a *separate architecture*: the design already gets the safety benefit (plans can't self-execute) from the policy gateway; a distinct plan-replay engine duplicates that boundary with more machinery. The run loop *is* a degenerate planner/executor where the "executor" is the policy-gated action stage.
- **Pure event-driven webhook service.** Rejected as the *first* build (infrastructure, no thesis). The adapter has now shipped alongside polling: same entrypoint, HMAC-verified, in-memory delivery-ID dedupe (G3). Persistent dedupe storage remains a production follow-up.
- **Pure scheduled batch (cron only).** Insufficient: can't demo "PR opened → assistant reacts" narrative cleanly, and pushes latency to hours. Kept as *one* of the two trigger adapters (the sweep), not the architecture.
- **Free-roaming coding agent with shell access (OpenHands-style) wrapped in prompts.** Rejected per the core thesis and RISKS A3/A1: safety must not rest on prompting. Semantic MCP tools + allowlisted sandbox commands instead.

**Consequences.** (a) The POC binary is a CLI (`assistant poll`, `assistant run --pr N`, `assistant serve` for webhook ingress); (b) horizontal scaling and webhook latency are still out of POC scope — serve is a single-process adapter; (c) every run is independently replayable from its audit record, which Phase 11's eval harness reuses directly.

## D-001: Work against fork `amartyatatspandey/kyverno`, verify claims against upstream via `gh`
Local clone of the fork (synced to upstream main 2026-08-13) for file-level facts; live queries against `kyverno/kyverno` for PR/issue/CI statistics. Rejected: cloning upstream separately (redundant).

## D-003 (Phase 2, proposed): POC scope = W1 dependency-PR decisioning + W3 scoped test selection (primary), W4 triage-classification (stretch), W5 repro (stretch)
Grounded in measured gaps; W2/W6/W7/W8 dropped or deferred (duplication, no demand, or scope creep). See PROBLEM_MAP.md. Final scope commitment happens at the Phase 9 gate.

## D-002: Do not rebuild existing automation
PR branch updating, path-based labeling, prow slash commands, cherry-pick, milestone assignment, stale-branch cleanup all exist (NOTES.md §9). POC targets only verified gaps: Dependabot auto-merge decisioning, scoped test selection, issue triage/repro. Rejected: implementing the brief's full Phase-1 list verbatim.
