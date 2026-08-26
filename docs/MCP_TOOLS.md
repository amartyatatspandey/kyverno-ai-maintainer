# MCP_TOOLS.md — Phase 6

Design rule: a tool exists only if a POC workflow (W1 dependency PRs, W3 scoped tests, W4 triage-lite; W5 repro as stretch) needs it. Deterministic pipeline stages (semver parsing, import-closure computation, policy evaluation, audit) are **not** tools — they are code the LLM cannot invoke or influence. Tools are the complete capability surface of the LLM loop; if it's not listed here, the model cannot request it (TRUST_MODEL: "a tool not exposed cannot be requested").

Anti-duplication check (NOTES.md §9): no tool re-implements branch updating (`pr-branch-updater`), full-CI triggering (`/conformance` comment), path-labeling of PRs (`pr-labelling`), or prow commands. `run_scoped_tests` runs *scoped* suites locally in a sandbox — a capability CI does not offer.

Every mutating tool call: (1) is schema-validated, (2) produces a policy Decision bound to fresh GitHub state + head SHA, (3) fails closed if no Decision exists (RISKS P3), (4) writes an audit record either way.

The same tools are also served over the MCP protocol (`assistant mcp`, stdio, official Go SDK). That is a transport: mutating calls still go through `Engine.Evaluate` then `ghx`.

---

## Read-only tools (no policy Decision; rate/size-bounded; audit-logged)

### 1. `github.get_pull_request`
- **Purpose:** PR facts for W1/W3 context and re-checks.
- **In:** `{number}` → **Out:** author, author-type (Bot/User), title, base/head ref + head SHA, labels, mergeable state, check-run summaries, draft flag.
- **Wrong-doing potential:** stale data if cached → policy layer refetches independently before ALLOW (P4); tool output includes untrusted title/body text → passed to LLM as quoted data only.

### 2. `github.get_diff`
- **Purpose:** changed-file list + bounded patch for W1 path-allowlist narrative and W3 selection explanation.
- **In:** `{number, max_bytes}` → **Out:** file list with status, truncated unified diff.
- **Wrong:** huge vendored diffs blow context → hard size cap + file-list-only fallback. Diff content is untrusted (can contain injection in code comments).

### 3. `github.get_issue`
- **Purpose:** W4 classification input; W5 repro extraction.
- **In:** `{number}` → **Out:** title, body, template fields parsed (versions, component), existing labels, author association.
- **Wrong:** body is the #1 injection vector (A2) → always quoted-data framing; parsed fields validated by schema before any deterministic use.

### 4. `github.search_issues`
- **Purpose:** duplicate-candidate lookup for W4.
- **In:** `{query, limit≤10}` → **Out:** number/title/state/labels per hit.
- **Wrong:** LLM crafting overly broad queries burns rate limit → server-side limit + per-run call budget; results are untrusted text.

