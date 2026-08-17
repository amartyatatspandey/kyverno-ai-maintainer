# VIDEO_CHECKLIST.md — Phase 18

Confirms the final video covers every required element. Tick each before submitting the cover-letter link.

| Required element | Where it lands | Evidence shown on screen | Done |
|---|---|---|---|
| Introduce yourself | 0:00–0:10 | — | ☐ |
| **The problem** | 0:00–0:25 | 21 dep-PRs/mo @ 9.3h median; 51% issues in `triage`; 342 jobs / 3,068 job-minutes per conformance run | ☐ |
| **The insight — why an agent helps** (and why existing automation doesn't) | 0:25–0:50 | NOTES.md §9 automation inventory; the three verified gaps | ☐ |
| **Architecture** (LLM / agent / MCP-style tool layer / policy / sandbox / GitHub / KinD / audit) | 0:50–1:20 | layered diagram from ARCHITECTURE.md | ☐ |
| **Safety model — why the model isn't trusted alone** | 0:50–1:20 + 3:00–3:30 | thesis statement; injection run; `merge_pr` unreachable from triage | ☐ |
| **Live demo** | 1:20–3:30 | ALLOW run (merge), DENY run (protected path), injection, kill switch | ☐ |
| **Technical depth on key decisions** | throughout | protected-paths-beats-allowlist ordering; title-not-body parsing; SHA-bound decisions; deterministic selection the LLM may only widen | ☐ |
| **Measured evidence** | 3:30–4:10 | 0 unsafe merges / 50 PRs; 61% compute reduction / 30 PRs; 7/7 injection vectors; **plus the corrected-metric story** | ☐ |
| **Future roadmap** | 4:10–4:40 | repro harness, webhooks, map coverage (37% fallback), `no_competing_pr` rule, VM isolation, MCP-over-protocol | ☐ |
| Runs 2–5 minutes | — | target 4:40 | ☐ |
| Link in cover letter (Google Drive or YouTube) | — | — | ☐ |

## Claims allowed on camera (measured only)
21 Dependabot PRs/month · 9.3h median merge latency · 82% patch/minor · 342 jobs / 3,068 job-minutes · 51% of open issues labeled `triage` · 67% of recent issues with zero comments · 0 unsafe merges in 50-PR replay · 61% conformance compute reduction in 30-PR replay · 37% full-suite fallback rate · 7/7 injection vectors contained · 19 policy golden cases green.

**Not allowed on camera** (unverified or estimated): maintainer minutes saved per PR; time-to-merge improvement in production (never run in production); selection *recall* (not yet measured — say "advisory until recall is measured"); any claim that upstream has adopted this.

## Pre-flight
- [ ] `go test ./...` green on the recording machine
- [ ] `go run ./cmd/eval` reproduces the numbers quoted
- [ ] fork has the GitHub App installed, `ai-maintainer.yaml` committed, `ai-hold`/`ai-reviewed` labels created
- [ ] a patch-bump PR and a workflow-pin PR exist on the fork (or pre-recorded fallback ready)
- [ ] secrets not visible in terminal scrollback (`gh auth status` output, env)
