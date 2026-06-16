package history

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBaseCommand(t *testing.T) {
	cases := map[string]string{
		"git status":                    "git",
		"/usr/bin/git status":           "git",
		"FOO=bar npm install":           "npm",
		"banish run -e 'go test ./...'":  "go",
		`banish "kubectl get pods"`:      "kubectl",
		"":                              "",
	}
	for in, want := range cases {
		if got := BaseCommand(in); got != want {
			t.Errorf("BaseCommand(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCommandsParsesAndLinksErrors(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".claude", "projects", "proj")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}

	lines := []string{
		`{"type":"assistant","timestamp":"2026-06-12T07:30:46.000Z","sessionId":"s1","message":{"content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"git statuss"}}]}}`,
		`{"type":"user","timestamp":"2026-06-12T07:30:47.000Z","sessionId":"s1","message":{"content":[{"type":"tool_result","tool_use_id":"t1","is_error":true}]}}`,
		`{"type":"assistant","timestamp":"2026-06-12T07:30:48.000Z","sessionId":"s1","message":{"content":[{"type":"tool_use","id":"t2","name":"Bash","input":{"command":"git status"}}]}}`,
		`{"type":"user","timestamp":"2026-06-12T07:30:49.000Z","sessionId":"s1","message":{"content":[{"type":"tool_result","tool_use_id":"t2","is_error":false}]}}`,
		`{"type":"user","message":{"content":"a plain string, not a block array"}}`,
	}
	if err := os.WriteFile(filepath.Join(dir, "s1.jsonl"), []byte(strings.Join(lines, "\n")+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cmds, err := Commands()
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 2 {
		t.Fatalf("got %d commands, want 2", len(cmds))
	}
	if cmds[0].Raw != "git statuss" || !cmds[0].IsError || cmds[0].Base != "git" {
		t.Errorf("cmd0 = %+v, want raw=git statuss, error=true, base=git", cmds[0])
	}
	if cmds[1].Raw != "git status" || cmds[1].IsError {
		t.Errorf("cmd1 = %+v, want raw=git status, error=false", cmds[1])
	}
}
