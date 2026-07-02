package compact

import (
	"bytes"
	"os/exec"
	"strings"

	"go.bani.sh/banish/internal/shell"
)

// ScriptFilterDef defines a filter that pipes output through a shell command.
type ScriptFilterDef struct {
	Name    string // filter name (for logging/debugging)
	Match   string // command substring to match
	Compact string // shell command that receives raw output on stdin
}

// ScriptFilter wraps a ScriptFilterDef into a Filter function.
func ScriptFilter(def ScriptFilterDef) Filter {
	return func(stdout, stderr string, exitCode int) string {
		raw := stdout
		if stderr != "" && exitCode != 0 {
			raw = stdout + "\n" + stderr
		}

		out, err := runScript(def.Compact, raw)
		if err != nil {
			// Script failed -- return raw output rather than losing data
			return strings.TrimRight(raw, "\n")
		}

		return strings.TrimRight(out, "\n")
	}
}

// runScript executes a shell command with input piped to stdin.
func runScript(script, input string) (string, error) {
	name, args := shell.Args(script)
	cmd := exec.Command(name, args...)
	cmd.Stdin = strings.NewReader(input)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// If the script exits non-zero but produced output, use the output.
		// This handles tools like grep that exit 1 on no match.
		if stdout.Len() > 0 {
			return stdout.String(), nil
		}
		return "", err
	}

	return stdout.String(), nil
}

// RegisterScriptFilters adds script-based filters to the registry.
// Called with filters from extensions (~/.banish/ext/) and BANISH manifest.
// Script filters have highest priority -- checked before TOML and Go filters.
func (r *Registry) RegisterScriptFilters(filters []ScriptFilterDef) {
	for _, f := range filters {
		if f.Match == "" || f.Compact == "" {
			continue
		}
		r.scriptFilters = append(r.scriptFilters, f)
	}
}
