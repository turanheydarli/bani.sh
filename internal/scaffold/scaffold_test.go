package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitProject(t *testing.T) {
	dir := t.TempDir()
	// Create go.mod to trigger Go detection
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test"), 0644)

	if err := InitProject(dir); err != nil {
		t.Fatalf("InitProject error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "BANISH"))
	if err != nil {
		t.Fatal("BANISH file not created")
	}

	content := string(data)
	if !strings.Contains(content, "!verb build") {
		t.Error("missing build verb")
	}
	if !strings.Contains(content, "go build") {
		t.Error("should detect Go project")
	}
}

func TestInitProjectAlreadyExists(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "BANISH"), []byte("existing"), 0644)

	err := InitProject(dir)
	if err == nil {
		t.Error("expected error for existing BANISH")
	}
}

func TestInitClaudeCode(t *testing.T) {
	// InitClaudeCode writes to ~/.claude/ and ~/.banish/ (global).
	// We test that it completes without error and creates the hook.
	dir := t.TempDir()

	if err := InitClaudeCode(dir); err != nil {
		t.Fatalf("InitClaudeCode error: %v", err)
	}

	home, _ := os.UserHomeDir()

	// Check global CLAUDE.md
	data, _ := os.ReadFile(filepath.Join(home, ".claude", "CLAUDE.md"))
	if !strings.Contains(string(data), "## Banish") {
		t.Error("~/.claude/CLAUDE.md missing banish section")
	}
	if !strings.Contains(string(data), "compaction") && !strings.Contains(string(data), "compacts") {
		t.Error("~/.claude/CLAUDE.md missing compaction documentation")
	}

	// Check hook exists
	hookPath := filepath.Join(home, ".claude", "hooks", "banish-hook.sh")
	if _, err := os.Stat(hookPath); os.IsNotExist(err) {
		t.Error("hook script not created")
	}

	// Check extensions deployed
	extDir := filepath.Join(home, ".banish", "ext")
	entries, _ := os.ReadDir(extDir)
	if len(entries) < 5 {
		t.Errorf("expected at least 5 extensions, got %d", len(entries))
	}
}

func TestInitClaudeCodeIdempotent(t *testing.T) {
	dir := t.TempDir()

	InitClaudeCode(dir)
	InitClaudeCode(dir) // second call should not duplicate

	home, _ := os.UserHomeDir()
	data, _ := os.ReadFile(filepath.Join(home, ".claude", "CLAUDE.md"))
	count := strings.Count(string(data), "## Banish")
	if count != 1 {
		t.Errorf("CLAUDE.md has %d banish sections, want 1", count)
	}
}

func TestInitCursor(t *testing.T) {
	dir := t.TempDir()

	if err := InitCursor(dir); err != nil {
		t.Fatalf("InitCursor error: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, ".cursor", "mcp.json"))
	if !strings.Contains(string(data), "banish") {
		t.Error("cursor mcp.json missing banish")
	}

	data, _ = os.ReadFile(filepath.Join(dir, ".cursorrules"))
	if !strings.Contains(string(data), "banish") {
		t.Error(".cursorrules missing banish")
	}
}

func TestDetectProjectType(t *testing.T) {
	tests := []struct {
		file string
		want string
	}{
		{"go.mod", "go"},
		{"package.json", "node"},
		{"Cargo.toml", "rust"},
		{"pyproject.toml", "python"},
	}

	for _, tt := range tests {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, tt.file), []byte(""), 0644)

		got := detectProjectType(dir)
		if got != tt.want {
			t.Errorf("detect(%s) = %s, want %s", tt.file, got, tt.want)
		}
	}
}

func TestInitMCPOnly(t *testing.T) {
	dir := t.TempDir()

	if err := InitMCPOnly(dir); err != nil {
		t.Fatalf("InitMCPOnly error: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, ".mcp.json"))
	if !strings.Contains(string(data), "banish") {
		t.Error(".mcp.json missing banish")
	}

	// Should NOT create CLAUDE.md
	if _, err := os.Stat(filepath.Join(dir, "CLAUDE.md")); err == nil {
		t.Error("MCP-only should not create CLAUDE.md")
	}
}
