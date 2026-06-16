package scaffold

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallCursorHook(t *testing.T) {
	home := withHome(t)

	if err := installCursorHook(home); err != nil {
		t.Fatalf("installCursorHook: %v", err)
	}

	// The hook script is deployed and executable.
	script := filepath.Join(home, ".cursor", "hooks", "banish-hook.sh")
	fi, err := os.Stat(script)
	if err != nil {
		t.Fatalf("hook script not written: %v", err)
	}
	if fi.Mode().Perm()&0100 == 0 {
		t.Errorf("hook script not executable: %v", fi.Mode())
	}

	// hooks.json registers a Shell preToolUse entry pointing at the script.
	pre := readCursorPreToolUse(t, home)
	if len(pre) != 1 {
		t.Fatalf("preToolUse has %d entries, want 1", len(pre))
	}
	entry := pre[0].(map[string]any)
	if entry["matcher"] != "Shell" {
		t.Errorf("matcher = %v, want Shell", entry["matcher"])
	}
	if entry["command"] != script {
		t.Errorf("command = %v, want %s", entry["command"], script)
	}

	// Re-installing must not duplicate the entry.
	if err := installCursorHook(home); err != nil {
		t.Fatalf("second installCursorHook: %v", err)
	}
	if pre := readCursorPreToolUse(t, home); len(pre) != 1 {
		t.Fatalf("after re-install preToolUse has %d entries, want 1 (not idempotent)", len(pre))
	}
}

func readCursorPreToolUse(t *testing.T, home string) []any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(home, ".cursor", "hooks.json"))
	if err != nil {
		t.Fatalf("read hooks.json: %v", err)
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatalf("parse hooks.json: %v", err)
	}
	hooks, _ := root["hooks"].(map[string]any)
	pre, _ := hooks["preToolUse"].([]any)
	return pre
}
