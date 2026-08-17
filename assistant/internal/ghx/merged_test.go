package ghx

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestResolveSinceDate_ISODate(t *testing.T) {
	got, err := resolveSinceDate("2026-08-01", "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "2026-08-01" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveSinceDate_GitTag(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-b", "main")
	run("config", "user.email", "t@example.com")
	run("config", "user.name", "Tester")
	run("config", "commit.gpgsign", "false")
	if err := os.WriteFile(dir+"/README", []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "README")
	cmd := exec.Command("git", "commit", "-m", "init")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_DATE=2026-01-15T12:00:00Z",
		"GIT_COMMITTER_DATE=2026-01-15T12:00:00Z",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("commit: %v\n%s", err, out)
	}
	run("tag", "v9.9.9")
	got, err := resolveSinceDate("v9.9.9", dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != "2026-01-15" {
		t.Fatalf("got %q want 2026-01-15", got)
	}
}

func TestMergedSearchQuery(t *testing.T) {
	q := mergedSearchQuery("2026-08-01")
	if !strings.Contains(q, "merged:>=2026-08-01") || !strings.Contains(q, "is:merged") {
		t.Fatalf("query %q", q)
	}
}

func TestResolveSinceDate_RejectsEmpty(t *testing.T) {
	if _, err := resolveSinceDate("", ""); err == nil {
		t.Fatal("empty ref must error")
	}
}

func TestISODateRoundTrip(t *testing.T) {
	tm, err := time.Parse("2006-01-02", "2026-08-01")
	if err != nil {
		t.Fatal(err)
	}
	if tm.Format("2006-01-02") != "2026-08-01" {
		t.Fatal("round trip")
	}
}
