// Package scaffold implements banish init commands that set up project manifests,
// agent hooks, MCP server configuration, and deploy default extensions.
package scaffold

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.banish.sh/banish/internal/extension"
)

// InitProject creates a starter BANISH file in the given directory.
func InitProject(dir string) error {
	path := filepath.Join(dir, "BANISH")
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("BANISH file already exists")
	}

	projectName := filepath.Base(dir)
	projectType := detectProjectType(dir)

	content := generateBANISH(projectName, projectType)
	return os.WriteFile(path, []byte(content), 0644)
}

// InitClaudeCode does full GLOBAL setup: extensions, hooks, CLAUDE.md.
// Everything goes to ~/  -- works for all projects, all Claude Code sessions.
// Optionally creates .mcp.json in cwd if it looks like a project directory.
func InitClaudeCode(dir string) error {
	home, _ := os.UserHomeDir()
	if home == "" {
		return fmt.Errorf("cannot determine home directory")
	}

	// 1. Deploy default extensions to ~/.banish/ext/
	if _, err := deployExtensions(home); err != nil {
		return fmt.Errorf("deploy extensions: %w", err)
	}

	// 2. Install global hook to ~/.claude/hooks/
	if err := installHook(home); err != nil {
		return fmt.Errorf("install hook: %w", err)
	}

	// 3. Register hook in ~/.claude/settings.json
	if err := registerHook(home); err != nil {
		return fmt.Errorf("register hook: %w", err)
	}

	// 4. Create global MCP config at ~/.claude/.mcp.json
	if err := writeMCPConfig(globalMCPPath(home)); err != nil {
		return fmt.Errorf("global MCP config: %w", err)
	}

	// 5. Append banish context to global ~/.claude/CLAUDE.md
	claudeMDPath := filepath.Join(home, ".claude", "CLAUDE.md")
	if err := writeClaudeMD(claudeMDPath); err != nil {
		return err
	}

	return nil
}


// InitCursor sets up banish for Cursor: the global preToolUse hook (so commands
// are compacted), the project MCP config, and project .cursorrules.
func InitCursor(dir string) error {
	home, _ := os.UserHomeDir()
	if home != "" {
		deployExtensions(home)
		if err := installCursorHook(home); err != nil {
			return fmt.Errorf("install cursor hook: %w", err)
		}
	}

	cursorDir := filepath.Join(dir, ".cursor")
	os.MkdirAll(cursorDir, 0755)

	if err := writeMCPConfig(filepath.Join(cursorDir, "mcp.json")); err != nil {
		return err
	}
	if err := writeCursorRules(filepath.Join(dir, ".cursorrules")); err != nil {
		return err
	}
	return nil
}

// InitMCPOnly writes just the MCP server config.
func InitMCPOnly(dir string) error {
	home, _ := os.UserHomeDir()
	if home != "" {
		deployExtensions(home)
	}
	return writeMCPConfig(filepath.Join(dir, ".mcp.json"))
}

// --- Extension deployment ---

// deployExtensions writes default .bsh extensions to ~/.banish/ext/.
// Skips files that already exist (user may have customized them).
// Returns count of newly created files.
func deployExtensions(home string) (int, error) {
	extDir := filepath.Join(home, ".banish", "ext")
	os.MkdirAll(extDir, 0755)

	defaults, err := extension.Builtin()
	if err != nil {
		return 0, fmt.Errorf("scaffold.deployExtensions: %w", err)
	}

	created := 0
	for name, content := range defaults {
		path := filepath.Join(extDir, name)
		if _, err := os.Stat(path); err == nil {
			continue // don't overwrite user customizations
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return created, err
		}
		created++
	}
	return created, nil
}

// --- Hook installation ---

func installHook(home string) error {
	hookDir := filepath.Join(home, ".claude", "hooks")
	os.MkdirAll(hookDir, 0755)

	script, err := agentAsset("claude-code", "hook.sh")
	if err != nil {
		return err
	}
	return os.WriteFile(claudeHookPath(home), script, 0755)
}

