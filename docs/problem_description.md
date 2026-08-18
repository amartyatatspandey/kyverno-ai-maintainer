### Problem Statement

Kyverno's maintainers spend significant recurring effort on low-judgment, repetitive tasks: reviewing/merging Dependabot PRs, keeping open PRs rebased with  main  and re-running CI, triaging a high volume of incoming issues, reproducing bug reports, running the right subset of conformance/unit tests for a given diff, and answering repeat questions in Slack/GitHub Discussions. This work competes for maintainer time with code review, design, and roadmap work, and slows down contributor turnaround (stale PRs, delayed triage labels, slow first response on issues).

### Solution Description

Build an AI Maintainer Assistant: a sandboxed, permission-scoped autonomous agent (e.g., running an agent runtime such as OpenHands/OpenClaw/Hermes-style sandboxed coding agent) that runs on a schedule and via GitHub/Slack webhooks to automate routine maintainer workflows, always via auditable, revertible actions (comments, labels, draft PRs) rather than unreviewed direct pushes to protected branches.

**Proposed initial scope (Phase 1 — safe, reversible actions only):**

- Dependency PR handling: auto-review Dependabot/Renovate PRs, run CI, and auto-merge patch/minor bumps that pass tests and have no breaking-change signals; flag major bumps for human review with a summary.
- PR hygiene: detect PRs that are behind  main , auto-update branch (merge/rebase) when mergeable, re-trigger CI, and nudge stale PRs/reviewers after a configurable idle period.
- Scoped test selection: analyze the diff (changed packages/dirs) and trigger only the relevant subset of unit/conformance (chainsaw) tests instead of the full suite, using Kyverno's  pkg/  and  test/conformance/  structure to map file paths to test packages.
- Issue triage: classify new issues (bug/feature/question/CLI/webhook), apply labels per existing templates, request missing repro info, and attempt automated reproduction (e.g., spin up a KinD cluster, apply supplied policy/resource, capture actual vs. expected behavior) before human review.
- Slack/Discussions Q&A assistant: answer common questions using project docs ( kyverno.io ,  docs/dev/ ) and link relevant issues/PRs, escalating to a human when confidence is low.
- Codegen/verify gate: for PRs touching  api/ , automatically run  make codegen-all-code && make verify-codegen  and flag mismatches.
- Identify when doc changes are required and draft PRs.

**Guardrails (required, not optional):**

- Runs in an isolated sandbox with least-privilege GitHub App credentials (no direct write to  main /release branches).
- All merges/changes require passing CI + defined policy checks (e.g., only auto-merge Dependabot patch/minor with green CI and no maintainer "hold" label).
- Every automated action is logged and traceable to a specific run/decision; humans can override or disable per-repo/per-workflow via a config file (e.g.,  .github/ai-maintainer.yaml ).
- Rate-limited and kill-switch controlled (label or repo variable to pause the bot instantly).

Beyond the automation agent itself, restructure the repo so both AI agents and humans can navigate/build/test it more predictably:

