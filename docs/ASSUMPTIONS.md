# ASSUMPTIONS.md

Unverified items, each with how to verify.

1. **Branch protection / required checks on `main`** — API returned 404 (needs admin scope; repo may use rulesets). *Verify:* ask a maintainer, or check `gh api repos/kyverno/kyverno/rules/branches/main` / repo settings.
2. **Conformance CI total compute cost** — wall time ~10 min observed on recent completed runs; per-job billable minutes not summed. *Verify:* `gh run view <id> --json jobs` and sum durations across all matrix jobs.
3. **Maintainer time per Dependabot PR** — merge latency (0–74h) measured, but hands-on review minutes per PR are inferred, not measured. *Verify:* ask mentors (@JimBugwadia, @realshuting) or sample PR review timelines.
4. **`triage` label = "not yet triaged"** — inferred from templates auto-applying it and 51% of open issues carrying it. *Verify:* confirm label semantics with maintainers (could also mean "triaged, awaiting decision").
5. **Slack/Discussions question volume** — not observable from the repo; GitHub Discussions appears unused/low-traffic for kyverno/kyverno. *Verify:* ask mentors for Slack stats.
6. ~~Prow command list~~ **Resolved 2026-08-13:** full set is `/assign`, `/unassign`, `/lgtm`, `/milestone` (comment-prow.yaml lines 27–29, 40–42).
7. **GitHub App permission granularity** assumed sufficient for "comment/label/draft-PR but not merge-to-main" — believed true (contents:write is branch-protection-gated), needs a concrete permissions matrix in Phase 7. *Verify:* GitHub App permissions docs + test installation on the fork.
8. ~~Chainsaw suite dir ≈ independent CI shard~~ **Mostly resolved 2026-08-13:** `.github/actions/tests/conformance/run/action.yaml` confirms per-suite invocation with `tests-path`, plus `chainsaw-tests` regex filter, `shard-index/count`, and `quarantined-tests` inputs — scoped selection has first-class hooks. Cross-suite independence still assumed (each job gets a fresh KinD cluster, which strongly implies independence).
