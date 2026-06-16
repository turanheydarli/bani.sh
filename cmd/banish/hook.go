package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"go.bani.sh/banish/internal/permissions"
)

// hookCmd is the PreToolUse hook entry point. It reads the tool input JSON from
// stdin, decides how the command should be handled, and writes the hook output
// JSON to stdout. It never auto-approves a command the host's own rules do not
// already allow.
func hookCmd() *cobra.Command {
	var host string
	cmd := &cobra.Command{
		Use:    "hook",
		Short:  "PreToolUse hook: permission-aware Bash routing through banish",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				return nil // defer silently on any input error
			}
			var in struct {
				ToolInput struct {
					Command string `json:"command"`
				} `json:"tool_input"`
			}
			if json.Unmarshal(data, &in) != nil {
				return nil
			}
			if out := decideHook(in.ToolInput.Command, hostFromString(host)); out != "" {
				fmt.Println(out)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&host, "host", "claude-code", "agent host whose permission rules apply: claude-code or cursor")
	return cmd
}

// hostFromString maps a host flag value to a permissions.Host.
func hostFromString(s string) permissions.Host {
	if s == "cursor" {
		return permissions.HostCursor
	}
	return permissions.HostClaudeCode
}

// decideHook returns the hook output JSON for a command, or "" to defer to the
// host's normal permission flow (which leaves the original command untouched).
func decideHook(cmd string, host permissions.Host) string {
	trimmed := strings.TrimSpace(cmd)
	if trimmed == "" || shouldSkipHook(trimmed) {
		return ""
	}

	verdict := permissions.CheckFor(trimmed, host)

	// Deny: let the host apply its own deny handling on the original command.
	if verdict == permissions.Deny {
		return ""
	}
	// Constructs we cannot safely rewrite are left to the host untouched.
	if permissions.ContainsUnattestable(trimmed) {
		return ""
	}

	decision := "ask"
	if verdict == permissions.Allow {
		decision = "allow"
	}
	return hookOutput(decision, wrapCommand(cmd))
}

// shouldSkipHook reports commands banish should not wrap: already-banish calls,
// stateful shell builtins, multi-line scripts, and heredocs. These are deferred
// so the host handles them normally.
func shouldSkipHook(cmd string) bool {
	if strings.HasPrefix(cmd, "banish") {
		return true
	}
	for _, p := range []string{"cd ", "export ", "source ", "alias ", "eval "} {
		if strings.HasPrefix(cmd, p) {
			return true
		}
	}
	if strings.Contains(cmd, "\n") || strings.Contains(cmd, "<<") {
		return true
	}
	return false
}

// wrapCommand rewrites a command to run through banish for output compaction.
func wrapCommand(cmd string) string {
	escaped := strings.ReplaceAll(cmd, `"`, `\"`)
	return `banish "` + escaped + `"`
}

// hookOutput builds the PreToolUse hook response JSON.
func hookOutput(decision, wrapped string) string {
	out := map[string]any{
		"hookSpecificOutput": map[string]any{
			"hookEventName":            "PreToolUse",
			"permissionDecision":       decision,
			"permissionDecisionReason": "banish output compaction",
			"updatedInput":             map[string]any{"command": wrapped},
		},
	}
	b, _ := json.Marshal(out)
	return string(b)
}
