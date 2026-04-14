package analyzer

import (
	"os"
	"path/filepath"
	"testing"
)

func setupTestHome(t *testing.T) func() {
	t.Helper()
	origHome := os.Getenv("HOME")
	tmpHome := t.TempDir()
	os.Setenv("HOME", tmpHome)
	os.MkdirAll(filepath.Join(tmpHome, ".banish", "ext"), 0755)
	return func() { os.Setenv("HOME", origHome) }
}

func TestSuggestBelowThreshold(t *testing.T) {
	cleanup := setupTestHome(t)
	defer cleanup()

	a := New()
	for i := 0; i < 3; i++ {
		a.Track(Entry{Command: "staticcheck ./...", InputToks: 5})
	}

	s := a.SuggestExtension("staticcheck ./...", map[string]bool{"ls": true})
	if s != nil {
		t.Error("expected nil below threshold")
	}
}

func TestSuggestAtThreshold(t *testing.T) {
	cleanup := setupTestHome(t)
	defer cleanup()

	a := New()
	for i := 0; i < 6; i++ {
		a.Track(Entry{Command: "staticcheck ./...", InputToks: 5, OutputToks: 40})
	}

	s := a.SuggestExtension("staticcheck ./...", map[string]bool{"ls": true})
	if s == nil {
		t.Fatal("expected suggestion at threshold")
	}
	if s.Command != "staticcheck" {
		t.Errorf("command = %q, want staticcheck", s.Command)
	}
	if !s.Confirm {
		t.Error("confirm should always be true")
	}
}

func TestSuggestSkipsBuiltins(t *testing.T) {
	cleanup := setupTestHome(t)
	defer cleanup()

	a := New()
	for i := 0; i < 10; i++ {
		a.Track(Entry{Command: "ls /tmp", InputToks: 3, OutputToks: 10})
	}

	s := a.SuggestExtension("ls /tmp", map[string]bool{"ls": true})
	if s != nil {
		t.Error("should not suggest for builtin")
	}
}

func TestSuggestSkipsExistingExtension(t *testing.T) {
	cleanup := setupTestHome(t)
	defer cleanup()

	// Create an extension file for "mycheck"
	home := os.Getenv("HOME")
	os.WriteFile(filepath.Join(home, ".banish", "ext", "mycheck.bsh"), []byte("!extension mycheck v:1.0\n"), 0644)

	a := New()
	for i := 0; i < 10; i++ {
		a.Track(Entry{Command: "mycheck ./...", InputToks: 5, OutputToks: 40})
	}

	s := a.SuggestExtension("mycheck ./...", map[string]bool{})
	if s != nil {
		t.Error("should not suggest when extension file exists")
	}
}

func TestSuggestCooldown(t *testing.T) {
	cleanup := setupTestHome(t)
	defer cleanup()

	a := New()
	for i := 0; i < 10; i++ {
		a.Track(Entry{Command: "slowtool ./...", InputToks: 5, OutputToks: 40})
	}

	// First suggestion should fire
	s1 := a.SuggestExtension("slowtool ./...", map[string]bool{})
	if s1 == nil {
		t.Fatal("expected first suggestion")
	}

	// Second suggestion should NOT fire (in cooldown)
	s2 := a.SuggestExtension("slowtool ./...", map[string]bool{})
	if s2 != nil {
		t.Error("expected nil during cooldown")
	}
}

func TestSuggestNormalizesPath(t *testing.T) {
	cleanup := setupTestHome(t)
	defer cleanup()

	a := New()
	// Track with full path -- should normalize to "git"
	for i := 0; i < 10; i++ {
		a.Track(Entry{Command: "/usr/bin/git status", InputToks: 5, OutputToks: 40})
	}

	// Should suggest for "git", not "/usr/bin/git"
	s := a.SuggestExtension("/usr/bin/git status", map[string]bool{})
	if s == nil {
		t.Fatal("expected suggestion")
	}
	if s.Command != "git" {
		t.Errorf("command = %q, want git (normalized)", s.Command)
	}
}

func TestSuggestAcceptedNeverAgain(t *testing.T) {
	cleanup := setupTestHome(t)
	defer cleanup()

	a := New()
	for i := 0; i < 10; i++ {
		a.Track(Entry{Command: "mytool run", InputToks: 5, OutputToks: 40})
	}

	// First suggestion fires
	s := a.SuggestExtension("mytool run", map[string]bool{})
	if s == nil {
		t.Fatal("expected suggestion")
	}

	// Mark accepted
	MarkAccepted("mytool run")

	// Should never suggest again
	s2 := a.SuggestExtension("mytool run", map[string]bool{})
	if s2 != nil {
		t.Error("should not suggest after accepted")
	}
}

func TestNormalizeCommand(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"staticcheck ./...", "staticcheck"},
		{"/usr/bin/git status", "git"},
		{"/usr/local/bin/node script.js", "node"},
		{"./build.sh", "build.sh"},
		{"ls /tmp", "ls"},
		{"", ""},
	}

	for _, tt := range tests {
		got := normalizeCommand(tt.input)
		if got != tt.want {
			t.Errorf("normalizeCommand(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