func registerHook(home string) error {
	settings, err := loadClaudeSettings(home)
	if err != nil {
		return err
	}

	// Remove any existing banish hooks, then add the current one.
	stripBanishHook(settings, "")
	addBanishHook(settings, claudeHookPath(home))

	return saveClaudeSettings(home, settings)
}

// claudeHookPath is the canonical location of the banish PreToolUse hook script.
func claudeHookPath(home string) string {
	return filepath.Join(home, ".claude", "hooks", "banish-hook.sh")
}

// claudeSettingsPath is the Claude Code settings file that registers hooks.
func claudeSettingsPath(home string) string {
	return filepath.Join(home, ".claude", "settings.json")
}

// globalMCPPath is the global Claude Code MCP server config.
func globalMCPPath(home string) string {
	return filepath.Join(home, ".claude", ".mcp.json")
}

// loadClaudeSettings reads ~/.claude/settings.json into a map. A missing file
// yields an empty (non-nil) map so callers can add keys unconditionally.
func loadClaudeSettings(home string) (map[string]any, error) {
	return loadSettingsFile(claudeSettingsPath(home))
}

// saveClaudeSettings writes settings back to ~/.claude/settings.json.
func saveClaudeSettings(home string, settings map[string]any) error {
	return saveSettingsFile(claudeSettingsPath(home), settings)
}

// loadSettingsFile reads any Claude Code settings file into a map. A missing
// file yields an empty (non-nil) map so callers can add keys unconditionally.
func loadSettingsFile(path string) (map[string]any, error) {
	settings := make(map[string]any)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return settings, nil
		}
		return nil, fmt.Errorf("scaffold.loadSettingsFile: %w", err)
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, fmt.Errorf("scaffold.loadSettingsFile: parse %s: %w", path, err)
	}
	if settings == nil {
		settings = make(map[string]any)
	}
	return settings, nil
}

// saveSettingsFile writes settings back to the given path, creating parent
// directories as needed.
func saveSettingsFile(path string, settings map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("scaffold.saveSettingsFile: %w", err)
	}
	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("scaffold.saveSettingsFile: %w", err)
	}
	return os.WriteFile(path, append(out, '\n'), 0644)
}

// isBanishHookEntry reports whether a PreToolUse entry routes commands through
// banish. projectDir resolves $CLAUDE_PROJECT_DIR and relative script paths in
// the hook command; it is empty when inspecting a global settings file.
func isBanishHookEntry(entry any, projectDir string) bool {
	m, ok := entry.(map[string]any)
	if !ok {
		return false
	}
	inner, ok := m["hooks"].([]any)
	if !ok {
		return false
	}
	for _, h := range inner {
		hm, ok := h.(map[string]any)
		if !ok {
			continue
		}
		if cmd, ok := hm["command"].(string); ok && commandRoutesThroughBanish(cmd, projectDir) {
			return true
		}
	}
	return false
}

