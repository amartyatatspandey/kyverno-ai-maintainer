// Command assistant is the Kyverno AI Maintainer Assistant POC CLI.
//
//	assistant run --pr N                         # dependency-PR flow
//	assistant run --pr N --workflow dco_check    # DCO sign-off guidance
//	assistant run --pr N --workflow welcome_bot  # first-time contributor welcome
//	assistant run --pr N --workflow reviewer_suggest  # CODEOWNERS / git-log reviewer suggestions
//	assistant run --pr N --workflow policy_lint --sandbox  # kyverno apply/test on policy-library YAML
//	assistant digest            # weekly repo dashboard (cron: 0 9 * * 1)
//	assistant flaky-report      # comment flaky suite candidates (never auto-quarantines)
//	assistant draft-release-notes --since v1.16.0 [--out CHANGELOG.draft.md]
//	assistant audit show <id>   # human-readable decision trace
//	assistant audit why <entity>
//	assistant stop              # kill running sandboxes
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/amartyatatspandey/kyverno-ai-maintainer/internal/audit"
	"github.com/amartyatatspandey/kyverno-ai-maintainer/internal/runtime"
	"github.com/amartyatatspandey/kyverno-ai-maintainer/internal/sandbox"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "run":
		cmdRun(os.Args[2:])
	case "digest":
		cmdDigest(os.Args[2:])
	case "flaky-report":
		cmdFlakyReport(os.Args[2:])
	case "draft-release-notes":
		cmdDraftReleaseNotes(os.Args[2:])
	case "audit":
		cmdAudit(os.Args[2:])
	case "stop":
		if err := sandbox.KillAll(); err != nil {
			fatal(err)
		}
		fmt.Println("killed all ai-maintainer sandboxes")
	default:
		usage()
	}
}

func cmdRun(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	pr := fs.Int("pr", 0, "pull request number")
	issue := fs.Int("issue", 0, "issue number (triage flow)")
	workflow := fs.String("workflow", "", "workflow: dependency_prs (default with --pr), dco_check, welcome_bot, reviewer_suggest, policy_lint")
	repo := fs.String("repo", envOr("AI_REPO", "amartyatatspandey/kyverno"), "owner/name")
	cfg := fs.String("config", "config/ai-maintainer.yaml", "policy config")
	tmap := fs.String("map", "config/test-map.yaml", "path→suite map")
	auditDir := fs.String("audit-dir", "audit", "audit directory")
	repoDir := fs.String("repo-dir", "", "local checkout (enables import closure + sandbox)")
	dry := fs.Bool("dry-run", true, "do not execute mutating GitHub calls")
	sbx := fs.Bool("sandbox", false, "run scoped tests in docker sandbox")
	fs.Parse(args)

	r, err := runtime.New(runtime.Options{
		Repo: *repo, ConfigPath: *cfg, MapPath: *tmap, AuditDir: *auditDir,
		RepoDir: *repoDir, DryRun: *dry, UseSandbox: *sbx,
	})
	if err != nil {
		fatal(err)
	}
	ctx := context.Background()
	switch {
	case *issue > 0:
		err = r.RunIssueTriage(ctx, *issue)
	case *pr > 0:
		switch *workflow {
		case "", "dependency_prs":
			err = r.RunDependencyPR(ctx, *pr)
		case "dco_check":
			err = r.RunDCOCheck(ctx, *pr)
		case "welcome_bot":
			err = r.RunWelcomeBot(ctx, *pr)
		case "reviewer_suggest":
			err = r.RunReviewerSuggest(ctx, *pr)
		case "policy_lint":
			err = r.RunPolicyLint(ctx, *pr)
		default:
			fatal(fmt.Errorf("unknown --workflow %q for --pr (want dependency_prs, dco_check, welcome_bot, reviewer_suggest, or policy_lint)", *workflow))
		}
	default:
		fatal(fmt.Errorf("need --pr N or --issue N"))
	}
	if err != nil {
		fatal(err)
	}
	// Show the trace immediately — the run is only useful if a human can read it.
	ids, _ := audit.ListRuns(*auditDir)
	if len(ids) > 0 {
		evs, _ := audit.ReadEvents(*auditDir, ids[len(ids)-1])
		fmt.Println(audit.Render(evs))
	}
}

// cmdDigest is the repo-wide weekly dashboard. No --pr/--issue target.
// Schedulable via cron, e.g. 0 9 * * 1 assistant digest
func cmdDigest(args []string) {
	fs := flag.NewFlagSet("digest", flag.ExitOnError)
	repo := fs.String("repo", envOr("AI_REPO", "amartyatatspandey/kyverno"), "owner/name")
	cfg := fs.String("config", "config/ai-maintainer.yaml", "policy config")
	tmap := fs.String("map", "config/test-map.yaml", "path→suite map")
	auditDir := fs.String("audit-dir", "audit", "audit directory")
	dry := fs.Bool("dry-run", true, "do not execute mutating GitHub calls")
	fs.Parse(args)

	r, err := runtime.New(runtime.Options{
		Repo: *repo, ConfigPath: *cfg, MapPath: *tmap, AuditDir: *auditDir,
		DryRun: *dry,
	})
	if err != nil {
		fatal(err)
	}
	if err := r.RunMaintainerDigest(); err != nil {
		fatal(err)
	}
	ids, _ := audit.ListRuns(*auditDir)
	if len(ids) > 0 {
		evs, _ := audit.ReadEvents(*auditDir, ids[len(ids)-1])
		fmt.Println(audit.Render(evs))
	}
}

