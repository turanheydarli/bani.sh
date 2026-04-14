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

func TestResultJSONWithHint(t *testing.T) {
	r := NewResult("data")
	r.Hint = &Hint{Shorter: "ls /var", Saved: 12, Why: "structured"}

	b, err := r.JSON()
	if err != nil {
		t.Fatalf("JSON error: %v", err)
	}

	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	hint, ok := out["_hint"].(map[string]any)
	if !ok {
		t.Fatalf("_hint missing or wrong type")
	}
	if hint["shorter"] != "ls /var" {
		t.Errorf("_hint.shorter = %v, want ls /var", hint["shorter"])
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
