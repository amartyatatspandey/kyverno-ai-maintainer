# AUTO_BACKPORT.md — Design-only (not built)

Same spec format as PROBLEM_MAP.md. Verdict: **DESIGN-ONLY**.

## Why design-only, not built (this Phase 2 round)

Everything else in Phase 2 either only writes comments/labels (Tier 1 bots, W6, W8) or writes to a fresh ephemeral sandbox with no credentials (W5). Auto-backport is the first workflow that would need to **open a PR against a release branch** — a different, more sensitive branch class than `main`, often with its own protection rules, its own release manager, and its own blast radius if a bad cherry-pick lands right before a patch release ships. It's a real capability with real toil behind it (`AGENTS.md` and Kyverno's release process already reference manual backport labeling), but it's also the single highest-consequence write this system could make, and I'd rather ship it after the lower-risk workflows have run in the wild for a while than bundle it into a "let's impress" build pass.

## Trigger
PR merged to `main` carrying a `backport/release-X.Y` label (existing Kyverno convention — verify exact label scheme against `labels.yml` before building).

## Inputs
The merged commit SHA, the target release branch, the label set on the source PR.

## Decision logic (LLM proposes)
Attempt `git cherry-pick <sha>` against the release branch in an isolated clone. Detect clean cherry-pick vs. conflict. If conflict, do not attempt resolution — a model resolving a merge conflict on a release branch unsupervised is exactly the kind of "shell + token" pattern this whole system's thesis argues against.

## Deterministic authorization (policy engine would decide)
`backport/*` label present ∧ source PR was merged (not closed) ∧ target branch exists ∧ cherry-pick is clean (no conflict markers) ∧ target branch is not in the `protected_paths`-style deny set for direct-push ∧ rate limit ∧ kill switch off. Any conflict → escalate to human, never attempt resolution, never open a PR with conflict markers in it.

## Safe actions
Open a **draft PR** against the release branch (never merge automatically — this is `mode: approval` only, no autonomous path, unlike W1's patch-bump merges which at least have a track record of low-risk deltas). Comment linking the original PR. Label `backport-pending-review`.

## Human checkpoint
Every backport PR is draft + human-merged, no exceptions, no config flag to change that in v1. This is the opposite default from W1's dependency merges — there the POC demo *is* autonomous merge under narrow conditions; here the POC design deliberately has no autonomous branch at all, because the cost of a bad backport (broken patch release) is categorically worse than the cost of a bad dependency bump (one revert).

## What would need to be true before building this for real
1. Confirmed release-branch protection rules (mirrors QUESTIONS.md #1, but for release branches specifically, not `main`).
2. A separate GitHub App permission scope check — does the same least-privilege App used for W1 already have `contents:write` on release branches, or does this need a second, more narrowly scoped credential?
3. At least a few weeks of W1 running in production first, so there's a track record to point to before asking maintainers to trust an agent anywhere near a release branch.
