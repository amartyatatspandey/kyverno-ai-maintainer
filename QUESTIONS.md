# QUESTIONS.md — open questions for mentors/user

1. What are the actual required status checks on `main` (branch protection or rulesets)? (Blocks precise auto-merge policy design.)
2. Is the `triage` label's meaning "untriaged inbox" or "triaged, pending decision"?
3. Roughly how much maintainer time per week goes to Dependabot batches vs. issue triage vs. answering Slack questions? (Calibrates Phase 10 baseline.)
4. Is there appetite for the assistant to run against a fork first (amartyatatspandey/kyverno) for the demo, with upstream adoption as roadmap? (Assumed yes for POC.)
5. Would maintainers accept a `.github/ai-maintainer.yaml` config file in-repo, or should POC config live out-of-repo?
6. Any existing internal work on scoped test selection or path→suite mapping we should align with?
7. For the demo: is auto-merge of a real (fork) Dependabot PR acceptable, or should the demo stop at "policy ALLOW + draft approval comment"?

**Added after the Phase 16 eval run:**

8. `pr-rate-limiter.yaml` auto-closes Dependabot PRs once the bot exceeds 8 open PRs — 4 of 9 closed PRs in our sample died this way and Dependabot later re-opens them. Is that intended for Dependabot, or should the bot be exempted (`except-author-associations`)? If the assistant drains the queue, this may stop mattering.
9. Should the assistant refuse to merge a dependency bump when another open PR touches the same dependency (the #16768 / #16782 cel-go case)? Proposed as a `no_competing_pr` policy rule — want maintainer confirmation on the desired behavior before implementing.
10. Who would own the `test-map.yaml` path→suite map upstream? Its 37% fallback rate is the main limiter on compute savings, and it needs to live near the tests it maps.
