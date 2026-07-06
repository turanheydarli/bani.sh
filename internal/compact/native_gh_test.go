package compact

import (
	"strings"
	"testing"
	"time"
)

// fixed clock lets us assert stable ages in goldens.
func withFixedNow(t *testing.T) {
	t.Helper()
	prev := nowFn
	nowFn = func() time.Time { return time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC) }
	t.Cleanup(func() { nowFn = prev })
}

const ghPRListJSON = `[
  {"number":42,"title":"feat: shiny thing","state":"OPEN","author":{"login":"alice"},"createdAt":"2026-07-05T10:00:00Z","headRefName":"feat/shiny"},
  {"number":41,"title":"fix: parser edge case","state":"MERGED","author":{"login":"bob"},"createdAt":"2026-07-04T18:30:00Z","headRefName":"fix/parser"},
  {"number":40,"title":"docs: update readme","state":"CLOSED","author":{"login":"carol"},"createdAt":"2026-06-15T08:00:00Z","headRefName":"docs/readme"}
]`

func TestRenderGHPRList(t *testing.T) {
	withFixedNow(t)
	out, ok := renderGHPRList(ghPRListJSON, "", 0)
	if !ok {
		t.Fatal("ok=false")
	}
	for _, want := range []string{
		"NUM", "STATE", "AUTHOR", "BRANCH", "AGE", "TITLE",
		"#42", "OPEN", "alice", "feat/shiny", "shiny thing",
		"#41", "MERGED", "bob", "fix/parser",
		"#40", "CLOSED", "carol",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "createdAt") || strings.Contains(out, `"login"`) {
		t.Errorf("raw JSON keys leaked:\n%s", out)
	}
}

func TestRenderGHPRListEmpty(t *testing.T) {
	out, ok := renderGHPRList("[]", "", 0)
	if !ok {
		t.Fatal("ok=false on empty array")
	}
	if !strings.Contains(out, "no pull requests") {
		t.Errorf("empty-list rendering missing:\n%s", out)
	}
}

func TestRenderGHPRListRejectsRunListShape(t *testing.T) {
	// A run-list array has databaseId, not number. renderGHPRList must
	// decline so the run-list renderer gets its turn.
	runShape := `[{"databaseId":9999,"name":"CI","status":"completed"}]`
	if _, ok := renderGHPRList(runShape, "", 0); ok {
		t.Error("PR renderer should decline run-list shape")
	}
}

func TestRenderGHPRListFallsThroughOnHumanTable(t *testing.T) {
	human := "ID   TITLE                       BRANCH       CREATED\n42   feat: shiny thing           feat/shiny   2d"
	if _, ok := renderGHPRList(human, "", 0); ok {
		t.Error("human table must not parse as JSON")
	}
}

const ghRunListJSON = `[
  {"databaseId":9001,"name":"CI","status":"completed","conclusion":"success","displayTitle":"Merge PR #42","createdAt":"2026-07-07T09:00:00Z"},
  {"databaseId":9000,"name":"CI","status":"completed","conclusion":"failure","displayTitle":"feat: broken change","createdAt":"2026-07-06T18:00:00Z"},
  {"databaseId":8999,"name":"Release","status":"in_progress","conclusion":null,"displayTitle":"Publish v1.2.3","createdAt":"2026-07-07T11:45:00Z"}
]`

func TestRenderGHRunList(t *testing.T) {
	withFixedNow(t)
	out, ok := renderGHRunList(ghRunListJSON, "", 0)
	if !ok {
		t.Fatal("ok=false")
	}
	for _, want := range []string{
		"ID", "STATUS", "CONCLUSION", "WORKFLOW", "AGE", "TITLE",
		"9001", "success", "CI", "Merge PR #42",
		"9000", "failure",
		"8999", "in_progress", "-", "Release",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	// null conclusion must render as "-", not the literal "null" JSON token.
	if strings.Contains(out, "null") {
		t.Errorf("null leaked into output:\n%s", out)
	}
}

func TestGhAgeUnitsAndFallback(t *testing.T) {
	withFixedNow(t)
	cases := map[string]string{
		"2026-07-07T11:59:30Z": "just now",
		"2026-07-07T11:30:00Z": "30m",
		"2026-07-07T09:00:00Z": "3h",
		"2026-07-05T12:00:00Z": "2d",
		"2026-04-01T12:00:00Z": "3mo",
		"":                     "-",
		"not-a-time":           "not-a-time",
	}
	for in, want := range cases {
		if got := ghAge(in); got != want {
			t.Errorf("ghAge(%q) = %q, want %q", in, got, want)
		}
	}
}