// commandRoutesThroughBanish reports whether a PreToolUse hook command sends
// commands through banish, either by invoking banish directly or by running a
// wrapper script whose contents reference banish. This recognizes banish hooks
// regardless of the wrapper script's filename.
func commandRoutesThroughBanish(cmd, projectDir string) bool {
	if strings.Contains(cmd, "banish") {
		return true
	}
	path := hookScriptPath(cmd, projectDir)
	if path == "" {
		return false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return bytes.Contains(data, []byte("banish"))
}

// hookScriptPath extracts and resolves the script a hook command runs. It
// expands the variables Claude Code injects ($CLAUDE_PROJECT_DIR, $HOME) and
// resolves relative paths against projectDir. It returns "" when the command is
// not a simple script invocation (for example a bare binary name).
func hookScriptPath(cmd, projectDir string) string {
	token := strings.TrimSpace(cmd)
	if token == "" {
		return ""
	}
	if i := strings.IndexAny(token, " \t"); i >= 0 {
		token = token[:i] // the first token is the script path
	}
	home, _ := os.UserHomeDir()
	token = strings.NewReplacer(
		"$CLAUDE_PROJECT_DIR", projectDir,
		"${CLAUDE_PROJECT_DIR}", projectDir,
		"$HOME", home,
		"${HOME}", home,
	).Replace(token)
	if strings.HasPrefix(token, "~/") {
		token = filepath.Join(home, token[2:])
	}
	if !strings.Contains(token, "/") {
		return "" // a bare binary name, not a script file
	}
	if !filepath.IsAbs(token) && projectDir != "" {
		token = filepath.Join(projectDir, token)
	}
	return token
}

// stripBanishHook removes every banish PreToolUse entry, leaving all other
// hooks intact. Empty containers are pruned so settings stays clean. Returns
// true if any banish entry was removed. projectDir resolves hook script paths
// when matching by content; it is empty for the global settings file.
func stripBanishHook(settings map[string]any, projectDir string) bool {
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		return false
	}
	pre, _ := hooks["PreToolUse"].([]any)
	if len(pre) == 0 {
		return false
	}

	var filtered []any
	removed := false
	for _, entry := range pre {
		if isBanishHookEntry(entry, projectDir) {
			removed = true
			continue
		}
		filtered = append(filtered, entry)
	}

	if len(filtered) == 0 {
		delete(hooks, "PreToolUse")
	} else {
		hooks["PreToolUse"] = filtered
	}
	if len(hooks) == 0 {
		delete(settings, "hooks")
	} else {
		settings["hooks"] = hooks
	}
	return removed
}

// addBanishHook appends the banish PreToolUse entry, preserving existing hooks.
func addBanishHook(settings map[string]any, hookPath string) {
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = make(map[string]any)
	}
	pre, _ := hooks["PreToolUse"].([]any)
	pre = append(pre, map[string]any{
		"matcher": "Bash",
		"hooks": []any{
			map[string]any{
				"type":    "command",
				"command": hookPath,
				"timeout": 30,
			},
		},
	})
	hooks["PreToolUse"] = pre
	settings["hooks"] = hooks
}

// hasBanishHook reports whether a banish PreToolUse entry is registered.
// projectDir resolves hook script paths when matching by content; it is empty
// for the global settings file.
func hasBanishHook(settings map[string]any, projectDir string) bool {
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		return false
	}
	pre, _ := hooks["PreToolUse"].([]any)
	for _, entry := range pre {
		if isBanishHookEntry(entry, projectDir) {
			return true
		}
	}
	return false
}

// --- MCP config ---

func writeMCPConfig(path string) error {
	banishBin := findBanishBinary()

	if _, err := os.Stat(path); err == nil {
		data, _ := os.ReadFile(path)
		var existing map[string]any
		json.Unmarshal(data, &existing)
		if existing == nil {
			existing = make(map[string]any)
		}
		servers, _ := existing["mcpServers"].(map[string]any)
		if servers == nil {
			servers = make(map[string]any)
		}
		if _, ok := servers["banish"]; ok {
			return nil
		}
		servers["banish"] = map[string]any{
			"command": banishBin,
			"args":    []string{"serve"},
		}
		existing["mcpServers"] = servers
		out, _ := json.MarshalIndent(existing, "", "  ")
		return os.WriteFile(path, append(out, '\n'), 0644)
	}

	config := map[string]any{
		"mcpServers": map[string]any{
			"banish": map[string]any{
				"command": banishBin,
				"args":    []string{"serve"},
			},
		},
	}
	out, _ := json.MarshalIndent(config, "", "  ")
	return os.WriteFile(path, append(out, '\n'), 0644)
}

func findBanishBinary() string {
	exe, err := os.Executable()
	if err == nil {
		resolved, err := filepath.EvalSymlinks(exe)
		if err == nil {
			return resolved
		}
		return exe
	}
	return "banish"
}

// --- CLAUDE.md ---

