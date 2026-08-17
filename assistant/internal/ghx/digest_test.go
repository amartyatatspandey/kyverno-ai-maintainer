package ghx

import (
	"testing"
	"time"

	"github.com/amartyatatspandey/kyverno-ai-maintainer/internal/policy"
)

func TestPRFactsFromListJSON(t *testing.T) {
	raw := []byte(`[
	  {
	    "number": 10,
	    "title": "fix: widget",
	    "state": "OPEN",
	    "author": {"login": "alice", "is_bot": false},
	    "authorAssociation": "MEMBER",
	    "baseRefName": "main",
	    "headRefOid": "abc",
	    "isDraft": false,
	    "mergeable": "MERGEABLE",
	    "labels": [{"name": "bug"}],
	    "files": [{"path": "pkg/foo.go"}],
	    "statusCheckRollup": [{"name": "ci", "status": "COMPLETED", "conclusion": "SUCCESS"}],
	    "createdAt": "2026-08-01T00:00:00Z"
	  }
	]`)
	got, err := prFactsFromListJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Number != 10 || got[0].AuthorLogin != "alice" {
		t.Fatalf("got %+v", got)
	}
	if !got[0].ChecksGreen {
		t.Fatal("successful check should be green")
	}
	if got[0].CreatedAt.IsZero() {
		t.Fatal("createdAt must be parsed for age math")
	}
}

func TestIssueCountFromJSON(t *testing.T) {
	raw := []byte(`[{"number":1},{"number":2},{"number":3}]`)
	n, err := issueCountFromJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("got %d want 3", n)
	}
}

func TestFailureRatesFromJSON(t *testing.T) {
	now := time.Date(2026, 8, 17, 0, 0, 0, 0, time.UTC)
	raw := []byte(`[
	  {"name":"conformance","conclusion":"failure","createdAt":"2026-08-16T00:00:00Z"},
	  {"name":"conformance","conclusion":"success","createdAt":"2026-08-16T01:00:00Z"},
	  {"name":"lint","conclusion":"failure","createdAt":"2026-08-16T02:00:00Z"},
	  {"name":"lint","conclusion":"failure","createdAt":"2026-08-01T00:00:00Z"}
	]`)
	got, err := failureRatesFromJSON(raw, now.Add(-7*24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if got["conformance"] != 0.5 {
		t.Fatalf("conformance rate=%v want 0.5", got["conformance"])
	}
	if got["lint"] != 1.0 {
		t.Fatalf("lint rate=%v want 1.0 (old run excluded)", got["lint"])
	}
}

func TestRecordsFromJobsJSON(t *testing.T) {
	raw := []byte(`{"jobs":[
	  {"name":"assert (v1.33.7)","conclusion":"failure","startedAt":"2026-08-16T00:00:00Z"},
	  {"name":"mutate (v1.33.7)","conclusion":"success","startedAt":"2026-08-16T00:01:00Z"},
	  {"name":"lint","conclusion":"skipped","startedAt":"2026-08-16T00:02:00Z"}
	]}`)
	created := time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)
	got, err := recordsFromJobsJSON("abc123", created, raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d records want 3", len(got))
	}
	if got[0].SHA != "abc123" || got[0].JobName != "assert (v1.33.7)" || got[0].Conclusion != "failure" {
		t.Fatalf("first record %+v", got[0])
	}
	if got[1].JobName != "mutate (v1.33.7)" {
		t.Fatalf("second job %q", got[1].JobName)
	}
}

func TestPRFactsFromView_ReusesGetPRFactsShape(t *testing.T) {
	v := prView{
		Number: 1, Title: "t", State: "OPEN",
		BaseRefName: "main", HeadRefOid: "deadbeef",
	}
	v.Author.Login = "dependabot"
	v.Author.IsBot = true
	f := prFactsFromView(v)
	if f.AuthorLogin != "app/dependabot" || !f.AuthorIsBot {
		t.Fatalf("bot normalize must match GetPRFacts: %+v", f)
	}
	_ = policy.PRFacts{} // type used by ListOpenPRs
}
