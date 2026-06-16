package scaffold

import (
	"os"
	"path/filepath"
	"testing"
)

// withHome isolates the test from the real home directory. os.UserHomeDir
// resolves to $HOME on unix, so overriding it redirects all ~/.claude and
// ~/.banish writes into a temp dir. It also moves the working directory into a
// fresh temp dir: project-scoped hook detection walks up from the cwd, and the
// real repository has its own .claude directory that tests must not touch.
func withHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)

	wd := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(wd); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	return home
}

func TestStopStartRoundTrip(t *testing.T) {
	home := withHome(t)

	if err := InitClaudeCode(home); err != nil {
		t.Fatalf("InitClaudeCode: %v", err)
	}

	st, err := Status()
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !st["claude-code"].Active {
		t.Fatal("expected active after init")
	}

	if err := Stop("claude-code"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	st, _ = Status()
	if st["claude-code"].Active {
		t.Fatal("expected inactive after stop")
	}
	if !st["claude-code"].Hook {
		t.Fatal("stop must not delete the hook script")
	}

	if err := Start("claude-code"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	st, _ = Status()
	if !st["claude-code"].Active {
		t.Fatal("expected active after start")
	}
}

// Default agent is claude-code when the argument is omitted.
func TestStopStartDefaultAgent(t *testing.T) {
	home := withHome(t)
	if err := InitClaudeCode(home); err != nil {
		t.Fatalf("InitClaudeCode: %v", err)
	}
	if err := Stop(""); err != nil {
		t.Fatalf("Stop(\"\"): %v", err)
	}
	st, _ := Status()
	if st["claude-code"].Active {
		t.Fatal("expected inactive after default-agent stop")
	}
	if err := Start(""); err != nil {
		t.Fatalf("Start(\"\"): %v", err)
	}
	st, _ = Status()
	if !st["claude-code"].Active {
		t.Fatal("expected active after default-agent start")
	}
}

// Stop removes the banish hook but leaves every other PreToolUse hook intact.
func TestStopPreservesOtherHooks(t *testing.T) {
	home := withHome(t)
	if err := InitClaudeCode(home); err != nil {
		t.Fatalf("InitClaudeCode: %v", err)
	}

	settings, _ := loadClaudeSettings(home)
	hooks := settings["hooks"].(map[string]any)
	pre := hooks["PreToolUse"].([]any)
	pre = append(pre, map[string]any{
		"matcher": "Write",
		"hooks": []any{
			map[string]any{"type": "command", "command": "/usr/local/bin/other-hook"},
		},
	})
	hooks["PreToolUse"] = pre
	if err := saveClaudeSettings(home, settings); err != nil {
		t.Fatalf("saveClaudeSettings: %v", err)
	}

	if err := Stop("claude-code"); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	settings, _ = loadClaudeSettings(home)
	hooks, _ = settings["hooks"].(map[string]any)
	if hooks == nil {
		t.Fatal("hooks map removed; non-banish hook was lost")
	}
	pre, _ = hooks["PreToolUse"].([]any)
	if len(pre) != 1 {
		t.Fatalf("expected 1 remaining hook, got %d", len(pre))
	}
	if hasBanishHook(settings, "") {
		t.Fatal("banish hook should be removed")
	}
}

// Repeated stop/start calls are safe no-ops that never duplicate the hook.
func TestStopStartIdempotent(t *testing.T) {
	home := withHome(t)
	if err := InitClaudeCode(home); err != nil {
		t.Fatalf("InitClaudeCode: %v", err)
	}

	if err := Stop("claude-code"); err != nil {
		t.Fatalf("first Stop: %v", err)
	}
	if err := Stop("claude-code"); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
	if err := Start("claude-code"); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	if err := Start("claude-code"); err != nil {
		t.Fatalf("second Start: %v", err)
	}

	settings, _ := loadClaudeSettings(home)
	hooks := settings["hooks"].(map[string]any)
	pre := hooks["PreToolUse"].([]any)
	count := 0
	for _, e := range pre {
		if isBanishHookEntry(e, "") {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 banish hook, got %d", count)
	}
}

// Stop on a clean home (nothing installed) is a no-op and creates no files.
func TestStopNothingInstalled(t *testing.T) {
	withHome(t)
	if err := Stop("claude-code"); err != nil {
		t.Fatalf("Stop on clean home should be a no-op: %v", err)
	}
}

// Start fails with an actionable error when the hook was never installed.
func TestStartWithoutInstall(t *testing.T) {
	withHome(t)
	if err := Start("claude-code"); err == nil {
		t.Fatal("expected error when hook is not installed")
	}
}

// Stop removes a project-scoped hook that routes through banish even when the
// wrapper script is not named "banish" (matched by reading its contents).
func TestStopProjectScopedWrapper(t *testing.T) {
	withHome(t)

	proj, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	// A project wrapper hook whose filename does not contain "banish", but
	// whose contents route commands through banish.
	hookDir := filepath.Join(proj, ".claude", "hooks")
	if err := os.MkdirAll(hookDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	script := filepath.Join(hookDir, "wrap-bash.sh")
	if err := os.WriteFile(script, []byte("#!/bin/bash\nbanish run -e \"$CMD\"\n"), 0755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	projSettings := filepath.Join(proj, ".claude", "settings.json")
	cfg := map[string]any{
		"hooks": map[string]any{
			"PreToolUse": []any{
				map[string]any{
					"matcher": "Bash",
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": "$CLAUDE_PROJECT_DIR/.claude/hooks/wrap-bash.sh",
						},
					},
				},
			},
		},
	}
	if err := saveSettingsFile(projSettings, cfg); err != nil {
		t.Fatalf("save project settings: %v", err)
	}

	st, _ := Status()
	if !st["claude-code"].Active {
		t.Fatal("expected active: project wrapper routes through banish")
	}

	if err := Stop("claude-code"); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	got, err := loadSettingsFile(projSettings)
	if err != nil {
		t.Fatalf("reload project settings: %v", err)
	}
	if hasBanishHook(got, proj) {
		t.Fatal("project wrapper hook should be removed after stop")
	}
}

func TestUnknownAgent(t *testing.T) {
	withHome(t)
	if err := Stop("emacs"); err == nil {
		t.Fatal("expected error for unknown agent")
	}
	if err := Start("emacs"); err == nil {
		t.Fatal("expected error for unknown agent")
	}
}
