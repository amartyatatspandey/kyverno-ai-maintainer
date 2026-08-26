// Package policy is the deterministic authorization layer. The LLM proposes;
// this package decides. Deny-by-default: the zero-value Decision is a denial.
package policy

import "time"

// Action type identifiers. Unknown types are denied (deny-by-default).
const (
	ActionCommentDCOGuidance        = "comment_dco_guidance"
	ActionCommentWelcome            = "comment_welcome"
	ActionCommentReviewerSuggestion = "comment_reviewer_suggestion"
	ActionCommentDigest             = "comment_digest"
	ActionCommentReleaseNotesDraft  = "comment_release_notes_draft"
	ActionRunPolicyLint             = "run_policy_lint"
	ActionCommentFlakyReport        = "comment_flaky_report"
	ActionCommentDocsGap            = "comment_docs_gap"
	ActionAnswerDiscussion          = "answer_discussion"
	ActionRunRepro                  = "run_repro"
)

// Action is a proposed mutating operation.
type Action struct {
	Type   string // "merge_pr" | "comment" | "set_labels" | "run_scoped_tests" | "comment_dco_guidance" | "comment_welcome" | "comment_reviewer_suggestion" | "comment_digest" | "comment_release_notes_draft" | "run_policy_lint" | "comment_flaky_report" | "comment_docs_gap" | "answer_discussion" | "run_repro"
	Target string // "pr/17067" | "issue/16992"
	Params map[string]any
}

// PRFacts is assembled fresh from the GitHub API by the engine's caller
// immediately before evaluation (fetch-fresh rule, RISKS P4).
type PRFacts struct {
	Number            int
	AuthorLogin       string
	AuthorIsBot       bool
	Title             string
	BaseRef           string
	HeadSHA           string
	Labels            []string
	ChecksGreen       bool
	ChecksPending     bool
	Mergeable         bool
	ChangedFiles      []string
	UpdateType        string // "patch" | "minor" | "major" | "unknown" — deterministic parse, never LLM
	IsDraft           bool
	State             string
	AuthorAssociation string // FIRST_TIME_CONTRIBUTOR | FIRST_TIMER | CONTRIBUTOR | MEMBER | OWNER
	CreatedAt         time.Time
	// CompetingPRs is other currently-open PR numbers whose changed files
	// overlap this PR's. Populated by the caller (runtime / eval harness),
	// never by Evaluate. Integers only — no titles or bodies (untrusted text
	// cannot reach policy inputs). Empty means none found.
	CompetingPRs []int
}

// IssueFacts for triage actions.
type IssueFacts struct {
	Number int
	Labels []string
	State  string
}

// CommitFacts is assembled from git log trailers (Signed-off-by), never
// from PR body text. Used by the DCO checker.
type CommitFacts struct {
	SHAs      []string
	SignedOff []bool
}

// DiscussionFacts for the W6 Q&A workflow. Scores are assembled by the
// caller (retrieval + LLM); the discussion body never appears here.
type DiscussionFacts struct {
	Number             int
	Category           string
	AnsweredByHuman    bool
	LLMConfidence      float64 // model-reported; not trusted alone
	BestRetrievalScore float64 // deterministic keyword-overlap; policy checks this
}

// Counters holds today's action counts (persisted by the store).
type Counters struct {
	MergesToday         int
	CommentsTodayEntity int
	LabelOpsTodayEntity int
	SandboxRunsToday    int
}

// Context is everything Evaluate may consult. No free text fields:
// untrusted content structurally cannot reach policy inputs.
type Context struct {
	Workflow   string // "dependency_prs" | "scoped_tests" | "issue_triage" | "dco_check" | "welcome_bot" | "reviewer_suggest" | "maintainer_digest" | "release_notes_draft" | "policy_lint" | "flaky_detection" | "docs_gap_detection" | "discussion_qa" | "issue_repro"
	Repo       string
	PR         *PRFacts
	Issue      *IssueFacts
	Commits    *CommitFacts     // nil when the workflow does not inspect commits
	Discussion *DiscussionFacts // nil when the workflow is not discussion Q&A
	// ReproBundleValid is the structured result of repro.ValidateReproBundle.
	// The YAML itself never appears here (untrusted content cannot reach policy).
	ReproBundleValid bool
	RunID            string
	Counters         Counters
	KillSwitch       bool // repo variable, fetched fresh
	Now              time.Time
}

// RuleResult records one rule's verdict — the full trace is the audit story.
type RuleResult struct {
	Rule   string `json:"rule"`
	Pass   bool   `json:"pass"`
	Reason string `json:"reason"`
}

// Decision is the outcome. Allowed==true requires every rule to pass.
type Decision struct {
	Allowed   bool         `json:"allowed"`
	Rules     []RuleResult `json:"rules"`
	BoundSHA  string       `json:"bound_sha,omitempty"`
	ExpiresAt time.Time    `json:"expires_at"`
}

// Deny returns the first failing rule, or "" if allowed.
func (d Decision) DenyReason() string {
	for _, r := range d.Rules {
		if !r.Pass {
			return r.Rule + ": " + r.Reason
		}
	}
	return ""
}
