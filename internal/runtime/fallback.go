package runtime

import (
	"context"
	"strings"

	"go.banish.sh/banish/internal/ast"
	"go.banish.sh/banish/internal/compact"
	"go.banish.sh/banish/internal/interpreter"
)

// FallbackHandler returns a VerbHandler that executes unknown verbs as OS
// commands via the Executor. Before execution, rewrite rules may swap the
// command for a machine-readable variant; after execution, output runs
// through the compaction cascade (native renderers, then .bsh filters).
func FallbackHandler(exec *Executor, scriptFilters []compact.ScriptFilterDef, rewrites []compact.RewriteRule) interpreter.VerbHandler {
	filters := compact.NewRegistry()
	filters.RegisterScriptFilters(scriptFilters)
	rewriter := compact.NewRewriter(rewrites)

	return func(ctx context.Context, cmd *ast.Command, input *interpreter.Result) (*interpreter.Result, error) {
		name := ""
		if id, ok := cmd.Verb.(*ast.Identifier); ok {
			name = id.Value
		}

		// __shell__ is used by dual input mode to run raw bash strings.
		// This is the agent path: rewrite applies here, and only here --
		// verb expansions already chose their exact command.
		if name == "__shell__" {
			if s, ok := cmd.Target.(*ast.StringLiteral); ok {
				executed, rule := rewriter.Rewrite(s.Value)

				r, err := exec.RunShell(ctx, executed)
				if err != nil {
					return nil, err
				}

				result := applyCompaction(filters, executed, string(r.Stdout), string(r.Stderr), r.ExitCode)
				result.Rewritten = rule

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

		cmdline := name
		if len(args) > 0 {
			cmdline += " " + strings.Join(args, " ")
		}

		// If input is provided, pipe it through stdin via shell.
		if input != nil && name != "" {
			r, err := exec.RunShell(ctx, cmdline)
			if err != nil {
				return nil, err
			}
			return applyCompaction(filters, cmdline, string(r.Stdout), string(r.Stderr), r.ExitCode), nil
		}

		r, err := exec.Run(ctx, name, args)
		if err != nil {
			return nil, err
		}

		return applyCompaction(filters, cmdline, string(r.Stdout), string(r.Stderr), r.ExitCode), nil
	}
}

// applyCompaction runs the output through the compaction cascade if anything
// matches, otherwise falls back to raw output. Tracks raw vs compacted sizes.
func applyCompaction(filters *compact.Registry, cmdline, stdout, stderr string, exitCode int) *interpreter.Result {
	text, handler := filters.Compact(cmdline, stdout, stderr, exitCode)
	if handler == "" {
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
