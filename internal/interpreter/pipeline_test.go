package interpreter

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.bani.sh/banish/internal/ast"
	"go.bani.sh/banish/internal/lexer"
	"go.bani.sh/banish/internal/parser"
)

func setupPipeline(t *testing.T) *Interpreter {
	t.Helper()
	reg := NewVerbRegistry()

	reg.RegisterBuiltin("echo", func(_ context.Context, cmd *ast.Command, _ *Result) (*Result, error) {
		if cmd.Target != nil {
			return NewResult(cmd.Target.String()), nil
		}
		return NewResult(""), nil
	})

	reg.RegisterBuiltin("count", func(_ context.Context, _ *ast.Command, input *Result) (*Result, error) {
		if input == nil {
			return NewResult(0), nil
		}
		if s, ok := input.Data.(string); ok {
			return NewResult(len(s)), nil
		}
		return NewResult(1), nil
	})

	reg.RegisterBuiltin("upper", func(_ context.Context, _ *ast.Command, _ *Result) (*Result, error) {
		return NewResult("UPPER"), nil
	})

	reg.RegisterBuiltin("fail", func(_ context.Context, _ *ast.Command, _ *Result) (*Result, error) {
		return nil, errors.New("command failed")
	})

	reg.RegisterBuiltin("ok", func(_ context.Context, _ *ast.Command, _ *Result) (*Result, error) {
		return NewResult("ok"), nil
	})

	reg.RegisterBuiltin("slowop", func(ctx context.Context, _ *ast.Command, _ *Result) (*Result, error) {
		select {
		case <-time.After(100 * time.Millisecond):
			return NewResult("done"), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})

	return New(WithRegistry(reg))
}

func evalPipe(t *testing.T, interp *Interpreter, input string) *Result {
	t.Helper()
	l := lexer.New(input)
	p := parser.New(l)
	prog := p.ParseProgram()
	for _, err := range p.Errors() {
		t.Fatalf("parse error: %s", err)
	}
	r, err := interp.Eval(prog)
	if err != nil {
		t.Fatalf("eval error: %v", err)
	}
	return r
}

func TestPipeOperator(t *testing.T) {
	interp := setupPipeline(t)
	r := evalPipe(t, interp, `echo "hello" | count`)

	// "hello" is 7 chars (includes quotes from StringLiteral.String())
	if r.Data != 7 {
		t.Errorf("pipe result = %v, want 7", r.Data)
	}
}

func TestSequentialOperator(t *testing.T) {
	interp := setupPipeline(t)
	r := evalPipe(t, interp, `echo "first" ; echo "second"`)

	if r.String() != `"second"` {
		t.Errorf("sequential result = %q, want %q", r.String(), `"second"`)
	}
}

func TestParallelOperator(t *testing.T) {
	interp := setupPipeline(t)

	start := time.Now()
	r := evalPipe(t, interp, "slowop & slowop & slowop")
	elapsed := time.Since(start)

	// Three 100ms ops in parallel should complete in ~100ms, not ~300ms.
	if elapsed > 250*time.Millisecond {
		t.Errorf("parallel took %v, want < 250ms (ops are 100ms each)", elapsed)
	}

	arr, ok := r.Data.([]any)
	if !ok {
		t.Fatalf("parallel result type = %T, want []any", r.Data)
	}
	if len(arr) != 3 {
		t.Errorf("parallel result count = %d, want 3", len(arr))
	}
}

func TestAndOperatorSuccess(t *testing.T) {
	interp := setupPipeline(t)
	r := evalPipe(t, interp, `ok && echo "yes"`)

	if r.String() != `"yes"` {
		t.Errorf("and success = %q, want %q", r.String(), `"yes"`)
	}
}

func TestAndOperatorFailure(t *testing.T) {
	interp := setupPipeline(t)

	l := lexer.New("fail && echo should-not-run")
	p := parser.New(l)
	prog := p.ParseProgram()
	_, err := interp.Eval(prog)

	if err == nil {
		t.Error("expected error from fail &&")
	}
}

func TestOrOperatorSuccess(t *testing.T) {
	interp := setupPipeline(t)
	// ok succeeds, so || branch should be skipped.
	r := evalPipe(t, interp, `ok || echo "fallback"`)

	if r.String() != "ok" {
		t.Errorf("or (success) = %q, want ok", r.String())
	}
}

func TestOrOperatorFailure(t *testing.T) {
	interp := setupPipeline(t)
	r := evalPipe(t, interp, `fail || echo "fallback"`)

	if r.String() != `"fallback"` {
		t.Errorf("or (failure) = %q, want %q", r.String(), `"fallback"`)
	}
}

func TestFilterOperator(t *testing.T) {
	interp := setupPipeline(t)

	// Set up data to filter.
	reg := interp.Registry()
	reg.RegisterBuiltin("data", func(_ context.Context, _ *ast.Command, _ *Result) (*Result, error) {
		return NewResult([]any{
			map[string]any{"name": "a", "priority": "high"},
			map[string]any{"name": "b", "priority": "low"},
			map[string]any{"name": "c", "priority": "high"},
		}), nil
	})

	r := evalPipe(t, interp, "data ? priority:high")

	arr, ok := r.Data.([]any)
	if !ok {
		t.Fatalf("filter result type = %T, want []any", r.Data)
	}
	if len(arr) != 2 {
		t.Errorf("filter count = %d, want 2 (only high priority)", len(arr))
	}
}

func TestFilterStringLines(t *testing.T) {
	interp := setupPipeline(t)

	reg := interp.Registry()
	reg.RegisterBuiltin("lines", func(_ context.Context, _ *ast.Command, _ *Result) (*Result, error) {
		return NewResult("error: disk full\ninfo: ok\nerror: timeout"), nil
	})

	r := evalPipe(t, interp, "lines ? type:error")

	s, ok := r.Data.(string)
	if !ok {
		t.Fatalf("filter string result type = %T", r.Data)
	}
	// String filter matches lines containing the predicate values.
	// "error" appears in 2 lines.
	lines := 0
	for _, line := range splitLines(s) {
		if line != "" {
			lines++
		}
	}
	if lines != 2 {
		t.Errorf("filtered lines = %d, want 2", lines)
	}
}

func TestMixedPipeAndConditional(t *testing.T) {
	interp := setupPipeline(t)
	r := evalPipe(t, interp, `echo "test" | count && echo "done"`)

	if r.String() != `"done"` {
		t.Errorf("mixed result = %q, want %q", r.String(), `"done"`)
	}
}

func splitLines(s string) []string {
	return split(s, "\n")
}

func split(s, sep string) []string {
	if s == "" {
		return nil
	}
	result := []string{}
	for {
		i := indexOf(s, sep)
		if i < 0 {
			result = append(result, s)
			break
		}
		result = append(result, s[:i])
		s = s[i+len(sep):]
	}
	return result
}

func indexOf(s, sub string) int {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
