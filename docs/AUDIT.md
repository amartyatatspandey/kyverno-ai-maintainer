# AUDIT.md — Phase 12

## Shape: append-only JSONL event stream per run + one run summary, plus a human render

- `audit/runs/<run_id>/events.jsonl` — every event, appended *before* the thing it describes executes (write-ahead: a mutating action with no prior audit event is a bug the executor enforces against, matching POLICY invariant 4).
- `audit/runs/<run_id>/summary.json` — closed at run end (or by the reaper on crash, `outcome: "orphaned"`).
- `audit/index.jsonl` — one line per run for listing/filtering.
- `assistant audit show <run_id>` renders the maintainer-facing view (below). `assistant audit why pr/17067` finds the run(s) that touched an entity and prints the decision trace — the Phase 13 "why did it do this" command reads this same data; no separate store.

Run IDs: `run_20260813_171203_pr17067_a1b2` — sortable, greppable, embedded in every GitHub comment marker so any comment links back to its run.

## Event schema (JSONL, one `type` per line)

Common envelope: `{run_id, seq, ts, type, ...}` — `seq` monotonic; hash-chain field `prev_sha256` per event (cheap tamper-evidence; nice-to-have flagged below).

| type | payload (beyond envelope) |
|---|---|
| `run_started` | trigger (`poll`/`manual`/`eval`), repo, entity (`pr/17067`), head_sha, config_sha (ai-maintainer.yaml blob SHA), **model_id, prompt_pack_version**, assistant git version, sandbox image digest |
| `tool_called` | tool name, args (validated form), read_only flag |
| `tool_result` | outcome, size, truncated flag — **bounded excerpt (≤4 KiB), not full payload**; full sandbox logs stay in `runs/<id>/artifacts/` |
| `llm_call` | insertion point (W1 risk summary / W3 widen / W4 classify), token counts, latency; prompt/response stored as artifact files, referenced by hash |
| `policy_decision` | action, target, **full rule trace** (every rule, pass/fail, reason), allowed, bound_sha, expires_at |
| `command_executed` | template id, rendered argv, sandbox id, limits applied, exit code, duration |
| `test_results` | per-target pass/fail, durations, parsed failure names |
| `action_executed` | action type, target, result (comment URL / label set / merge SHA), decision_seq it consumed |
| `budget_tick` / `budget_exceeded` | which counter, value, cap |
| `kill_switch_checked` | source (variable/label), state |
| `run_finished` | outcome (`completed`/`denied`/`escalated`/`aborted`/`budget_exceeded`/`orphaned`), duration, totals (tools, llm calls, tokens, est. cost) |

Everything the eval harness needs (EVAL_HARNESS.md per-case record) is derivable from these events — by design, since eval replays write the same stream.

## No secrets, enforced not promised

1. **Structural:** the audit writer lives in the executor process; sandbox has no credentials to leak (SANDBOX.md), so nothing secret exists in tool_result/test payloads by construction.
2. **Belt-and-braces scrubber** on every event before write: regexes for GitHub token shapes (`gh[pousr]_…`, `github_pat_…`), `Authorization:` headers, LLM API key shapes (provider-prefixed), plus exact-match redaction of the process's own live token values. Scrubber has unit tests with seeded fakes.
3. Prompt/response artifacts pass the same scrubber (prompts contain untrusted repo text — fine; they must never contain credentials).

## Human-readable render (`audit show`) — the maintainer view

```
Run run_20260813_171203_pr17067_a1b2 — dependency_prs — PR #17067
Trigger: poll  |  Model: <model-id> (prompts v3)  |  Config: ai-maintainer.yaml@9f3c21d
Outcome: MERGED (squash 8c1d44f) in 4m12s — 3 LLM calls, 14.2k tokens (~$0.12)

What happened:
 1. Read PR: dependabot bump k8s.io/client-go 0.34.1 → 0.34.2 (patch, group: kubernetes)
 2. Diff check: 2 files (go.mod, go.sum) — within allowed paths
 3. Risk summary (LLM): "routine patch; changelog shows bugfixes only; no API changes"  [advisory]
 4. Scoped tests: 14 unit pkgs (import closure) + suite 'assert' — all green in sandbox (11m03s)
 5. POLICY merge_pr → ALLOW  [bound to head 7e22a1c, fresh state 09:12:41Z]
      ✓ workflow enabled          ✓ kill switch off        ✓ author app/dependabot (Bot)
      ✓ update patch ∈ {patch,minor} ✓ required checks green  ✓ labels ∩ deny = ∅
      ✓ files ⊆ allowed globs     ✓ no protected paths     ✓ base main
      ✓ merges today 3/10         ✓ decision fresh (TTL 60s)
 6. Merged 8c1d44f; comment posted (upsert) with this trace; label ai-approved

To undo: git revert 8c1d44f   |  To stop the bot: label 'ai-hold' or set AI_MAINTAINER_PAUSED=true
```

DENY renders identically with the failing rule first (`✗ update major — requires human review`), and `escalated` runs end with "what a human should look at." Step 3's LLM text is visibly tagged `[advisory]` — reinforcing on-camera that it never gates anything.

**Must-have:** events.jsonl + summary + scrubber + `audit show`/`why`. **Nice-to-have:** hash chain, cost estimation precision, index filters. **Demo theater that earns it:** `audit show` on screen right after the live merge — it *is* the "not a bare LLM wrapper" money shot. **Not built:** external log shipping, dashboards (roadmap).
