# SECURITY_ADVISORY_TRIAGE.md — Design-only (not built)

Same spec format as PROBLEM_MAP.md. Verdict: **DESIGN-ONLY**. Not implemented in the POC or Phase 2 build — the risk-to-demo-value ratio doesn't clear the bar the other workflows did, and severity judgment is the one place I don't want a deterministic policy engine anywhere near a false sense of authority.

## Why design-only, not built

Every other workflow in this system works because the policy engine's rules are checkable against objective facts: is the author Dependabot, are checks green, does the diff touch an allowlisted path. Severity assessment for a security report isn't that — "is this a P1" is a judgment call informed by exploitability, blast radius, and context a maintainer holds that isn't in the issue body. An agent that drafts a severity number, even labeled "advisory," risks being treated as one by a triager skimming a queue. That's a worse failure mode than the workflows this POC already ships, where a wrong advisory summary just gets ignored in favor of the policy trace.

## Trigger
New issue matching `VULN-TEMPLATE.md` (or filed via GitHub's private security advisory flow, if Kyverno uses that instead of public issues — needs verification, see below).

## Inputs (maximally untrusted — same trust tier as W5's user-supplied YAML)
Report body: affected component, versions, reproduction steps, claimed impact. Cross-reference data: `go.mod` dependency versions, public CVE feeds (NVD/OSV) for named dependencies.

## What it would decide vs. what it would never decide
- **Deterministic, safe to automate:** does the named dependency+version appear in `go.mod`/`go.sum`? Does a CVE ID mentioned in the report resolve to a real NVD/OSV entry? Is the affected version range satisfied by what's currently pinned? These are yes/no facts, not judgment.
- **LLM-advisory only, never authoritative:** a first-pass summary of "what the reporter claims, in plain language" — same pattern as W1's risk summary.
- **Never automated, human-only:** the actual severity label (P0–P3 or CVSS score), whether to embargo, whether to notify downstream consumers, whether the report is credible at all.

## Safe actions (if ever built)
Comment with the dependency-match facts only ("kyverno pins github.com/x/y@v1.2.3; the reported range is <v1.3.0 per OSV-2026-xxxx — version check: MATCH"). No severity label, no public comment that confirms or denies vulnerability status (that itself can be an information leak before a fix ships). Route to a private channel/label for human triage only.

## Human checkpoint
Everything past the dependency-match fact-check is human-only. This workflow, if built, should probably not even use the public issue-comment mechanism the rest of the system uses — it likely needs a private advisory channel, which is a different trust surface than anything else in this POC and deserves its own credential/permission review before a line of code gets written.

## Open question for mentors
Does Kyverno use GitHub's private security advisories, or triage `VULN-TEMPLATE.md` reports as public issues today? The design above assumes public issues; if it's private advisories, the trust model changes (the report itself is already access-controlled, which removes some of the concern above but adds a new credential scope — `security_events` — that nothing else in this system currently touches).
