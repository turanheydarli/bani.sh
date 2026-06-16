package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

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

// decideHook returns the hook output for a command in the host's native JSON
// envelope, or "" to emit nothing (Claude Code defers; Cursor callers turn this
// into "{}", which Cursor requires on every path).
func decideHook(cmd string, host permissions.Host) string {
	decision, wrapped := classifyHook(cmd, host)
	if host == permissions.HostCursor {
		return cursorOutput(decision, wrapped)
	}
	return claudeOutput(decision, wrapped)
}

// classifyHook decides what to do with a command: it returns one of allow / ask
// / deny / defer / skip, plus the wrapped command when a rewrite applies.
func classifyHook(cmd string, host permissions.Host) (decision, wrapped string) {
	trimmed := strings.TrimSpace(cmd)
	if trimmed == "" || shouldSkipHook(trimmed) {
		return "skip", ""
	}
	verdict := permissions.CheckFor(trimmed, host)
	if verdict == permissions.Deny {
		return "deny", ""
	}
	if permissions.ContainsUnattestable(trimmed) {
		return "defer", ""
	}
	if verdict == permissions.Allow {
		appendAuditAllow(host, cmd)
		return "allow", wrapCommand(cmd)
	}
	return "ask", wrapCommand(cmd)
}

// claudeOutput emits the Claude Code PreToolUse envelope, or "" to defer.
func claudeOutput(decision, wrapped string) string {
	switch decision {
	case "allow", "ask":
		return hookOutput(decision, wrapped)
	default: // skip, deny, defer
		return ""
	}
}

// cursorOutput emits Cursor's preToolUse envelope. Cursor requires JSON on every
// path, so anything other than an auto-allow returns "{}" (no rewrite). banish
// only rewrites and auto-allows commands Cursor's own rules already allow.
func cursorOutput(decision, wrapped string) string {
	if decision != "allow" {
		return "{}"
	}
	out := map[string]any{
		"permission":    "allow",
		"updated_input": map[string]any{"command": wrapped},
	}
	b, _ := json.Marshal(out)
	return string(b)
}

// appendAuditAllow records an auto-approved command so you can review exactly
// what banish let run without prompting you. Best-effort: it never fails the
// hook, and only auto-allows are recorded (ask/deferred commands already involve
// you or the host).
func appendAuditAllow(host permissions.Host, cmd string) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return
	}
	dir := filepath.Join(home, ".banish")
	if os.MkdirAll(dir, 0755) != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(dir, "hook-audit.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	entry := struct {
		TS      string `json:"ts"`
		Host    string `json:"host"`
		Command string `json:"command"`
	}{
		TS:      time.Now().UTC().Format(time.RFC3339),
		Host:    hostName(host),
		Command: cmd,
	}
	if b, err := json.Marshal(entry); err == nil {
		f.Write(append(b, '\n'))
	}
}

// hostName is the stable string label for a host, used in the audit log.
func hostName(h permissions.Host) string {
	if h == permissions.HostCursor {
		return "cursor"
	}
	return "claude-code"
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
