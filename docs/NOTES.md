# NOTES.md — Phase 1 Discovery (Kyverno repo inspection)

Repo inspected: local clone at `kyverno/` (fork `amartyatatspandey/kyverno`, HEAD `664adfb1` = upstream main as of 2026-08-13). Live-repo data queried via `gh` against `kyverno/kyverno` on 2026-08-13.

## 1. Repo structure

- Single Go module `github.com/kyverno/kyverno` (`go.mod` at root). Not a multi-module monorepo, but depends on external Kyverno repos (chainsaw, kyverno-json etc. — see go.mod).
- Top level: `api/` (CRD types: `kyverno/`, `policyreport/`, `reports/`), `cmd/` (7 binaries: kyverno, kyverno-init, cli, cleanup-controller, reports-controller, background-controller, readiness-checker), `pkg/` (36 packages incl. `engine`, `webhooks`, `controllers`, `cel`, `client` [generated], `cosign`… actually cosign/notary live under `pkg/` per CODEOWNERS lines 19–20), `test/`, `charts/`, `config/crds/`, `hack/`, `scripts/`, `docs/dev/`, `ext/`.
- **`AGENTS.md` already exists at root and is thorough (247 lines)**: structure map, make targets, codegen commands, API design rules, import-alias conventions, logging levels, a "PR Failure-Prevention Checklist" (DCO signoff, codegen verify, fmt/imports). The brief's "expand AGENTS.md" = per-directory stubs + machine-readable metadata, not creating the root file.

## 2. Dependabot / Renovate handling

