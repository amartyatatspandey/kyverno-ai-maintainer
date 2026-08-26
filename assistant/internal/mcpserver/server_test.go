package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/amartyatatspandey/kyverno-ai-maintainer/internal/ghx"
	"github.com/amartyatatspandey/kyverno-ai-maintainer/internal/intel"
	"github.com/amartyatatspandey/kyverno-ai-maintainer/internal/policy"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type fakeGH struct {
	mu       sync.Mutex
	pr       *policy.PRFacts
	issue    *ghx.IssueView
	diff     string
	merges   int
	comments int
	labels   int
}

func (f *fakeGH) GetPRFacts(number int) (*policy.PRFacts, string, error) {
	p := *f.pr
	p.Number = number
	return &p, "", nil
}
func (f *fakeGH) GetIssue(number int) (*ghx.IssueView, error) {
	if f.issue == nil {
		return &ghx.IssueView{Number: number, Title: "t", Body: "b"}, nil
	}
	return f.issue, nil
}
func (f *fakeGH) GetDiff(number int, maxBytes int) (string, error) { return f.diff, nil }
func (f *fakeGH) UpsertComment(*policy.Decision, string, int, string, string) (string, error) {
	f.mu.Lock()
	f.comments++
	f.mu.Unlock()
	return "commented", nil
}
func (f *fakeGH) SetLabels(*policy.Decision, int, []string, []string) (string, error) {
	f.mu.Lock()
	f.labels++
	f.mu.Unlock()
	return "labeled", nil
}
func (f *fakeGH) MergePR(*policy.Decision, int, string) (string, error) {
	f.mu.Lock()
	f.merges++
	f.mu.Unlock()
	return "merged", nil
}
func (f *fakeGH) KillSwitchActive(string) bool { return false }

func testHost(t *testing.T, gh *fakeGH) *Host {
	t.Helper()
	cfg, err := policy.LoadConfig("../../config/ai-maintainer.yaml")
	if err != nil {
		t.Fatal(err)
	}
	tmap, err := intel.LoadMap("../../config/test-map.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return &Host{
		Engine:   policy.NewEngine(cfg),
		GH:       gh,
		Map:      tmap,
		Cfg:      cfg,
		Repo:     "amartyatatspandey/kyverno",
		AuditDir: t.TempDir(),
		Classify: func(title string) string {
			if strings.Contains(title, "2.0.0") {
				return "major"
			}
			return "patch"
		},
	}
}

func connect(t *testing.T, h *Host) *mcp.ClientSession {
	t.Helper()
	srv := New(h)
	ct, st := mcp.NewInMemoryTransports()
	if _, err := srv.Connect(context.Background(), st, nil); err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v0.0.1"}, nil)
	cs, err := client.Connect(context.Background(), ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

func TestListToolsSchema(t *testing.T) {
	gh := &fakeGH{pr: &policy.PRFacts{Number: 1}}
	cs := connect(t, testHost(t, gh))
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		ToolGetPR, ToolGetDiff, ToolGetIssue, ToolGetAffectedTests,
		ToolComment, ToolSetLabels, ToolMergePR,
	}
	got := map[string]*mcp.Tool{}
	for _, tl := range res.Tools {
		got[tl.Name] = tl
	}
	for _, name := range want {
		tl, ok := got[name]
		if !ok {
			t.Fatalf("missing tool %s; have %v", name, keys(got))
		}
		if tl.InputSchema == nil {
			t.Fatalf("tool %s has no input schema", name)
		}
	}
}

func TestReadOnlyGetAffectedTestsRoundTrip(t *testing.T) {
	gh := &fakeGH{pr: &policy.PRFacts{Number: 1}}
	cs := connect(t, testHost(t, gh))
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      ToolGetAffectedTests,
		Arguments: map[string]any{"changed_files": []string{"pkg/engine/engine.go"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("read-only call failed: %+v", res)
	}
	text := toolText(t, res)
	var sel intel.Selection
	if err := json.Unmarshal([]byte(text), &sel); err != nil {
		t.Fatalf("selection json: %v\n%s", err, text)
	}
	if len(sel.Suites) == 0 {
		t.Fatalf("expected suites for pkg/engine, got %+v", sel)
	}
	found := false
	for _, s := range sel.Suites {
		if s == "mutate" {
			found = true
		}
	}
	if !found {
		t.Fatalf("pkg/engine should select mutate, got %v", sel.Suites)
	}
}

func TestMergeRefusedWithoutPolicyAuthorization(t *testing.T) {
	// Human-authored PR: same golden case as engine_test human_author_denied.
	gh := &fakeGH{pr: &policy.PRFacts{
		Number: 1, AuthorLogin: "someuser", AuthorIsBot: false,
		BaseRef: "main", HeadSHA: "aa", ChecksGreen: true, Mergeable: true,
		State: "OPEN", ChangedFiles: []string{"go.mod", "go.sum"}, UpdateType: "patch",
		Title: "chore(deps): bump github.com/foo/bar from 1.0.0 to 1.0.1",
	}}
	cs := connect(t, testHost(t, gh))
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      ToolMergePR,
		Arguments: map[string]any{"number": 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatal("human-authored merge must be a tool error (policy DENY), not a protocol error")
	}
	msg := toolText(t, res)
	if !strings.Contains(msg, "author_allowlisted") && !strings.Contains(msg, "policy DENY") {
		t.Fatalf("want policy DENY naming author_allowlisted, got %q", msg)
	}
	if gh.merges != 0 {
		t.Fatalf("MergePR must not run after DENY, got %d calls", gh.merges)
	}
}

func TestMergeRefusedOnMajorBump(t *testing.T) {
	gh := &fakeGH{pr: &policy.PRFacts{
		Number: 2, AuthorLogin: "app/dependabot", AuthorIsBot: true,
		BaseRef: "main", HeadSHA: "bb", ChecksGreen: true, Mergeable: true,
		State: "OPEN", ChangedFiles: []string{"go.mod", "go.sum"},
		Title:     "chore(deps): bump github.com/foo/bar from 1.0.0 to 2.0.0",
		CreatedAt: time.Now(),
	}}
	cs := connect(t, testHost(t, gh))
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      ToolMergePR,
		Arguments: map[string]any{"number": 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatal("major bump must be DENY")
	}
	if gh.merges != 0 {
		t.Fatal("MergePR must not run on major bump")
	}
}

func toolText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

func keys(m map[string]*mcp.Tool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
