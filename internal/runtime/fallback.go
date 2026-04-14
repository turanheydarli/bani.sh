package runtime

import (
	"context"
	"strings"

	"go.bani.sh/banish/internal/ast"
	"go.bani.sh/banish/internal/hints"
	"go.bani.sh/banish/internal/interpreter"
)

// FallbackHandler returns a VerbHandler that executes unknown verbs as OS
// commands via the Executor. Attaches _hint when a banish equivalent exists.
func FallbackHandler(exec *Executor) interpreter.VerbHandler {
	hinter := hints.New()

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
				result := interpreter.NewResult(strings.TrimRight(string(r.Stdout), "\n"))
				// Extract command name from the shell string for _hint lookup.
				parts := strings.Fields(s.Value)
				if len(parts) > 0 {
					result.Hint = hinter.Suggest(parts[0], parts[1:])
				}
				if len(r.Stderr) > 0 {
					if result.Meta == nil {
						result.Meta = make(map[string]any)
					}
					result.Meta["stderr"] = strings.TrimRight(string(r.Stderr), "\n")
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
			result := interpreter.NewResult(strings.TrimRight(string(r.Stdout), "\n"))
			result.Hint = hinter.Suggest(name, args)
			return result, nil
		}

		r, err := exec.Run(ctx, name, args)
		if err != nil {
			return nil, err
		}

		result := interpreter.NewResult(strings.TrimRight(string(r.Stdout), "\n"))
		if len(r.Stderr) > 0 {
			if result.Meta == nil {
				result.Meta = make(map[string]any)
			}
			result.Meta["stderr"] = strings.TrimRight(string(r.Stderr), "\n")
		}
		if r.ExitCode != 0 {
			if result.Meta == nil {
				result.Meta = make(map[string]any)
			}
			result.Meta["exit"] = r.ExitCode
		}

		// Attach _hint if a shorter banish alternative exists.
		result.Hint = hinter.Suggest(name, args)

		return result, nil
	}
}
