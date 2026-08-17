package ghx

import (
	"testing"

	"github.com/amartyatatspandey/kyverno-ai-maintainer/internal/policy"
)

func TestDiscussionsFromGraphQLJSON(t *testing.T) {
	raw := []byte(`{
	  "data": {
	    "repository": {
	      "discussions": {
	        "nodes": [
	          {
	            "number": 10,
	            "category": {"name": "Q&A"},
	            "answer": null,
	            "comments": {"nodes": [{"author": {"login": "dependabot", "__typename": "Bot"}}]}
	          },
	          {
	            "number": 11,
	            "category": {"name": "General"},
	            "answer": {"id": "D_kw"},
	            "comments": {"nodes": []}
	          },
	          {
	            "number": 12,
	            "category": {"name": "Q&A"},
	            "answer": null,
	            "comments": {"nodes": [{"author": {"login": "alice", "__typename": "User"}}]}
	          }
	        ]
	      }
	    }
	  }
	}`)
	got, err := discussionsFromGraphQLJSON(raw, "Q&A")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("category filter: got %d want 2 Q&A", len(got))
	}
	byNum := map[int]policy.DiscussionFacts{}
	for _, d := range got {
		byNum[d.Number] = d
	}
	if byNum[10].AnsweredByHuman {
		t.Fatal("bot-only thread is not answered by a human")
	}
	if !byNum[12].AnsweredByHuman {
		t.Fatal("a User comment means a human already replied")
	}
	all, err := discussionsFromGraphQLJSON(raw, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("empty category returns all, got %d", len(all))
	}
	if !all[1].AnsweredByHuman {
		t.Fatal("accepted answer counts as answered by human")
	}
}

func TestDiscussionBodyFromGraphQLJSON(t *testing.T) {
	raw := []byte(`{"data":{"repository":{"discussion":{"body":"hello docs"}}}}`)
	got, err := discussionBodyFromGraphQLJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got != "hello docs" {
		t.Fatalf("got %q", got)
	}
}

func TestPostDiscussionCommentFailClosed(t *testing.T) {
	c := &Client{Repo: "o/r"}
	if _, err := c.PostDiscussionComment(nil, 1, "hi"); err == nil {
		t.Fatal("nil decision must fail closed")
	}
	denied := &policy.Decision{Allowed: false}
	if _, err := c.PostDiscussionComment(denied, 1, "hi"); err == nil {
		t.Fatal("DENY decision must fail closed")
	}
}
