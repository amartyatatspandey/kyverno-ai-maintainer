package policy

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config mirrors .github/ai-maintainer.yaml (POLICY_ENGINE.md).
type Config struct {
	Version    int  `yaml:"version"`
	Enabled    bool `yaml:"enabled"`
	KillSwitch struct {
		Label        string `yaml:"label"`
		RepoVariable string `yaml:"repo_variable"`
	} `yaml:"kill_switch"`
	RateLimits struct {
		MergesPerDay            int `yaml:"merges_per_day"`
		CommentsPerEntityPerDay int `yaml:"comments_per_entity_per_day"`
		LabelOpsPerEntityPerDay int `yaml:"label_ops_per_entity_per_day"`
		SandboxRunsPerDay       int `yaml:"sandbox_runs_per_day"`
	} `yaml:"rate_limits"`
	Workflows map[string]struct {
		Enabled      bool   `yaml:"enabled"`
		AdvisoryOnly bool   `yaml:"advisory_only"`
		Mode         string `yaml:"mode"` // advisory | approval | autonomous
	} `yaml:"workflows"`
	Branches struct {
		MergeTargetsAllowed []string `yaml:"merge_targets_allowed"`
	} `yaml:"branches"`
	ProtectedPaths []string `yaml:"protected_paths"`
	GeneratedPaths []string `yaml:"generated_paths"`
	AutoMerge      struct {
		AllowedAuthors        []string `yaml:"allowed_authors"`
		UpdateTypes           []string `yaml:"update_types"`
		ChangedFilesMustMatch []string `yaml:"changed_files_must_match"`
		DenyLabels            []string `yaml:"deny_labels"`
		MinAgeHours           int      `yaml:"min_age_hours"`
		Method                string   `yaml:"method"`
	} `yaml:"auto_merge"`
	GitHubOps map[string][]string `yaml:"github_ops"`
	Labels    struct {
		AssignableDenylist []string `yaml:"assignable_denylist"`
		NeverRemove        []string `yaml:"never_remove"`
	} `yaml:"labels"`
}

// LoadConfig parses and validates. Any error must halt the assistant
// (invariant 2: config error => global deny).
func LoadConfig(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config unreadable (global halt): %w", err)
	}
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("config parse error (global halt): %w", err)
	}
	if c.Version != 1 {
		return nil, fmt.Errorf("unsupported config version %d (global halt)", c.Version)
	}
	return &c, nil
}