// cmdFlakyReport comments candidate flaky chainsaw suites on the digest issue.
func cmdFlakyReport(args []string) {
	fs := flag.NewFlagSet("flaky-report", flag.ExitOnError)
	repo := fs.String("repo", envOr("AI_REPO", "amartyatatspandey/kyverno"), "owner/name")
	cfg := fs.String("config", "config/ai-maintainer.yaml", "policy config")
	tmap := fs.String("map", "config/test-map.yaml", "path→suite map")
	auditDir := fs.String("audit-dir", "audit", "audit directory")
	dry := fs.Bool("dry-run", true, "do not execute mutating GitHub calls")
	fs.Parse(args)

	r, err := runtime.New(runtime.Options{
		Repo: *repo, ConfigPath: *cfg, MapPath: *tmap, AuditDir: *auditDir,
		DryRun: *dry,
	})
	if err != nil {
		fatal(err)
	}
	if err := r.RunFlakyDetection(); err != nil {
		fatal(err)
	}
	ids, _ := audit.ListRuns(*auditDir)
	if len(ids) > 0 {
		evs, _ := audit.ReadEvents(*auditDir, ids[len(ids)-1])
		fmt.Println(audit.Render(evs))
	}
}

// cmdDraftReleaseNotes writes a local changelog draft. It never mutates GitHub.
func cmdDraftReleaseNotes(args []string) {
	fs := flag.NewFlagSet("draft-release-notes", flag.ExitOnError)
	since := fs.String("since", "", "git tag or YYYY-MM-DD (required)")
	outPath := fs.String("out", "", "write markdown to this local file (default: stdout)")
	repo := fs.String("repo", envOr("AI_REPO", "amartyatatspandey/kyverno"), "owner/name")
	cfg := fs.String("config", "config/ai-maintainer.yaml", "policy config")
	tmap := fs.String("map", "config/test-map.yaml", "path→suite map")
	repoDir := fs.String("repo-dir", "", "local checkout (needed to resolve git tags)")
	fs.Parse(args)
	if *since == "" {
		fatal(fmt.Errorf("need --since <tag-or-YYYY-MM-DD>"))
	}

	r, err := runtime.New(runtime.Options{
		Repo: *repo, ConfigPath: *cfg, MapPath: *tmap, RepoDir: *repoDir,
	})
	if err != nil {
		fatal(err)
	}
	md, err := r.DraftReleaseNotes(*since)
	if err != nil {
		fatal(err)
	}
	if *outPath == "" {
		fmt.Print(md)
		return
	}
	if err := os.WriteFile(*outPath, []byte(md), 0o644); err != nil {
		fatal(err)
	}
	fmt.Fprintf(os.Stderr, "wrote %s\n", *outPath)
}

func cmdAudit(args []string) {
	if len(args) == 0 {
		usage()
	}
	dir := envOr("AI_AUDIT_DIR", "audit")
	switch args[0] {
	case "show":
		ids, err := audit.ListRuns(dir)
		if err != nil {
			fatal(err)
		}
		id := ""
		if len(args) > 1 {
			id = args[1]
		} else if len(ids) > 0 {
			id = ids[len(ids)-1]
		}
		evs, err := audit.ReadEvents(dir, id)
		if err != nil {
			fatal(err)
		}
		fmt.Println(audit.Render(evs))
	case "why":
		if len(args) < 2 {
			fatal(fmt.Errorf("usage: assistant audit why <pr17067|issue123>"))
		}
		needle := strings.NewReplacer("/", "", "#", "").Replace(args[1])
		ids, _ := audit.ListRuns(dir)
		found := false
		for _, id := range ids {
			if strings.Contains(id, needle) {
				evs, _ := audit.ReadEvents(dir, id)
				fmt.Println(audit.Render(evs))
				found = true
			}
		}
		if !found {
			fmt.Printf("no runs found touching %s\n", args[1])
		}
	case "list":
		ids, _ := audit.ListRuns(dir)
		for _, id := range ids {
			fmt.Println(id)
		}
	default:
		usage()
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `Kyverno AI Maintainer Assistant (POC)

  assistant run --pr N [--workflow dependency_prs|dco_check|welcome_bot|reviewer_suggest|policy_lint] [--repo owner/name] [--dry-run=false] [--sandbox] [--repo-dir path]
  assistant run --issue N
  assistant digest [--repo owner/name] [--dry-run=false]   # weekly dashboard; cron: 0 9 * * 1 assistant digest
  assistant flaky-report [--repo owner/name] [--dry-run=false]  # flag flaky suites; never auto-quarantines
  assistant draft-release-notes --since <tag-or-YYYY-MM-DD> [--out file] [--repo-dir path]
  assistant audit list | show [run_id] | why <pr17067>
  assistant stop

Model provider: AI_PROVIDER=anthropic|openai|stub (default stub), AI_MODEL=...
`)
	os.Exit(2)
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