- `.github/dependabot.yml`: gomod (root + 2 hack dirs) and github-actions ecosystems, **daily** schedule, `rebase-strategy: disabled` (lines 10, 27), dependency groups for k8s.io/sigstore/otel.
- No Renovate config found (`renovate.json` absent).
- **No auto-merge automation exists** — grep of `.github/workflows` for automerge/`gh pr merge` finds nothing. Live data: last 30 merged Dependabot PRs were merged manually by @realshuting, @eddycharly, @fjogeleit; latency 0–74h, roughly half ≥20h, often merged in daily batches (e.g. #16933–16937 all at ~24h). Dependabot commits = **151 of 578 commits on main in the last 6 months (~26%)**.

## 3. CI structure

- ~40 workflows in `.github/workflows/`. PR-triggered checks: `check-codegen.yaml` (jobs `verify-codegen-code` running `make codegen-all-code` + `make verify-codegen`, and `verify-codegen-docs`; on failure uploads a `codegen-code.patch` artifact with apply instructions), `check-unit-tests.yaml` (single job, `make test-unit`, ~10 min wall), `check-fmt`, `check-vet`, `check-imports`, `check-golangci-lint`, `check-unused-package`, `check-cli-tests`, `check-sha-pinned-actions`, `tests-conformance.yaml` (many jobs — one per chainsaw suite dir — each a 3-version k8s matrix `[v1.33.7, v1.34.3, v1.35.1]`, via composite action `.github/actions/tests/conformance/run`).
- Semantic PR titles enforced via `.github/semantic.yml` (`titleOnly: true`, types incl. `feat/fix/autogen/security/ci/chore`).
- Branch protection / required-check list is **unverified** (API returns 404 without admin; may use rulesets). `code-freeze.yaml` says the "freeze" check is marked required for frozen `release-*` branches via `FROZEN_BRANCHES` repo variable.

## 4. Test organization

- Unit tests: colocated `_test.go` under `pkg/` etc.; `make test-unit` = `go test -race` over `./...` (AGENTS.md:118).
- Conformance: `test/conformance/chainsaw/` with **52 suite directories** (assert, autogen, cel, cleanup, generate, mutate, webhooks, …). CI runs one workflow job per suite dir (`tests-path:` input), so suite dir ≈ CI shard — a natural unit for scoped test selection.
- CLI tests: `test/cli/` fixtures run via `kubectl-kyverno test` (`make test-cli`).
- Also `test/fuzz/`, `test/load/` (k6), `test/openshift/`, `test/policy/`.
- **No path→suite mapping exists anywhere** (no `.ai/`, no test-map metadata). The mapping the brief wants must be built.

## 5. Issue templates / labels / triage

- `.github/ISSUE_TEMPLATE/`: `bug-other.yaml` (labels `["bug","triage"]`), `bug-cli.yaml` (`["bug","type:cli","triage"]`), `bug-webhook.yaml`, `feature.yaml` (`["enhancement","triage"]`), `VULN-TEMPLATE.md`, `config.yml`.
- `.github/labels.yml` is the single source of truth for labels **and** contains `rules:` path-globs consumed by `pr-labelling.yaml`, which renders it into an actions/labeler config at run time (lines 28–38). Area labels (`area/api`, `area/crds`, …) are auto-applied from changed paths.
- Live state: **270 open issues, 138 (51%) still carry `triage`** — issue triage is the real backlog. Only 1 open issue has no labels at all (templates guarantee initial labels).
- 293 open PRs; only 4 untouched since before 2026-05-01 (auto-branch-updates muddy "staleness by updated-at").

## 6. Ownership

- `CODEOWNERS`: everything falls back to `@kyverno/kyverno-core-maintainers`; per-path owners for `/api`, `/api/kyverno/v1`, `/pkg/engine`, `/pkg/webhooks`, `/pkg/cosign`, `/pkg/notary` (Vishal-Chdhry, lucchmielowski), `/pkg/cel`, `/cmd/cli`, `/test`, etc. Good machine-readable input for reviewer suggestion / sensitivity mapping.
- Also `MAINTAINERS.md`, `OWNERS.md`, `GOVERNANCE.md` at root.

## 7. Generated vs hand-written; codegen

- Generated: `zz_generated.deepcopy.go` / `zz_generated.register.go` (14 `zz_generated*` files), all of `pkg/client/` (~860K), `config/crds/*.yaml`, helm chart CRDs/docs.
- Commands: `make codegen-all-code` (= codegen-api-all + client-all + crds-all + cli-all + helm-all, Makefile:811–816), `make codegen-all` (code + docs), `make verify-codegen`. AGENTS.md:198: never hand-edit generated files.

## 8. Sensitive paths (candidate deny-list, grounded in repo)

- `api/kyverno/v1` (extra CODEOWNERS entry line 4; AGENTS.md forbids new types in v1)
- `pkg/cosign/`, `pkg/notary/` (supply-chain verification; dedicated owners)
- `.github/workflows/**` (workflows hold secrets: `release.yaml`, `images-publish.yaml`, `helm-release.yaml`; `check-sha-pinned-actions.yaml` shows they enforce action pinning)
- `main`, `release-*` branches (code-freeze machinery; cherry-pick flow targets release branches)
- `charts/` (release artifacts), `config/crds/` (must only change via codegen), `SECURITY.md`/`DEPENDENCY-POLICY.md`.

## 9. Existing automation (do NOT rebuild)

| Workflow | What it already does |
|---|---|
| `pr-branch-updater.yml` | **Auto-updates PR branches on every push to main**, via shared workflow + dedicated GitHub App token. The brief's "PR hygiene: auto-update branch" already exists. |
| `pr-labelling.yaml` | Path-based PR area labels from `labels.yml` + conflict `dirtyLabel` handling |
| `pr-rate-limiter.yaml` | Closes PRs beyond 8 open per non-member author |
| `comment-prow.yaml` | Prow-style slash commands: `/assign`, `/unassign` (+ more in `execute` job) |
| `comment-conformance.yaml` | `/conformance` comment triggers conformance run on a PR |
| `cherry-pick-on-merge.yaml` + `comment-cherry-pick.yaml` | `/cherry-pick <branch>` automation to release branches |
| `assign-milestone.yaml` | Auto-assigns release milestone from git history at tag push |
| `periodic-cleanup.yaml` | Nightly stale-branch cleanup (dependabot/, cherry-pick- prefixes) + failure-issue sync |
| `code-freeze.yaml` | Freeze check for `release-*` via `FROZEN_BRANCHES` var |
| `.github/config.yml` | behaviorbot welcome comments for first-time issue/PR authors |
| `sync-trivy-issues.yaml`, `periodic-trivy.yaml`, scorecard/semgrep/sonar/fossa | Security scanning automation |

- `.github/actions/` has reusable composite actions (go setup, ko, kind, conformance run) the POC can reuse rather than reinvent.

## 10. Where maintainer time actually goes (evidence)

- **Dependabot merging is manual**: ~26% of main-branch commits, hand-merged in daily batches by 2–3 maintainers. Clear, measurable target.
- **Issue triage backlog**: 51% of open issues labeled `triage` (unprocessed). Templates apply coarse labels but classification/repro/дedup is human work.
- PR branch updating: already automated (see above) — the brief overstates this gap.
- Conformance CI: every PR runs all ~52 suites × 3 k8s versions (wall ~10 min thanks to parallelism, but large compute); no scoped selection exists.

## Contradictions / gaps vs. the challenge brief

1. "Keeping PRs rebased with main" — **already automated** (`pr-branch-updater.yml`). POC shouldn't rebuild it; scoped-test + dependency-merge + triage are the live gaps.
2. Root `AGENTS.md` already exists and is good; the Phase-0 work is per-directory stubs, machine-readable task index, path→test map — none of which exist yet (verified: no `.ai/` dir).
3. Brief says maintainers "re-run CI" — Kyverno already has `/conformance` comment triggering; gap is *scoping*, not triggering.
4. `pkg/cosign`/`pkg/notary` as sensitive paths: confirmed real (CODEOWNERS 19–20).
5. Stale-PR nudging: staleness detection by `updatedAt` is unreliable here because the branch-updater touches PRs constantly; would need review-activity-based signals.
6. Branch protection / required checks not verifiable with current token (needs maintainer confirmation).
