package interpreter

import (
	"context"
	"encoding/json"
	"testing"

	"go.bani.sh/banish/internal/ast"
	"go.bani.sh/banish/internal/lexer"
	"go.bani.sh/banish/internal/parser"
)

func setup(t *testing.T) *Interpreter {
	t.Helper()
	reg := NewVerbRegistry()

	// echo: returns target as result
	reg.RegisterBuiltin("echo", func(_ context.Context, cmd *ast.Command, _ *Result) (*Result, error) {
		if cmd.Target != nil {
			return NewResult(cmd.Target.String()), nil
		}
		return NewResult(""), nil
	})

	// count: returns length of input data if string, or "0"
	reg.RegisterBuiltin("count", func(_ context.Context, _ *ast.Command, input *Result) (*Result, error) {
		if input == nil {
			return NewResult(0), nil
		}
		if s, ok := input.Data.(string); ok {
			return NewResult(len(s)), nil
		}
		return NewResult(1), nil
	})

	// upper: test verb that returns "UPPER"
	reg.RegisterBuiltin("upper", func(_ context.Context, _ *ast.Command, _ *Result) (*Result, error) {
		return NewResult("UPPER"), nil
	})

	return New(WithRegistry(reg))
}

func eval(t *testing.T, interp *Interpreter, input string) *Result {
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

func TestEvalSimpleCommand(t *testing.T) {
	interp := setup(t)
	r := eval(t, interp, `echo "hello"`)

	if r.String() != `"hello"` {
		t.Errorf("result = %q, want %q", r.String(), `"hello"`)
	}
}

func TestEvalPipeline(t *testing.T) {
	interp := setup(t)
	r := eval(t, interp, `echo "hello" | count`)

	if r.Data != 7 { // len(`"hello"`) = 7 (includes quotes from StringLiteral.String())
		t.Errorf("result = %v, want 7", r.Data)
	}
}

func TestEvalAssignment(t *testing.T) {
	interp := setup(t)
	eval(t, interp, `$x = echo "world"`)

	val, ok := interp.env.Get("x")
	if !ok {
		t.Fatal("variable $x not set")
	}
	if val.String() != `"world"` {
		t.Errorf("$x = %q, want %q", val.String(), `"world"`)
	}
}

func TestEvalDirectiveHuman(t *testing.T) {
	interp := setup(t)

	if interp.Human() {
		t.Error("human mode should be off by default")
	}

	eval(t, interp, "!human")

	if !interp.Human() {
		t.Error("human mode should be on after !human")
	}
}

func TestEvalDirectiveQuiet(t *testing.T) {
	interp := setup(t)
	eval(t, interp, "!human")
	eval(t, interp, "!quiet")

	if interp.Human() {
		t.Error("human mode should be off after !quiet")
	}
}

func TestEvalConditionalAnd(t *testing.T) {
	interp := setup(t)
	r := eval(t, interp, `echo "first" && echo "second"`)

	if r.String() != `"second"` {
		t.Errorf("result = %q, want %q", r.String(), `"second"`)
	}
}

func TestEvalUnknownVerb(t *testing.T) {
	interp := setup(t)

	l := lexer.New("nonexistent")
	p := parser.New(l)
	prog := p.ParseProgram()
	_, err := interp.Eval(prog)

	if err == nil {
		t.Error("expected error for unknown verb")
	}
}

// A plain shell command whose verb is not a registered .bsh verb must reach the
// shell fallback verbatim, so positional arguments (here a relative path) are
// never dropped by the lossy verb+target grammar.
func TestEvalSourcePreservesShellArgs(t *testing.T) {
	reg := NewVerbRegistry()
	reg.RegisterBuiltin("echo", func(_ context.Context, _ *ast.Command, _ *Result) (*Result, error) {
		return NewResult(""), nil
	})
	var gotShell string
	reg.SetFallback(func(_ context.Context, cmd *ast.Command, _ *Result) (*Result, error) {
		if s, ok := cmd.Target.(*ast.StringLiteral); ok {
			gotShell = s.Value
		}
		return NewResult("ran"), nil
	})
	interp := New(WithRegistry(reg))

	const cmd = "go test ./internal/scaffold/"
	if _, err := interp.EvalSource(cmd); err != nil {
		t.Fatalf("EvalSource: %v", err)
	}
	if gotShell != cmd {
		t.Fatalf("shell fallback got %q, want verbatim %q (positional path dropped)", gotShell, cmd)
	}
}

// A registered .bsh verb must still run through the interpreter, not the shell.
func TestEvalSourceRegisteredVerbStaysBSH(t *testing.T) {
	reg := NewVerbRegistry()
	reg.RegisterBuiltin("gs", func(_ context.Context, _ *ast.Command, _ *Result) (*Result, error) {
		return NewResult("verb"), nil
	})
	shelled := false
	reg.SetFallback(func(_ context.Context, _ *ast.Command, _ *Result) (*Result, error) {
		shelled = true
		return NewResult("shell"), nil
	})
	interp := New(WithRegistry(reg))

	r, err := interp.EvalSource("gs")
	if err != nil {
		t.Fatalf("EvalSource: %v", err)
	}
	if shelled {
		t.Fatal("registered verb gs must run as .bsh, not shell")
	}
	if r.String() != "verb" {
		t.Errorf("result = %q, want verb", r.String())
	}
}

func TestResultJSON(t *testing.T) {
	r := NewResult("hello")
	b, err := r.JSON()
	if err != nil {
		t.Fatalf("JSON error: %v", err)
	}

	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if out["result"] != "hello" {
		t.Errorf("result = %v, want hello", out["result"])
	}
}

func TestEnvironmentScoping(t *testing.T) {
	parent := NewEnvironment()
	parent.Set("x", NewResult("parent"))

	child := NewEnclosed(parent)
	child.Set("y", NewResult("child"))

	// Child sees parent vars
	if v, ok := child.Get("x"); !ok || v.String() != "parent" {
		t.Errorf("child.Get(x) = %v, want parent", v)
	}

	// Child sees own vars
	if v, ok := child.Get("y"); !ok || v.String() != "child" {
		t.Errorf("child.Get(y) = %v, want child", v)
	}

	// Parent does not see child vars
	if _, ok := parent.Get("y"); ok {
		t.Error("parent should not see child var y")
	}
}

func TestVerbRegistryResolutionOrder(t *testing.T) {
	reg := NewVerbRegistry()

	builtin := func(_ context.Context, _ *ast.Command, _ *Result) (*Result, error) {
		return NewResult("builtin"), nil
	}
	ext := func(_ context.Context, _ *ast.Command, _ *Result) (*Result, error) {
		return NewResult("extension"), nil
	}

	// Register extension first, then builtin. Builtin should win.
	reg.RegisterExtension("test", ext)
	reg.RegisterBuiltin("test", builtin)

	h, err := reg.Resolve("test")
	if err != nil {
		t.Fatalf("resolve error: %v", err)
	}

	r, _ := h(context.Background(), nil, nil)
	if r.String() != "builtin" {
		t.Errorf("resolved to %s, want builtin (builtins should win)", r.String())
	}
}

func TestVerbRegistryFallback(t *testing.T) {
	reg := NewVerbRegistry()
	reg.SetFallback(func(_ context.Context, cmd *ast.Command, _ *Result) (*Result, error) {
		return NewResult("fallback"), nil
	})

	h, err := reg.Resolve("anything")
	if err != nil {
		t.Fatalf("resolve error: %v", err)
	}

	r, _ := h(context.Background(), nil, nil)
	if r.String() != "fallback" {
		t.Errorf("resolved to %s, want fallback", r.String())
	}
}

func TestMultiStatementProgram(t *testing.T) {
	interp := setup(t)
	r := eval(t, interp, "$x = echo \"hello\"\n$y = echo \"world\"\nupper")

	if r.String() != "UPPER" {
		t.Errorf("last result = %q, want UPPER", r.String())
	}

	if v, ok := interp.env.Get("x"); !ok || v.String() != `"hello"` {
		t.Errorf("$x = %v, want \"hello\"", v)
	}
	if v, ok := interp.env.Get("y"); !ok || v.String() != `"world"` {
		t.Errorf("$y = %v, want \"world\"", v)
	}
}
