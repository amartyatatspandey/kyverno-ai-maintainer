# PROBLEM_MAP.md — Phase 2

One spec per candidate workflow: **trigger → inputs → decision logic → safe actions → required human checkpoint**. Safe/unsafe boundaries grounded in Phase 1 findings (NOTES.md). Each ends with a POC verdict: **PRIMARY / STRETCH / DEFER / DROP**.

---

## W1. Dependency PR handling — **PRIMARY**

*Gap verified:* no auto-merge exists; ~26% of main commits are hand-merged Dependabot batches (NOTES.md §2, §10).

- **Trigger:** `pull_request` webhook where author is `app/dependabot` (opened/synchronize), or scheduled sweep of open Dependabot PRs.
- **Inputs:** PR metadata (author, labels, base branch, mergeability), diff (`go.mod`/`go.sum`/workflow pins only?), semver delta parsed from Dependabot's structured PR title/metadata, dependency group (k8s/sigstore/otel per `dependabot.yml`), CI check states, release notes of the bumped dep (untrusted text).
- **Decision logic (LLM proposes):** classify update type (patch/minor/major), detect breaking-change signals in changelog, confirm diff touches only lockfile/manifest paths, summarize risk.
- **Deterministic authorization (policy engine decides):** author is Dependabot ∧ update ∈ {patch, minor} ∧ all required checks green ∧ no `hold`/`security`/`breaking-change` label ∧ changed files ⊆ {go.mod, go.sum, .github/**pinned-action lines} ∧ base = main ∧ rate limit not exceeded ∧ kill switch off. Major bumps or any deviation → comment summary + label, never merge.
- **Safe actions:** comment (risk summary), label, approve-style review, merge via the same squash path maintainers use (revertible via `git revert`), escalation comment.
- **Human checkpoint:** major bumps always; `hold` label stops everything; kill-switch label/variable; audit record per decision.

## W2. PR hygiene (rebase/update + stale nudges) — **DROP (rebuild) / DEFER (nudges)**

*Phase 1 contradiction:* branch updating is already automated by `pr-branch-updater.yml`; `updatedAt`-based staleness is unreliable because of it (NOTES.md §9, contradiction 1, 5).

- Rebuilding branch updates would duplicate live infra — dropped.
- Review-activity-based stale nudges (last human comment/review, not last push) are genuinely missing but low-demo-value; deferred to roadmap.
- **Human checkpoint if ever built:** nudge comments only, never close.

## W3. Scoped test selection — **PRIMARY**

*Gap verified:* all ~52 chainsaw suites × 3 k8s versions run per PR; the conformance composite action already exposes `tests-path`, `chainsaw-tests` regex, sharding, quarantine inputs (action.yaml:16–30) — but nothing computes the scope. No path→suite map exists.

- **Trigger:** PR opened/synchronized (or `/scoped-tests` comment for demo).
- **Inputs:** diff file list, Go import graph (`go list`-derived reverse deps), path→suite map (to be built in Phase 0 metadata work), `labels.yml` path globs (existing path→area knowledge), CODEOWNERS.
- **Decision logic:** deterministic core — changed paths → affected Go packages (reverse import closure) → unit-test packages; changed paths → chainsaw suites via map. LLM only fills gaps (novel paths not in map) and explains selection; LLM can *widen* the selection, never narrow below the deterministic result.
- **Safe actions:** run selected `go test` packages in sandbox; run selected chainsaw suites in sandboxed KinD; comment with selection + rationale + results; label (e.g. `scoped-tests-passed`). Never marks required checks — advisory alongside full CI in POC.
- **Human checkpoint:** results are advisory; a maintainer decides whether scoped-green is sufficient. Fallback rule: unmappable paths ⇒ full suite.

## W4. Issue triage (classify + label + missing-info) — **STRETCH (classification), sandbox repro is W5**

*Gap verified:* 51% of open issues sit in `triage`; templates give only coarse labels (NOTES.md §5).

- **Trigger:** `issues` webhook (opened), or scheduled sweep of `triage`-labeled backlog.
- **Inputs:** issue title/body/template fields (**untrusted**), label taxonomy from `labels.yml`, similar-issue search results, docs.
- **Decision logic:** LLM classifies (bug/feature/question + area/* label + affected component from template's version/component fields), checks template completeness, drafts missing-info request, finds probable duplicates.
- **Deterministic authorization:** allowed label set = labels that exist in `labels.yml` minus privileged ones (`good first issue`, milestone-ish, `security`); comment rate limit per issue; never close/assign; kill switch.
- **Safe actions:** add/remove area+type labels, one comment (classification rationale or missing-info request), link candidate duplicates.
- **Human checkpoint:** `triage` label is removed only by humans in POC (assistant proposes via comment); all labels revertible.

## W5. Automated reproduction — **STRETCH (only if time allows)**

- **Trigger:** issue classified as bug with complete repro (policy YAML + resource YAML + versions).
- **Inputs:** user-supplied YAML from the issue body — **maximally untrusted; this is the prompt-injection and resource-abuse hot path**.
- **Decision logic:** extract repro artifacts; deterministic validation gate: YAML parses, kinds ∈ allowlist (Kyverno policies + core resources), no `exec`-style constructs, size limits; then scripted repro (never free-form shell): fresh KinD → install pinned Kyverno version → apply → observe → diff actual vs expected.
- **Safe actions:** comment with evidence (logs, actual vs expected), apply `repro-confirmed`/`repro-failed` label.
- **Human checkpoint:** results advisory; sandbox is network-restricted, credential-free, time/CPU/mem-capped, destroyed after run.

## W6. Slack/Discussions Q&A — **DROP for POC**

No observable volume in-repo (Discussions low-traffic; Slack invisible — ASSUMPTIONS #5); needs a different trust surface (external users) and adds no new pillar beyond W4. Roadmap only.

## W7. Codegen/verify gate — **DROP (rebuild), fold detection into W3**

*Phase 1 contradiction:* `check-codegen.yaml` already runs `make codegen-all-code && make verify-codegen` on every PR and even uploads a fix patch artifact (NOTES.md §3). Rebuilding is pure duplication. What's missing is trivial: W3's analyzer flags "PR touches `api/` ⇒ expect codegen churn; generated files edited without source changes ⇒ warn" as part of its comment.

## W8. Docs-change detection + draft PRs — **DEFER**

Real need (AGENTS.md:203 requires a website-repo docs PR for feature changes) but the target is a *different repo* (kyverno/website), multiplying credential scope for little demo value. Roadmap: detect "feature PR lacks linked docs issue" and comment — that much could ride on W3's analysis later.

---

## Summary

| Workflow | Verdict | Why |
|---|---|---|
| W1 Dependency PRs | **PRIMARY** | Biggest measured toil (26% of commits), crisp policy story, exercises merge authority safely |
| W3 Scoped tests | **PRIMARY** | Real gap with existing CI hooks to plug into; showcases repo intelligence + sandbox |
| W4 Issue triage | **STRETCH** | Real backlog (51%), pure-reversible actions |
| W5 Repro | **STRETCH** | Best sandbox/injection demo, but heaviest to build |
| W2 PR hygiene | DROP/DEFER | Already automated / unreliable signal |
| W6 Q&A | DROP | No measurable in-repo demand |
| W7 Codegen gate | DROP | Fully exists in CI |
| W8 Docs drafts | DEFER | Cross-repo scope creep |

The two PRIMARY flows plus W4-lite (classification only) cover all seven pillars: reasoning (W1 risk summary, W3 gap-filling, W4 classification), MCP tools, repo intelligence (path→suite map, import graph), sandbox (W3 test execution), policy enforcement (W1 merge gate), real GitHub actions (comment/label/merge), audit + override (all).
