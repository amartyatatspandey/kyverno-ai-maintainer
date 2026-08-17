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
| **Breadth beyond the two PRIMARY flows** (optional, 4:10–4:25) | after measured evidence | 10 more workflows on the same substrate — one quick live beat (e.g. `dco_check` or `policy_lint`) is enough; do not let this displace the W1/W3 spine | ☐ |
| **Future roadmap** | 4:25–4:50 | webhooks, map coverage (37% fallback), `no_competing_pr` rule, VM isolation, MCP-over-protocol, security-advisory triage & auto-backport (**designed, deliberately not built** — say why) | ☐ |
| Runs 2–5 minutes | — | target 4:50 | ☐ |
| Link in cover letter (Google Drive or YouTube) | — | — | ☐ |

## Claims allowed on camera (measured only)
21 Dependabot PRs/month · 9.3h median merge latency · 82% patch/minor · 342 jobs / 3,068 job-minutes · 51% of open issues labeled `triage` · 67% of recent issues with zero comments · 0 unsafe merges in 50-PR replay · 61% conformance compute reduction in 30-PR replay · 37% full-suite fallback rate · 7/7 original injection vectors contained (+10 more added alongside the 10 new workflows) · 204 automated test cases green · 13 workflows on one policy/audit/ghx substrate, only 1 of which can merge.

**Not allowed on camera** (unverified or estimated): maintainer minutes saved per PR; time-to-merge improvement in production (never run in production); selection *recall* (not yet measured — say "advisory until recall is measured"); any claim that upstream has adopted this; any claim that the 10 additional workflows have run against real GitHub data the way W1/W3 have (they're policy/injection-tested, not eval-replayed against historical cases the way W1/W3 are).

## Pre-flight
- [ ] `go test ./...` green on the recording machine (204 cases)
- [ ] `go run ./cmd/eval` reproduces the numbers quoted (W1/W3 only — the 10 additional workflows aren't in the eval harness)
- [ ] fork has the GitHub App installed, `ai-maintainer.yaml` committed, `ai-hold`/`ai-reviewed` labels created
- [ ] a patch-bump PR and a workflow-pin PR exist on the fork (or pre-recorded fallback ready)
- [ ] if demoing a Tier-1 workflow: `maintainer_digest.digest_issue_number` in `ai-maintainer.yaml` points at a real pinned issue on the fork, not the placeholder
- [ ] secrets not visible in terminal scrollback (`gh auth status` output, env)
