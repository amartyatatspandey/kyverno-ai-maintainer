# BASELINE.md — Phase 10 (current state, measured 2026-08-13 against kyverno/kyverno)

## Measured numbers

### Dependency PRs (primary flow target)
- Last **100 merged Dependabot PRs** span 2026-03-20 → now (≈ **21 per month**).
- Merge latency: **median 9.3 h**, p25 2.3 h, p75 30.3 h, max 1,683 h (70 days). Merges are manual, often daily batches by 2–3 maintainers (NOTES.md §2).
- Semver split (parsed from titles): **82% patch/minor, 9% major, 9% unparsed** (grouped/multi-dep titles). → The auto-merge-addressable share is ~80%+ of ~21 PRs/mo ≈ **17 PRs/mo**.
- Maintainer hands-on time per dep PR: **estimated** 2–10 min (open, scan diff/CI, merge) — not directly measurable from history; at 5 min avg ≈ **1.4 h/mo direct**, plus batching/context-switch overhead. (ASSUMPTIONS #3 — mentors to confirm.)

### CI cost (scoped-testing target)
- One full successful conformance run on upstream: **342 jobs, ≈3,068 job-minutes (~51 CPU-hours) of runners** (run 23287428030; jobs API summed). Wall time ~10 min thanks to massive parallelism — the cost is compute, not latency.
- Unit tests: single job, ~10 min (NOTES.md §3).
- This runs on PR events to main/release branches; a go.mod patch bump pays the same ~3,068 job-minutes as an engine rewrite.
- **Plausible scoped-run estimate** (to be *measured* in Phase 11, not asserted): dependency bumps and single-area code PRs plausibly map to ≤10 of 52 suites → **~70–85% job-minute reduction** for the majority class of PRs. Lockfile-only bumps arguably need even less (unit + smoke suite).

### Issue triage (secondary flow target)
- **270 open issues; 138 (51%) still labeled `triage`** (NOTES.md §5).
- Of the 30 most recent issues (since 2026-06-01): **20 (67%) have zero comments**; the 10 with responses show median 0.7 h / p75 3.6 h to first comment — *caveat:* first comments include the behaviorbot welcome bot, so true human first-response is slower than this reads; the 67%-silent figure is the honest signal.
- Issue templates guarantee coarse labels only (`bug`/`triage`); area/component labeling and completeness checks are human work.

### PR hygiene (excluded from POC — for the record)
- Branch updating automated (`pr-branch-updater.yml`); 293 open PRs; `updatedAt`-staleness meaningless under auto-updates (NOTES.md contradiction 5).

## Success metrics (defined now, before any implementation claims)

### W1 — dependency flow
| Metric | Definition | Target |
|---|---|---|
| Update-type classification accuracy | patch/minor/major vs. ground truth on eval set (Phase 11) | ≥ 98% (it's deterministic parsing; errors ⇒ unparsed ⇒ human) |
| False-positive merge rate | evals where policy ALLOWed a merge a maintainer would have blocked | **0** on eval set — any instance is a design bug |
| Policy golden-case correctness | table-driven policy tests | 100% (hard gate on shipping) |
| Time-to-merge, eligible bumps | PR open → merge in assistant-on operation | < 1 h (vs 9.3 h median baseline) |
| Human intervention rate | eligible PRs escalated anyway / total eligible | measured + reported (no target; honesty metric) |

### W3 — scoped tests
| Metric | Definition | Target |
|---|---|---|
| Selection recall | historical PRs with a conformance failure: did selection include ≥1 actually-failing suite? | ≥ 90% measured; report per-area breakdown |
| Compute reduction | job-minutes selected / 3,068 baseline, per eval PR class | ≥ 60% median (est. 70–85%; measure decides) |
| Full-suite fallback rate | runs where unmapped files forced full selection | measured (map-coverage health) |
| Selection-size inflation | LLM-widened vs deterministic-only size | measured (detects A6) |

### W4 — triage-lite
| Metric | Definition | Target |
|---|---|---|
| Label accuracy | area/type labels vs maintainer-applied ground truth on historical issues | ≥ 80% |
| First-response coverage | new issues receiving a substantive classification comment | 100% of processed issues (vs ~33% any-comment today) |
| Human correction rate | assistant labels later changed by humans | measured (honesty metric) |

### Cross-cutting
| Metric | Definition |
|---|---|
| Cost per run | LLM calls, tokens, $ estimate, wall time — from audit records |
| Actions per run | mutating actions, all policy-traced; DENY rate per rule (P2 detector) |
| Injection resistance | Phase 14 suite: 0 policy-crossing actions from injected content (hard gate) |

## Claim discipline
Numbers above marked *estimated* stay out of the video unless Phase 11 converts them to measurements; the demo cites: 21 dep-PRs/mo at 9.3 h median latency, 3,068 job-minutes per conformance run, 51% triage backlog, 67% zero-comment issues.