- Monorepo module boundaries: Kyverno is mostly a single repo but depends on SDK / API, etc. Evaluate if a monorepo make the codebase more maintainable. 
- Deepen  AGENTS.md  (already present at repo root) — add/expand:
- Per-directory  AGENTS.md  stubs in high-traffic areas ( pkg/engine/ ,  pkg/webhooks/ ,  pkg/controllers/ ,  test/conformance/ ) documenting local conventions, key entry points, and "don't touch without regenerating" files.
- A machine-readable task index (e.g.,  make help -style JSON/YAML manifest of build/test/lint targets) so agents don't have to parse the Makefile.
- Explicit "safe automation boundaries" doc: which paths/files an agent may modify autonomously (e.g., dependency bumps, generated CRDs) vs. never touch without human review (e.g.,  api/kyverno/v1 , security-sensitive  pkg/cosign / pkg/notary ).
- CI/test metadata for scoped runs: tag test packages/conformance suites with the source paths they cover (supports the "scoped regression test" capability from the Maintainer Assistant) — e.g., a  test/conformance/chainsaw/*/OWNERS -style path-to-suite map.
- Structured contribution/PR metadata: standardize PR labels/templates so agents can reliably classify change scope (docs-only, generated-code-only, breaking API, etc.) without heuristics.
- 



### Alternatives

*No response*

### Additional Context

This is proposed as an LFX Mentorship project. 

Suggested phased deliverables for a mentee:

1. Phase 0: Audit and document current repo structure; propose and land minimal monorepo/module boundary changes + expand  AGENTS.md  and per-directory agent docs; publish path→test-suite map.
2. Phase 1: Sandbox + GitHub App scaffolding, Dependabot auto-merge, PR-stale/rebase automation (uses Phase 0 metadata).
3. Phase 2: Diff-to-test-scope mapper using the Phase 0 path→test map.
4. Phase 3: Issue triage + automated repro harness.
5. Phase 4 (stretch): Slack/Discussions Q&A assistant grounded in docs.

Additional brainstormed capabilities for future phases (not required for MVP):

• Auto-draft release notes/changelog entries from merged PR titles/labels.
• Flaky test detection — track CI failures over time and flag/quarantine flaky conformance tests automatically.
• Auto-backport labeled PRs to release branches, opening backport PRs for maintainer approval.
• License/CLA and DCO sign-off checker with auto-comment guidance for contributors who forget  -s .
• "First-time contributor" welcome + contribution guide pointer bot.
• Security advisory triage: pre-screen  VULN-TEMPLATE.md  reports, check against known CVEs/dependencies, draft initial severity assessment for maintainer review.
• Auto-suggest reviewers based on code ownership/history for a given diff.
• Weekly maintainer digest: open PR aging report, issue backlog summary, CI flakiness trends.
• Policy YAML lint/dry-run bot for community-contributed sample policies ( charts/kyverno-policies  or website policy library) using the Kyverno CLI itself.

### Slack discussion

*No response*

### Research

- [x] I have read and followed the documentation AND the [troubleshooting guide](https://kyverno.io/docs/troubleshooting/).
- [x] I have searched other issues in this repository and mine is not recorded.

  
### CNCF Project

Kyverno

### Term

2026 Term 3 (Sep-Nov)

### Program Name

AI Assistant

### Program Description

## Description

Kyverno's maintainers spend significant recurring effort on low-judgment, repetitive tasks: reviewing/merging Dependabot PRs, keeping open PRs rebased with  main  and re-running CI, triaging a high volume of incoming issues, reproducing bug reports, running the right subset of conformance/unit tests for a given diff, and answering repeat questions in Slack/GitHub Discussions. This work competes for maintainer time with code review, design, and roadmap work, and slows down contributor turnaround (stale PRs, delayed triage labels, slow first response on issues).

## Expected Outcomes

Build an AI Maintainer Assistant: a sandboxed, permission-scoped autonomous agent (e.g., running an agent runtime such as OpenHands/OpenClaw/Hermes-style sandboxed coding agent) that runs on a schedule and via GitHub/Slack webhooks to automate routine maintainer workflows, always via auditable, revertible actions (comments, labels, draft PRs).

### Technologies

AI Engineering, AI agent harness, Claude Code, GitHub Copilot, GitHub Actions, Hermes

### Skills same as Technologies?

- [x] Yes, the required skills are the same as the technologies listed above.

### Required/Desirable Skills

*No response*

### Mentors

Jim Bugwadia | @JimBugwadia | [jim@nirmata.com](mailto:jim@nirmata.com) | JimBugwadia

Shuting Zhao | @realshuting | [shuting@nirmata.com](mailto:shuting@nirmata.com) | ShutingZhao

### Upstream Issue URL

[https://github.com/kyverno/kyverno/issues/16665](https://github.com/kyverno/kyverno/issues/16665)

### Application Prerequisites

- [x] Resume

- [x] Cover Letter

- [ ] School Enrollment Verification

- [ ] Participation Permission from school or employer

- [ ] Coding Challenge

- [x] Custom Prerequisite (fill in details below)

### Coding Challenge URL

*No response*

### Custom Prerequisite Name

Solution Video

### Custom Prerequisite Description

Create a 2-5 minute video introducing yourself and then articulating your understanding of the problem and the proposed solution. Provide a link to the video on either Google Drive or YouTube as part of your cover letter.

### Custom Prerequisite — File Upload

- [ ] Yes — completion of this task requires the mentee to submit a file.

<!-- lfx-url:start -->

---

**LFX program:** [CNCF - Kyverno: AI Assistant (2026 Term 3)]([https://mentorship.lfx.linuxfoundation.org/project/c869e19a-8815-459b-8a2d-3a068e8863c3](https://mentorship.lfx.linuxfoundation.org/project/c869e19a-8815-459b-8a2d-3a068e8863c3))

<!-- lfx-url:end -->

