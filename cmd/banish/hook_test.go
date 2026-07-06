package main

import "testing"

func TestCursorOutput(t *testing.T) {
	got := cursorOutput("allow", `banish "git status"`)
	want := `{"permission":"allow","updated_input":{"command":"banish \"git status\""}}`
	if got != want {
		t.Errorf("cursorOutput(allow) = %s, want %s", got, want)
	}
	for _, d := range []string{"ask", "deny", "defer", "skip"} {
		if got := cursorOutput(d, "x"); got != "{}" {
			t.Errorf("cursorOutput(%s) = %s, want {}", d, got)
		}
	}
}

func TestClaudeOutput(t *testing.T) {
	if got := claudeOutput("allow", `banish "git status"`); got == "" {
		t.Error("claudeOutput(allow) should emit an envelope")
	}
	for _, d := range []string{"deny", "defer", "skip"} {
		if got := claudeOutput(d, "x"); got != "" {
			t.Errorf("claudeOutput(%s) = %q, want empty", d, got)
		}
	}
}

func TestIsSubcommand(t *testing.T) {
	tests := []struct {
		arg  string
		want bool
	}{
		{"run", true},
		{"version", true},
		{"--version", true},
		{"-v", true},
		{"echo", false},
		{"ls", false},
		{"--foo", false},
	}
	for _, tc := range tests {
		if got := isSubcommand(tc.arg); got != tc.want {
			t.Errorf("isSubcommand(%q) = %t, want %t", tc.arg, got, tc.want)
		}
	}
}