### 5. `repo.read_file`
- **Purpose:** let the LLM ground claims in the pinned checkout (e.g., read a chainsaw suite's test to justify widening selection; read `AGENTS.md` conventions).
- **In:** `{path, start_line?, end_line?, max_bytes}` → **Out:** file slice from a read-only checkout **pinned to the run's target SHA**.
- **Wrong:** token burn via mass reads → per-run byte budget; path traversal → normalized paths jailed to checkout root; file contents (incl. `AGENTS.md`) are data, not instructions to the runtime.

### 6. `kyverno.get_affected_tests`
- **Purpose:** THE repo-intelligence tool for W3. Deterministic mapping executed server-side.
- **In:** `{changed_files[]}` → **Out:** `{unit_packages[] (reverse import closure), chainsaw_suites[] (path→suite map), unmapped_files[], mapping_version}`.
- **Wrong:** map staleness/gaps → `unmapped_files` forces the full-suite fallback rule; recall measured in Phase 11 (T1). LLM may union extra suites on top, never subtract (A6).

### 7. `kyverno.get_label_taxonomy`
- **Purpose:** W4's allowed-label vocabulary with descriptions, derived from `.github/labels.yml`, minus privileged labels (`security`, `good first issue`, release/milestone labels).
- **In:** `{}` → **Out:** `[{name, description, category}]`.
- **Wrong:** taxonomy drift vs. upstream → generated from the pinned checkout each run.

### 8. `kyverno.get_owners`
- **Purpose:** map changed paths → CODEOWNERS entries (W1 escalation targets, W4 area routing).
- **In:** `{paths[]}` → **Out:** `[{path, owners[]}]`.
- **Wrong:** @-mentioning owners in comments could spam → comment templates cap mentions; policy limits comment frequency (G5).

## Sandboxed execution tools (policy Decision required; run in Zone X)

### 9. `sandbox.run_scoped_tests`
- **Purpose:** W3 execution. Runs selected unit packages and/or chainsaw suites in an ephemeral container (KinD inside for chainsaw).
- **In:** `{unit_packages[], chainsaw_suites[], timeout_s}` → **Out:** `{per_target: pass/fail, parsed failures, bounded logs, durations}`.
- **Policy checks:** every package/suite must exist in the deterministic selection ∪ LLM-widened set recorded for this run; command template is fixed (`go test <pkgs>`, `chainsaw test --test-dir <suite>`); resource caps; no credentials in env (S2/S3).
- **Sandboxed:** yes — the only way this tool exists.
- **Wrong:** resource exhaustion (S2) → cgroup caps + timeout; flaky suites (T2) → one recorded retry max; output logs are untrusted when fed back to LLM.

### 10. `sandbox.reproduce_issue` *(stretch, W5)*
- **Purpose:** scripted repro of a bug report. **Not** free-form: fixed script (fresh KinD → install pinned Kyverno → apply policy → apply resource → capture outcome).
- **In:** `{policy_yaml, resource_yaml, kyverno_version, k8s_version, expected_behavior}` → **Out:** `{applied_ok, admission_result, relevant_logs (bounded), actual_vs_expected}`.
- **Policy checks:** YAML kind allowlist (Kyverno policy kinds + core workload kinds), size caps, no `hostPath`/`privileged` in supplied resources, version pin allowlist; egress denied during execution (S4).
- **Wrong:** hostile YAML attacks the cluster → it only ever attacks its own throwaway KinD; escape risk accepted-and-documented for POC (S1); false repro via wrong versions (T4) → versions pinned from issue fields and echoed in evidence.

## Mutating GitHub tools (policy Decision required; least privilege; all revertible)

### 11. `github.comment`
- **Purpose:** the assistant's voice — W1 risk summaries/escalations, W3 selection rationale + results, W4 classification/missing-info.
- **In:** `{entity (pr/issue), number, template_id, params}` → **Out:** comment URL.
- **Design:** body is rendered server-side from a fixed template + validated params; the LLM supplies *fields*, not raw markdown; every comment carries a hidden run-ID marker and is **upserted** (edit-in-place) per (entity, template) to prevent spam (G5, A2 containment).
- **Policy:** per-entity daily budget; kill switch; template must exist.
- **Wrong:** injection-shaped content inside a param (residual A2) → params length-capped and escaped; misleading advice possible but advisory-only.

### 12. `github.set_labels`
- **Purpose:** W1 (`ai-approved`, `needs-human-review`), W3 (`scoped-tests-passed/failed`), W4 (area/type labels).
- **In:** `{entity, number, add[], remove[]}` → **Out:** resulting label set.
- **Policy:** add/remove sets ⊆ workflow-specific allowlist (taxonomy minus privileged); never removes `hold`; never removes `triage` (human-only per W4); rate-limited.
- **Wrong:** wrong classification label (A4) → revertible, tracked as correction-rate metric.

### 13. `github.merge_pr`
- **Purpose:** W1's single consequential action.
- **In:** `{number, expected_head_sha}` → **Out:** merge SHA or structured refusal.
- **Policy (the full W1 gate, deterministic):** author=Dependabot ∧ update∈{patch,minor} (parsed by pipeline, not LLM) ∧ all required checks green ∧ labels ∩ {hold,security,breaking-change}=∅ ∧ changed files ⊆ {go.mod, go.sum, workflow-pin lines} ∧ base=main ∧ under daily merge budget ∧ kill switch off — re-verified against **fresh** API state, merged with `expected_head_sha` (G2).
- **Method:** squash (single revert-able commit), matching observed maintainer practice.
- **Wrong:** G1 residual (compromised green patch bump) → rate limit + optional min-age + revert runbook in audit record.

---

## Explicitly absent (and why)

| Not a tool | Reason |
|---|---|
| `github.push` / branch or file writes | No POC workflow drafts code; removes an entire risk class (G4). Draft-PR tooling returns with W8/docs on the roadmap. |
| `github.close_issue` / `assign` / `request_review` | Prow commands + humans already do this; W4 is propose-only. |
| `shell.exec` (free-form) | Thesis violation (D-004); only templated commands inside tools 9–10. |
| `github.approve_pr` (formal review) | Approval semantics reserved for humans; W1's `ai-approved` label + comment carries the same information without impersonating review. |
| Full-CI trigger | `/conformance` comment already exists; duplicating CI is anti-goal. |

**Coverage check:** W1 uses 1,2,11,12,13. W3 uses 1,2,5,6,9,11,12. W4 uses 3,4,7,8,11,12. W5 adds 10. Every pillar has at least one tool exercising it; total 13 tools (12 without stretch).
