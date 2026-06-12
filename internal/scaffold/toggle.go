package scaffold

import (
	"encoding/json"
	"fmt"
	"os"
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

// Status reports the active state of banish per supported agent.
func Status() (map[string]AgentState, error) {
	home, err := homeDir()
	if err != nil {
		return nil, err
	}

	state := AgentState{}
	if _, err := os.Stat(claudeHookPath(home)); err == nil {
		state.Hook = true
	}
	settings, err := loadClaudeSettings(home)
	if err != nil {
		return nil, err
	}
	state.Active = state.Hook && hasBanishHook(settings)
	state.MCP = mcpHasBanish(globalMCPPath(home))

	return map[string]AgentState{"claude-code": state}, nil
}

func stopClaudeCode() error {
	home, err := homeDir()
	if err != nil {
		return err
	}
	// Nothing installed: stopping is a no-op (do not create an empty settings file).
	if _, err := os.Stat(claudeSettingsPath(home)); os.IsNotExist(err) {
		return nil
	}
	settings, err := loadClaudeSettings(home)
	if err != nil {
		return err
	}
	if !stripBanishHook(settings) {
		return nil // already stopped
	}
	return saveClaudeSettings(home, settings)
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
