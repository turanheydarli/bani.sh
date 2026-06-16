package scaffold

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// AgentState reports whether banish is currently intercepting commands for an
// agent. Active means banish is registered AND its hook script is on disk.
type AgentState struct {
	Active bool `json:"active"`
	Hook   bool `json:"hook"`
	MCP    bool `json:"mcp"`
}

// Stop disables banish interception for the agent WITHOUT uninstalling assets.
// It removes only the banish hook registration; extensions, the hook script,
// MCP config, and CLAUDE.md are left in place so Start is instant. Stop is
// idempotent: stopping an already-stopped agent is a no-op success.
func Stop(agent string) error {
	switch resolveAgent(agent) {
	case "claude-code":
		return stopClaudeCode()
	case "cursor":
		return fmt.Errorf("cursor integration is MCP-only; there is no hook to stop")
	default:
		return fmt.Errorf("unknown agent: %s (use claude-code)", agent)
	}
}

// Start re-enables banish interception for the agent. It reuses the same hook
// registration code path as banish init. Start is idempotent. If required
// assets are missing (the user deleted them), it returns an actionable error
// pointing at banish init rather than half-enabling.
func Start(agent string) error {
	switch resolveAgent(agent) {
	case "claude-code":
		return startClaudeCode()
	case "cursor":
		return fmt.Errorf("cursor integration is MCP-only; there is no hook to start")
	default:
		return fmt.Errorf("unknown agent: %s (use claude-code)", agent)
	}
}

// Status reports the active state of banish per supported agent. banish is
// active if any settings scope (global or the current project) registers a hook
// that routes commands through banish.
func Status() (map[string]AgentState, error) {
	home, err := homeDir()
	if err != nil {
		return nil, err
	}

	state := AgentState{}
	if _, err := os.Stat(claudeHookPath(home)); err == nil {
		state.Hook = true
	}
	for _, sc := range settingsScopes(home) {
		settings, err := loadSettingsFile(sc.path)
		if err != nil {
			return nil, err
		}
		if hasBanishHook(settings, sc.projectDir) {
			state.Active = true
		}
	}
	state.MCP = mcpHasBanish(globalMCPPath(home))

	return map[string]AgentState{"claude-code": state}, nil
}

// stopClaudeCode removes the banish hook from every settings scope (global and
// the current project) without deleting any other assets. Existing files are
// rewritten only when a banish hook was actually present; missing files are
// never created.
func stopClaudeCode() error {
	home, err := homeDir()
	if err != nil {
		return err
	}
	for _, sc := range settingsScopes(home) {
		if _, err := os.Stat(sc.path); os.IsNotExist(err) {
			continue // do not create a settings file just to strip nothing
		}
		settings, err := loadSettingsFile(sc.path)
		if err != nil {
			return err
		}
		if stripBanishHook(settings, sc.projectDir) {
			if err := saveSettingsFile(sc.path, settings); err != nil {
				return err
			}
		}
	}
	return nil
}

// settingsScope is one Claude Code settings file banish may inspect or edit,
// paired with the project directory used to resolve relative hook script paths
// inside it (empty for the global scopes).
type settingsScope struct {
	path       string
	projectDir string
}

// settingsScopes returns every settings file banish should manage: the global
// user files plus, when the working tree has its own .claude directory, the
// project-scoped files Claude Code merges on top of the global ones.
func settingsScopes(home string) []settingsScope {
	scopes := []settingsScope{
		{path: claudeSettingsPath(home)},
		{path: claudeLocalSettingsPath(home)},
	}
	if pdir := projectClaudeDir(home); pdir != "" {
		scopes = append(scopes,
			settingsScope{path: filepath.Join(pdir, ".claude", "settings.json"), projectDir: pdir},
			settingsScope{path: filepath.Join(pdir, ".claude", "settings.local.json"), projectDir: pdir},
		)
	}
	return scopes
}

// claudeLocalSettingsPath is the user-local Claude Code settings override.
func claudeLocalSettingsPath(home string) string {
	return filepath.Join(home, ".claude", "settings.local.json")
}

// projectClaudeDir returns the nearest ancestor of the working directory that
// contains a .claude directory, which is how Claude Code locates project-scoped
// settings. It returns "" when none is found, or when the only match is the
// user's home directory (whose ~/.claude is the global scope, handled
// separately).
func projectClaudeDir(home string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	dir := cwd
	for {
		if dir == home {
			return ""
		}
		if fi, err := os.Stat(filepath.Join(dir, ".claude")); err == nil && fi.IsDir() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func startClaudeCode() error {
	home, err := homeDir()
	if err != nil {
		return err
	}
	if _, err := os.Stat(claudeHookPath(home)); err != nil {
		return fmt.Errorf("banish hook not installed; run: banish init claude-code")
	}
	return registerHook(home)
}

// resolveAgent maps an empty agent argument to the default (claude-code).
func resolveAgent(agent string) string {
	if agent == "" {
		return "claude-code"
	}
	return agent
}

func homeDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", fmt.Errorf("cannot determine home directory")
	}
	return home, nil
}

// mcpHasBanish reports whether the MCP config at path registers the banish server.
func mcpHasBanish(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var cfg map[string]any
	if json.Unmarshal(data, &cfg) != nil {
		return false
	}
	servers, _ := cfg["mcpServers"].(map[string]any)
	_, ok := servers["banish"]
	return ok
}
