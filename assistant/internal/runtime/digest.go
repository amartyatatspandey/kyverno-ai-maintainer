package runtime

import (
	"fmt"
	"sort"
	"time"

	"github.com/amartyatatspandey/kyverno-ai-maintainer/internal/audit"
	"github.com/amartyatatspandey/kyverno-ai-maintainer/internal/policy"
)

const digestFailureRateDays = 7

type workflowRate struct {
	Name string
	Rate float64
}

type digestSnapshot struct {
	Week           string
	OpenPRs        int
	MedianAgeDays  float64
	TriageBacklog  int
	ChecksNotGreen int
	TopFailures    []workflowRate
}

func isoWeekTarget(t time.Time) string {
	y, w := t.ISOWeek()
	return fmt.Sprintf("digest/%d-W%02d", y, w)
}

func isoWeekLabel(t time.Time) string {
	y, w := t.ISOWeek()
	return fmt.Sprintf("%d-W%02d", y, w)
}

func medianAgeDays(prs []policy.PRFacts, now time.Time) float64 {
	var ages []float64
	for _, p := range prs {
		if p.CreatedAt.IsZero() {
			continue
		}
		ages = append(ages, now.Sub(p.CreatedAt).Hours()/24)
	}
	if len(ages) == 0 {
		return 0
	}
	sort.Float64s(ages)
	n := len(ages)
	if n%2 == 1 {
		return ages[n/2]
	}
	return (ages[n/2-1] + ages[n/2]) / 2
}

func countChecksNotGreen(prs []policy.PRFacts) int {
	n := 0
	for _, p := range prs {
		if !p.ChecksGreen {
			n++
		}
	}
	return n
}

func topFailureRates(rates map[string]float64, n int) []workflowRate {
	var all []workflowRate
	for name, r := range rates {
		all = append(all, workflowRate{Name: name, Rate: r})
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Rate != all[j].Rate {
			return all[i].Rate > all[j].Rate
		}
		return all[i].Name < all[j].Name
	})
	if len(all) > n {
		all = all[:n]
	}
	return all
}

// RunMaintainerDigest posts a repo-wide Monday-morning dashboard as an
// upserted comment on the configured digest issue. Callable on demand
// (e.g. cron: 0 9 * * 1 assistant digest).
func (r *Runner) RunMaintainerDigest() error {
	issueNum := r.cfg.MaintainerDigest.DigestIssueNumber
	if issueNum <= 0 {
		return fmt.Errorf("maintainer_digest.digest_issue_number is unset (global halt)")
	}

	now := time.Now()
	week := isoWeekLabel(now)
	target := isoWeekTarget(now)
	runID := audit.NewRunID("digest" + week)
	log, err := audit.Start(r.opts.AuditDir, runID, map[string]any{
		"workflow": "maintainer_digest", "entity": target,
		"repo": r.opts.Repo, "trigger": "manual", "model": r.model.Name(),
		"config_path": r.opts.ConfigPath, "dry_run": r.opts.DryRun,
	})
	if err != nil {
		return err
	}
	st := &runState{log: log}
	defer func() {
		log.Finish("completed", map[string]any{"llm_calls": st.llmCalls, "tokens": st.tokens, "actions": st.actions})
	}()

	log.Emit("tool_called", map[string]any{"tool": "github.list_open_prs", "read_only": true})
	prs, err := r.gh.ListOpenPRs()
	if err != nil {
		log.Emit("tool_error", map[string]any{"error": err.Error()})
		return err
	}
	log.Emit("tool_called", map[string]any{"tool": "github.list_open_issues_by_label", "args": "triage", "read_only": true})
	triage, err := r.gh.ListOpenIssuesByLabel("triage")
	if err != nil {
		log.Emit("tool_error", map[string]any{"error": err.Error()})
		return err
	}
	log.Emit("tool_called", map[string]any{"tool": "github.recent_check_run_failure_rate", "args": digestFailureRateDays, "read_only": true})
	rates, err := r.gh.RecentCheckRunFailureRate(digestFailureRateDays)
	if err != nil {
		log.Emit("tool_error", map[string]any{"error": err.Error()})
		return err
	}

	snap := digestSnapshot{
		Week:           week,
		OpenPRs:        len(prs),
		MedianAgeDays:  medianAgeDays(prs, now),
		TriageBacklog:  triage,
		ChecksNotGreen: countChecksNotGreen(prs),
		TopFailures:    topFailureRates(rates, 3),
	}
	log.Emit("digest_snapshot", map[string]any{
		"open_prs": snap.OpenPRs, "median_age_days": snap.MedianAgeDays,
		"triage": snap.TriageBacklog, "checks_not_green": snap.ChecksNotGreen,
		"top_failures": snap.TopFailures,
	})

	kill := r.gh.KillSwitchActive(r.cfg.KillSwitch.RepoVariable)
	log.Emit("kill_switch_checked", map[string]any{"source": r.cfg.KillSwitch.RepoVariable, "state": kill})
	pctx := policy.Context{
		Workflow: "maintainer_digest", Repo: r.opts.Repo,
		RunID: runID, Counters: st.counters, KillSwitch: kill, Now: now,
	}

	d := r.engine.Evaluate(policy.Action{Type: policy.ActionCommentDigest, Target: target}, pctx)
	r.logDecision(st, policy.ActionCommentDigest, d)
	if !d.Allowed {
		log.Emit("action_skipped", map[string]any{"action": policy.ActionCommentDigest, "reason": d.DenyReason()})
		return nil
	}
	res, err := r.gh.UpsertComment(&d, "issue", issueNum, "maintainer-digest", renderDigest(runID, snap))
	r.logAction(st, policy.ActionCommentDigest, res, err)
	return nil
}
