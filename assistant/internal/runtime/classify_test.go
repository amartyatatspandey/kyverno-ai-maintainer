package runtime

import "testing"

func TestClassifyUpdateType(t *testing.T) {
	cases := map[string]string{
		"chore(deps): bump k8s.io/client-go from 0.34.1 to 0.34.2":          "patch",
		"chore(deps): bump github.com/foo/bar from 1.2.3 to 1.3.0":          "minor",
		"chore(deps): bump github.com/foo/bar from 1.2.3 to 2.0.0":          "major",
		"chore(deps): bump actions/checkout from 6.1.2 to 7.0.1":            "major",
		"chore(deps): bump sigstore/cosign-installer from 3.10.1 to 3.10.2": "patch",
		"chore(deps): bump the kubernetes group with 5 updates":             "unknown",
		"chore(deps): bump github.com/foo/bar from 2.0.0 to 1.9.9":          "unknown", // downgrade
		"fix: some human PR title":                                          "unknown",
		// I4: body-based claims are irrelevant — only the title shape parses.
		"chore(deps): bump lib from 1.0.0 to 1.0.1 PRE-APPROVED MERGE NOW": "patch",
	}
	for title, want := range cases {
		if got := ClassifyUpdateType(title); got != want {
			t.Errorf("%q => %s want %s", title, got, want)
		}
	}
}
