# OVERRIDE.md — Phase 13: Human Override & Control

Principle: every control is something a maintainer can operate **from GitHub alone** (labels, repo variables, revert) — no access to the assistant's host required. The assistant's own machinery honors them; GitHub App suspension works even if the assistant is fully compromised.

## The seven required controls

### 1. Kill switch (global)
- **Mechanism:** repo variable `AI_MAINTAINER_PAUSED=true` (`gh variable set` or repo Settings — maintainer permission, no deploy).
- **Honored:** at run start, inside every `Evaluate`, and by the executor immediately before each side effect *and* each sandbox stage (POLICY invariant 3, RISKS P5). Mid-run flip ⇒ current run finishes its in-flight read, then aborts with `run_finished: aborted(kill_switch)`.
- **Break-glass tier:** suspend the GitHub App installation (Settings → Integrations) — platform-level revocation of all API access, effective even against assistant bugs. Documented in README as the "assume the worst" option.

### 2. Per-workflow / per-repo disable
- `workflows.<name>.enabled: false` in `.github/ai-maintainer.yaml` — takes effect next run (config re-read per run, pinned per run). Per-repo = the config file lives in the repo; no file ⇒ assistant refuses to operate on that repo at all (deny-by-default, POLICY invariant 2).

### 3. Human-hold label (per entity)
- `ai-hold` (assistant-specific) or `hold` (existing Kyverno convention) on any PR/issue freezes **all** actions on that entity. Checked fresh in every Decision; `never_remove` list makes it impossible for the assistant to lift its own hold. Adding the label mid-run wins at the next Decision (fetch-fresh, P4).

### 4. Approval requirement for mutating actions — staged autonomy
Per-workflow `mode` in config; the POC ships demonstrating all three:

| Mode | Behavior |
|---|---|
| `advisory` | Analysis + comment only; policy denies label/merge actions structurally. |
| `approval` | Assistant posts intent comment ("would merge; all 11 rules pass — apply label `ai-approve` to proceed"); acts only after a **maintainer-authored** approval label (author association checked: MEMBER/OWNER; the assistant cannot self-approve, and label *events* from non-members are ignored). |
| `autonomous` | Acts immediately on ALLOW (still fully policy-gated + rate-limited). |

Recommended rollout (mirrors D-005 staging): triage-lite starts `autonomous` (labels/comments only — lowest stakes), dependency merges start `approval`, go `autonomous` per maintainer comfort. Demo shows `approval` → flip to `autonomous` in config → merge happens.

### 5. Stop a running task
- `assistant stop <run_id>` (host-side) → cancels context, kills sandbox (`docker kill` path from SANDBOX.md), writes `run_finished: aborted(manual)`.
- From GitHub only: flip kill switch or add `ai-hold` — takes effect at the next action/stage boundary (≤ seconds, since stages are short; the longest non-interruptible window is a single GitHub API call).
- Sandbox reaper covers the crash case (SANDBOX.md lifecycle).

### 6. Revert agent-generated changes
- **Merges:** squash-only ⇒ one commit ⇒ `git revert <sha>`; the exact command with the real SHA is printed in the merge comment and `audit show` footer. Assistant labels reverted-PRs `ai-reverted` when it observes the revert (feedback signal for eval; nice-to-have).
- **Labels:** remove in UI; assistant never re-applies a label a human removed (dedupe store records human removals per entity — required, else label ping-pong, RISKS G5).
- **Comments:** upserted single comment per (entity, template) — edit or delete in UI; deleted comments are not re-posted (same dedupe rule).
- Nothing else exists to revert: no pushes, no branch writes, no closes (MCP_TOOLS "explicitly absent" table).

### 7. "Why did it do this" trace
- `assistant audit why <entity>` → decision trace(s) (AUDIT.md); every GitHub comment carries its run-ID marker linking to it; the merge/flag comment already *contains* the rendered rule trace, so for the most consequential action the "why" is on the PR itself, zero tooling required.

## Control-plane trust note
Labels and repo variables are maintainer-permission surfaces on GitHub; the approval label check additionally verifies the labeler's author association. So the override plane is exactly as trustworthy as the repo's existing permission model — no new admin surface is introduced by the POC. (Config file changes ride the normal PR review path.)

**Must-have:** 1–7 all; they are cheap because they reuse policy/audit/sandbox machinery already specified. **Nice-to-have:** `ai-reverted` observation loop. **Demo:** kill switch mid-run + `audit why` + the revert command printed on the PR are the three on-camera override moments (POC_SCOPE demo plan).
