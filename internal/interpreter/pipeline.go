package interpreter

import (
	"fmt"
	"strings"
	"sync"

	"go.banish.sh/banish/internal/ast"
	"go.banish.sh/banish/internal/token"
)

// evalPipeline executes a pipeline of commands connected by operators.
func (interp *Interpreter) evalPipeline(pipe *ast.Pipeline) (*Result, error) {
	if len(pipe.Commands) == 0 {
		return nil, fmt.Errorf("empty pipeline")
	}

	// Single command pipeline (shouldn't happen, parser unwraps these, but be safe)
	if len(pipe.Commands) == 1 {
		return interp.evalCommand(pipe.Commands[0], nil)
	}

	var prev *Result
	var prevErr error
	i := 0

	for i < len(pipe.Commands) {
		cmd := pipe.Commands[i]

		if i == 0 {
			prev, prevErr = interp.evalCommand(cmd, nil)
			i++
			continue
		}

		op := pipe.Ops[i-1]

		switch op {
		case token.Pipe:
			// Pass prev result as input to next command.
			prev, prevErr = interp.evalCommand(cmd, prev)

		case token.Semicolon:
			// Run regardless of previous result.
			prev, prevErr = interp.evalCommand(cmd, nil)

		case token.Ampersand:
			// Collect all consecutive & commands into a parallel group.
			group := []*ast.Command{pipe.Commands[i-1], cmd}
			for i < len(pipe.Ops) && pipe.Ops[i] == token.Ampersand {
				i++
				group = append(group, pipe.Commands[i])
			}
			prev, prevErr = interp.evalParallelGroup(group)

		case token.Question:
			// Filter: apply right command's modifiers as predicate on left's output.
			prev, prevErr = interp.evalFilter(prev, cmd)

		case token.And:
			// Only run if previous succeeded.
			if prevErr != nil {
				return prev, prevErr
			}
			prev, prevErr = interp.evalCommand(cmd, prev)

		case token.Or:
			// Only run if previous failed.
			if prevErr == nil {
				// Previous succeeded, skip this command.
				i++
				continue
			}
			// Previous failed, try this one.
			prev, prevErr = interp.evalCommand(cmd, nil)

		default:
			return nil, fmt.Errorf("unknown pipeline operator: %v", op)
		}

		i++
	}

	if prevErr != nil {
		return nil, prevErr
	}
	return prev, nil
}

// evalParallelGroup runs a set of commands concurrently and returns combined results.
func (interp *Interpreter) evalParallelGroup(cmds []*ast.Command) (*Result, error) {
	type entry struct {
		r   *Result
		err error
	}

	results := make([]entry, len(cmds))
	var wg sync.WaitGroup
	wg.Add(len(cmds))

	for idx, c := range cmds {
		go func(i int, cmd *ast.Command) {
			defer wg.Done()
			r, err := interp.evalCommand(cmd, nil)
			results[i] = entry{r: r, err: err}
		}(idx, c)
	}
	wg.Wait()

	var combined []any
	for _, re := range results {
		if re.err != nil {
			return nil, re.err
		}
		if re.r != nil {
			combined = append(combined, re.r.Data)
		}
	}

	return NewResult(combined), nil
}

// evalFilter applies a predicate (right command's modifiers) against items in prev result.
func (interp *Interpreter) evalFilter(prev *Result, predicate *ast.Command) (*Result, error) {
	if prev == nil {
		return NewResult(nil), nil
	}

	// Build predicate from modifiers: key:value pairs to match.
	predicates := make(map[string]string)
	for _, m := range predicate.Modifiers {
		predicates[m.Key] = m.Value
	}

	// Filter depends on the type of data.
	switch data := prev.Data.(type) {
	case []any:
		var filtered []any
		for _, item := range data {
			if matchesPredicate(item, predicates) {
				filtered = append(filtered, item)
			}
		}
		return NewResult(filtered), nil

	case string:
		// Filter lines matching predicate values.
		lines := strings.Split(data, "\n")
		var filtered []string
		for _, line := range lines {
			match := true
			for _, v := range predicates {
				if !strings.Contains(line, v) {
					match = false
					break
				}
			}
			if match {
				filtered = append(filtered, line)
			}
		}
		return NewResult(strings.Join(filtered, "\n")), nil

	default:
		// Single item: check if it matches.
		if matchesPredicate(data, predicates) {
			return prev, nil
		}
		return NewResult(nil), nil
	}
}

// matchesPredicate checks if an item matches all key:value predicates.
func matchesPredicate(item any, predicates map[string]string) bool {
	m, ok := item.(map[string]any)
	if !ok {
		return false
	}

	for k, v := range predicates {
		val, exists := m[k]
		if !exists {
			return false
		}
		if fmt.Sprintf("%v", val) != v {
			return false
		}
	}
	return true
}
