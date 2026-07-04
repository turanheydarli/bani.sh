package compact

import (
	"bytes"
	"os/exec"
	"strings"

	"go.banish.sh/banish/internal/shell"
)

// ScriptFilterDef defines a .bsh output filter: an optional shell pipe
// (!compact) plus declarative ops (!drop/!keep/!max-lines/...) that run
// in-process without spawning a subprocess.
type ScriptFilterDef struct {
	Name    string    // filter name (for logging/debugging)
	Match   string    // tokenized prefix pattern, e.g. "git status"
	Compact string    // optional shell command that receives raw output on stdin
	Ops     FilterOps // declarative transformations, applied after Compact
}

// ScriptFilter wraps a ScriptFilterDef into a Filter function.
func ScriptFilter(def ScriptFilterDef) Filter {
	return func(stdout, stderr string, exitCode int) string {
		raw := stdout
		if stderr != "" && exitCode != 0 {
			raw = stdout + "\n" + stderr
		}

		text := raw
		if def.Compact != "" {
			out, err := runScript(def.Compact, raw)
			if err != nil {
				// Script failed -- return raw output rather than losing data
				return strings.TrimRight(raw, "\n")
			}
			text = out
		}

		text = def.Ops.Apply(strings.TrimRight(text, "\n"))
		return strings.TrimRight(text, "\n")
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
// A filter needs a match pattern and at least one action (shell pipe or ops).
func (r *Registry) RegisterScriptFilters(filters []ScriptFilterDef) {
	for _, f := range filters {
		if f.Match == "" || (f.Compact == "" && f.Ops.IsZero()) {
			continue
		}
		r.scriptFilters = append(r.scriptFilters, f)
	}
	r.sorted = false
}
