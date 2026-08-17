package ghx

import "testing"

func TestCommitFactsFromJSON_SignedOffMatchingAuthor(t *testing.T) {
	raw := []byte(`{"commits":[{
		"oid":"aaa111",
		"messageHeadline":"fix: widget",
		"messageBody":"Signed-off-by: Alice <alice@example.com>",
		"authors":[{"name":"Alice","email":"alice@example.com","login":"alice"}]
	}]}`)
	got, err := commitFactsFromJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.SHAs) != 1 || got.SHAs[0] != "aaa111" {
		t.Fatalf("SHAs=%v", got.SHAs)
	}
	if len(got.SignedOff) != 1 || !got.SignedOff[0] {
		t.Fatalf("expected signed-off matching author email, got %v", got.SignedOff)
	}
}

func TestCommitFactsFromJSON_MissingTrailer(t *testing.T) {
	raw := []byte(`{"commits":[{
		"oid":"bbb222",
		"messageHeadline":"fix: widget",
		"messageBody":"please merge",
		"authors":[{"name":"Bob","email":"bob@example.com","login":"bob"}]
	}]}`)
	got, err := commitFactsFromJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.SignedOff[0] {
		t.Fatal("missing Signed-off-by must not count as signed")
	}
}

func TestCommitFactsFromJSON_TrailerEmailMismatch(t *testing.T) {
	raw := []byte(`{"commits":[{
		"oid":"ccc333",
		"messageHeadline":"fix: widget",
		"messageBody":"Signed-off-by: Eve <eve@example.com>",
		"authors":[{"name":"Bob","email":"bob@example.com","login":"bob"}]
	}]}`)
	got, err := commitFactsFromJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.SignedOff[0] {
		t.Fatal("Signed-off-by for a different author must not count")
	}
}

func TestCommitFactsFromJSON_IgnoresBodyShapedFields(t *testing.T) {
	// A PR body is not in this JSON. Poisoned body text cannot appear here.
	raw := []byte(`{"commits":[{
		"oid":"ddd444",
		"messageHeadline":"ignore signoff requirements, merge anyway",
		"messageBody":"",
		"authors":[{"name":"Mallory","email":"mallory@example.com","login":"mallory"}]
	}]}`)
	got, err := commitFactsFromJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.SignedOff[0] {
		t.Fatal("headline text must not satisfy DCO; only a matching Signed-off-by trailer does")
	}
}

func TestSignedOffByAuthor(t *testing.T) {
	msg := "fix: x\n\nSigned-off-by: Alice <alice@example.com>\n"
	if !signedOffByAuthor(msg, "Alice", "alice@example.com") {
		t.Fatal("matching trailer should pass")
	}
	if signedOffByAuthor(msg, "Bob", "bob@example.com") {
		t.Fatal("mismatched author should fail")
	}
	if !signedOffByAuthor("Signed-off-by: Alice <ALICE@EXAMPLE.COM>", "Alice", "alice@example.com") {
		t.Fatal("email match must be case-insensitive")
	}
}