func writeClaudeMD(path string) error {
	content, err := agentAsset("claude-code", "awareness.md")
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		existing, _ := os.ReadFile(path)
		if strings.Contains(string(existing), "## Banish") {
			return nil
		}
		return os.WriteFile(path, append(append(existing, '\n'), content...), 0644)
	}
	return os.WriteFile(path, content, 0644)
}

// --- Cursor ---

func writeCursorRules(path string) error {
	content, err := agentAsset("cursor", "rules.md")
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		existing, _ := os.ReadFile(path)
		if strings.Contains(string(existing), "banish") {
			return nil
		}
		return os.WriteFile(path, append(append(existing, '\n'), content...), 0644)
	}
	return os.WriteFile(path, content, 0644)
}

// installCursorHook deploys the Cursor hook script globally and registers it in
// ~/.cursor/hooks.json (shared by the Cursor editor and cursor-cli).
func installCursorHook(home string) error {
	hookDir := filepath.Join(home, ".cursor", "hooks")
	if err := os.MkdirAll(hookDir, 0755); err != nil {
		return err
	}
	script, err := agentAsset("cursor", "hook.sh")
	if err != nil {
		return err
	}
	hookPath := filepath.Join(hookDir, "banish-hook.sh")
	if err := os.WriteFile(hookPath, script, 0755); err != nil {
		return err
	}
	return registerCursorHook(home, hookPath)
}

// registerCursorHook adds the banish preToolUse entry to ~/.cursor/hooks.json,
// merging with any existing config. It is idempotent.
func registerCursorHook(home, hookPath string) error {
	path := filepath.Join(home, ".cursor", "hooks.json")

	root := map[string]any{}
	if data, err := os.ReadFile(path); err == nil && len(bytes.TrimSpace(data)) > 0 {
		if json.Unmarshal(data, &root) != nil {
			root = map[string]any{}
		}
	}
	if _, ok := root["version"]; !ok {
		root["version"] = 1
	}
	hooks, _ := root["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	pre, _ := hooks["preToolUse"].([]any)
	for _, e := range pre {
		if m, ok := e.(map[string]any); ok {
			if c, ok := m["command"].(string); ok && strings.Contains(c, "banish") {
				return nil // already registered
			}
		}
	}
	pre = append(pre, map[string]any{"command": hookPath, "matcher": "Shell"})
	hooks["preToolUse"] = pre
	root["hooks"] = hooks

	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(out, '\n'), 0644)
}

// --- Project detection ---

func detectProjectType(dir string) string {
	checks := map[string]string{
		"go.mod":           "go",
		"package.json":     "node",
		"Cargo.toml":       "rust",
		"pyproject.toml":   "python",
		"requirements.txt": "python",
		"pom.xml":          "java",
		"build.gradle":     "java",
	}
	for file, lang := range checks {
		if _, err := os.Stat(filepath.Join(dir, file)); err == nil {
			return lang
		}
	}
	return "generic"
}

func generateBANISH(name, projectType string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# BANISH -- %s\n\n", name)

	switch projectType {
	case "go":
		b.WriteString("!verb build\n!expand exec go build ./...\n\n")
		b.WriteString("!verb test\n!expand exec go test -race ./...\n\n")
		b.WriteString("!verb lint\n!expand exec go vet ./...\n\n")
	case "node":
		b.WriteString("!verb build\n!expand exec npm run build\n\n")
		b.WriteString("!verb test\n!expand exec npm test\n\n")
		b.WriteString("!verb lint\n!expand exec npm run lint\n\n")
	case "rust":
		b.WriteString("!verb build\n!expand exec cargo build\n\n")
		b.WriteString("!verb test\n!expand exec cargo test\n\n")
		b.WriteString("!verb lint\n!expand exec cargo clippy\n\n")
	case "python":
		b.WriteString("!verb test\n!expand exec pytest\n\n")
		b.WriteString("!verb lint\n!expand exec ruff check .\n\n")
	default:
		b.WriteString("# Add project verbs:\n# !verb build\n# !expand exec make build\n\n")
	}

	b.WriteString("!config\n!timeout \"30s\"\n!output json\n")
	return b.String()
}
