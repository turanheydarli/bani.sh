// Package scaffold implements banish init commands that set up project manifests,
// agent hooks, MCP server configuration, and deploy default extensions.
package scaffold

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	globalMCP := filepath.Join(home, ".claude", ".mcp.json")
	if err := writeMCPConfig(globalMCP); err != nil {
		return fmt.Errorf("global MCP config: %w", err)
	}

	// 5. Append banish context to global ~/.claude/CLAUDE.md
	claudeMDPath := filepath.Join(home, ".claude", "CLAUDE.md")
	if err := writeClaudeMD(claudeMDPath); err != nil {
		return err
	}

	return nil
}


// InitCursor sets up banish for Cursor: MCP config + .cursorrules.
func InitCursor(dir string) error {
	home, _ := os.UserHomeDir()
	if home != "" {
		deployExtensions(home)
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

	created := 0
	for name, content := range defaultExtensions {
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

const hookScript = `#!/bin/bash
# banish-hook.sh -- Route bash commands through banish for output compaction.
# Installed by: banish init claude-code

if ! command -v jq &>/dev/null; then exit 0; fi
if ! command -v banish &>/dev/null; then exit 0; fi

INPUT=$(cat)
CMD=$(echo "$INPUT" | jq -r '.tool_input.command // empty')

[ -z "$CMD" ] && exit 0

# Skip: already banish, shell builtins, multi-line scripts, heredocs
case "$CMD" in
  banish*) exit 0 ;;
  cd\ *|export\ *|source\ *|alias\ *|eval\ *) exit 0 ;;
  *$'\n'*) exit 0 ;;
  *"<<"*) exit 0 ;;
  *EOF*) exit 0 ;;
esac

# Wrap: banish "original command"
# Use printf to avoid quote escaping issues
WRAPPED=$(printf 'banish "%s"' "$(echo "$CMD" | sed 's/"/\\"/g')")

echo "$INPUT" | jq -c --arg cmd "$WRAPPED" '{
  hookSpecificOutput: {
    hookEventName: "PreToolUse",
    permissionDecision: "allow",
    permissionDecisionReason: "banish output compaction",
    updatedInput: { command: $cmd }
  }
}'
`

func installHook(home string) error {
	hookDir := filepath.Join(home, ".claude", "hooks")
	os.MkdirAll(hookDir, 0755)

	hookPath := filepath.Join(hookDir, "banish-hook.sh")
	if err := os.WriteFile(hookPath, []byte(hookScript), 0755); err != nil {
		return err
	}
	return nil
}

func registerHook(home string) error {
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	hookPath := filepath.Join(home, ".claude", "hooks", "banish-hook.sh")

	var settings map[string]any

	if data, err := os.ReadFile(settingsPath); err == nil {
		json.Unmarshal(data, &settings)
	}
	if settings == nil {
		settings = make(map[string]any)
	}

	// Remove any existing banish hooks, then add the current one.
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = make(map[string]any)
	}

	pre, _ := hooks["PreToolUse"].([]any)
	var filtered []any
	for _, entry := range pre {
		isBanish := false
		if m, ok := entry.(map[string]any); ok {
			if innerHooks, ok := m["hooks"].([]any); ok {
				for _, h := range innerHooks {
					if hm, ok := h.(map[string]any); ok {
						if cmd, ok := hm["command"].(string); ok {
							if strings.Contains(cmd, "banish") {
								isBanish = true
							}
						}
					}
				}
			}
		}
		if !isBanish {
			filtered = append(filtered, entry)
		}
	}

	// Add current hook
	filtered = append(filtered, map[string]any{
		"matcher": "Bash",
		"hooks": []any{
			map[string]any{
				"type":    "command",
				"command": hookPath,
				"timeout": 30,
			},
		},
	})
	hooks["PreToolUse"] = filtered
	settings["hooks"] = hooks

	out, _ := json.MarshalIndent(settings, "", "  ")
	return os.WriteFile(settingsPath, append(out, '\n'), 0644)
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

const claudeMDContent = `## Banish

banish is installed. Bash commands are automatically routed through banish
for output compaction and token savings.

What banish does:
- Compacts command output (git status, log, diff, ls, find, grep, etc.)
- Tracks token savings (run banish gain to see stats)
- Provides MCP tools (banish_run, banish_ls, banish_read, banish_fetch)
- Auto-exposes extension verbs as MCP tools

Extensions in ~/.banish/ext/ define:
- Verb shortcuts (gs = git status --short, dps = docker ps compact, etc.)
- Output filters (strip noise from git, npm, cargo, docker, kubectl, etc.)

### BANISH file

If a BANISH file exists in the project root, read it for project-specific
verbs and filters.
`

func writeClaudeMD(path string) error {
	if _, err := os.Stat(path); err == nil {
		existing, _ := os.ReadFile(path)
		if strings.Contains(string(existing), "## Banish") {
			return nil
		}
		content := string(existing) + "\n" + claudeMDContent
		return os.WriteFile(path, []byte(content), 0644)
	}
	return os.WriteFile(path, []byte(claudeMDContent), 0644)
}

// --- Cursor ---

const cursorRulesContent = `Bash commands are routed through banish for output compaction and token savings.
banish provides MCP tools and auto-exposes extension verbs.
Read the BANISH file in the project root for project-specific verbs and filters.
`

func writeCursorRules(path string) error {
	if _, err := os.Stat(path); err == nil {
		existing, _ := os.ReadFile(path)
		if strings.Contains(string(existing), "banish") {
			return nil
		}
		content := string(existing) + "\n" + cursorRulesContent
		return os.WriteFile(path, []byte(content), 0644)
	}
	return os.WriteFile(path, []byte(cursorRulesContent), 0644)
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
