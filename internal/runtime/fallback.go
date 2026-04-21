package runtime

import (
	"context"
	"path/filepath"
	"strings"

	"go.bani.sh/banish/internal/ast"
	"go.bani.sh/banish/internal/compact"
	"go.bani.sh/banish/internal/hints"
	"go.bani.sh/banish/internal/interpreter"
)

// FallbackHandler returns a VerbHandler that executes unknown verbs as OS
// commands via the Executor. Applies output compaction when a filter exists,
// and attaches _hint when a banish equivalent exists.
// scriptFilters are .bsh-defined filters loaded from extensions and BANISH manifest.
func FallbackHandler(exec *Executor, scriptFilters []compact.ScriptFilterDef) interpreter.VerbHandler {
	hinter := hints.New()
	filters := compact.NewRegistry()
	filters.RegisterScriptFilters(scriptFilters)

	return func(ctx context.Context, cmd *ast.Command, input *interpreter.Result) (*interpreter.Result, error) {
		name := ""
		if id, ok := cmd.Verb.(*ast.Identifier); ok {
			name = id.Value
		}

		// __shell__ is used by dual input mode to run raw bash strings.
		if name == "__shell__" {
			if s, ok := cmd.Target.(*ast.StringLiteral); ok {
				r, err := exec.RunShell(ctx, s.Value)
				if err != nil {
					return nil, err
				}

				stdout := string(r.Stdout)
				stderr := string(r.Stderr)

				// Try to compact the output using a registered filter.
				parts := strings.Fields(s.Value)
				baseName := ""
				if len(parts) > 0 {
					baseName = filepath.Base(parts[0])
				}
				result := applyCompaction(filters, baseName, parts[1:], stdout, stderr, r.ExitCode)

				if len(parts) > 0 {
					result.Hint = hinter.Suggest(parts[0], parts[1:])
				}
				if r.ExitCode != 0 {
					if result.Meta == nil {
						result.Meta = make(map[string]any)
					}
					result.Meta["exit"] = r.ExitCode
				}
				return result, nil
			}
		}

		var args []string
		if cmd.Target != nil {
			args = append(args, cmd.Target.String())
		}
		for _, m := range cmd.Modifiers {
			args = append(args, m.Key+":"+m.Value)
		}

		// If input is provided, pipe it through stdin via shell.
		if input != nil && name != "" {
			full := name + " " + strings.Join(args, " ")
			r, err := exec.RunShell(ctx, full)
			if err != nil {
				return nil, err
			}
			result := applyCompaction(filters, name, args, string(r.Stdout), string(r.Stderr), r.ExitCode)
			result.Hint = hinter.Suggest(name, args)
			return result, nil
		}

		r, err := exec.Run(ctx, name, args)
		if err != nil {
			return nil, err
		}

		result := applyCompaction(filters, name, args, string(r.Stdout), string(r.Stderr), r.ExitCode)

		// Attach _hint if a shorter banish alternative exists.
		result.Hint = hinter.Suggest(name, args)

		return result, nil
	}
}

// applyCompaction runs the output through a compact filter if one exists,
// otherwise falls back to raw output. Tracks raw vs compacted sizes.
func applyCompaction(filters *compact.Registry, cmdName string, args []string, stdout, stderr string, exitCode int) *interpreter.Result {
	f := filters.Lookup(cmdName, args)

	var text string
	if f != nil {
		text = f(stdout, stderr, exitCode)
	} else {
		text = strings.TrimRight(stdout, "\n")
		if exitCode != 0 && stderr != "" {
			if text != "" {
				text += "\n"
			}
			text += "[stderr] " + strings.TrimRight(stderr, "\n")
		}
	}

	result := interpreter.NewResult(text)

	// Track raw vs compacted for internal token accounting.
	// These fields are NOT serialized to output -- only used by the analyzer.
	rawLen := len(stdout) + len(stderr)
	compactLen := len(text)
	result.RawTokens = int64(rawLen / 4)
	result.OutTokens = int64(compactLen / 4)
	if rawLen > 0 && compactLen < rawLen {
		result.SavedPct = (rawLen - compactLen) * 100 / rawLen
	}

	return result
}
