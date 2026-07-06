package extension

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"go.banish.sh/banish/internal/ast"
	"go.banish.sh/banish/internal/interpreter"
)

func TestLoadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.bsh")

	content := `!extension test v:1.0
!verb greet
!verb hello
`
	os.WriteFile(path, []byte(content), 0644)

	loader := NewLoader()
	if err := loader.LoadFile(path); err != nil {
		t.Fatalf("LoadFile error: %v", err)
	}

	exts := loader.Extensions()
	if len(exts) != 1 {
		t.Fatalf("expected 1 extension, got %d", len(exts))
	}
	if exts[0].Name != "test" {
		t.Errorf("name = %q, want test", exts[0].Name)
	}
	if exts[0].Version != "v:1.0" {
		t.Errorf("version = %q, want v:1.0", exts[0].Version)
	}
	if len(exts[0].Verbs) != 2 {
		t.Errorf("verbs count = %d, want 2", len(exts[0].Verbs))
	}
}

func TestLoadDir(t *testing.T) {
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "a.bsh"), []byte("!extension a v:1.0\n!verb alpha\n"), 0644)
	os.WriteFile(filepath.Join(dir, "b.bsh"), []byte("!extension b v:1.0\n!verb beta\n"), 0644)
	os.WriteFile(filepath.Join(dir, "skip.txt"), []byte("not an extension"), 0644)

	loader := NewLoader()
	if err := loader.LoadDir(dir); err != nil {
		t.Fatalf("LoadDir error: %v", err)
	}

	if len(loader.Extensions()) != 2 {
		t.Errorf("expected 2 extensions, got %d", len(loader.Extensions()))
	}
}

func TestLoadDirMissing(t *testing.T) {
	loader := NewLoader()
	err := loader.LoadDir("/nonexistent/path")
	if err != nil {
		t.Errorf("expected nil for missing dir, got %v", err)
	}
}

func TestRegister(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.bsh"), []byte("!extension test v:1.0\n!verb greet\n"), 0644)

	loader := NewLoader()
	loader.LoadDir(dir)

	reg := interpreter.NewVerbRegistry()
	loader.Register(reg)

	h, err := reg.Resolve("greet")
	if err != nil {
		t.Fatalf("resolve greet: %v", err)
	}

	r, err := h(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("greet error: %v", err)
	}
	if r == nil {
		t.Fatal("expected result from greet")
	}
}

// A verb whose expand splices $1 into a shell command must not let a crafted
// target value execute injected commands: the substituted value is shell-quoted,
// so shell metacharacters are literal. Regression for the MCP arg-injection
// finding.
func TestExtensionArgInjectionQuoted(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "PWNED")

	h := MakeVerbHandler("greet", "exec echo hi $1")
	payload := "x; touch " + marker + "; echo done"
	cmd := &ast.Command{Target: &ast.StringLiteral{Value: payload}}

	r, err := h(context.Background(), cmd, nil)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if _, statErr := os.Stat(marker); statErr == nil {
		t.Fatal("injection succeeded: marker file was created")
	}
	// The payload must survive verbatim as a single argument to echo.
	if got := r.String(); got != "hi "+payload {
		t.Errorf("output = %q, want %q", got, "hi "+payload)
	}
}

// The same guarantee for a modifier value ($key substitution).
func TestExtensionModifierInjectionQuoted(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "PWNED")

	h := MakeVerbHandler("greet", "exec echo hi $who")
	payload := "a; touch " + marker
	cmd := &ast.Command{Modifiers: []*ast.Modifier{{Key: "who", Value: payload}}}

	if _, err := h(context.Background(), cmd, nil); err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if _, statErr := os.Stat(marker); statErr == nil {
		t.Fatal("modifier injection succeeded: marker file was created")
	}
}

func TestExtensionShadowsBuiltin(t *testing.T) {
	reg := interpreter.NewVerbRegistry()

	// Register builtin first
	reg.RegisterBuiltin("test", func(_ context.Context, _ *ast.Command, _ *interpreter.Result) (*interpreter.Result, error) {
		return interpreter.NewResult("builtin"), nil
	})

	// Extension tries to shadow
	reg.RegisterExtension("test", func(_ context.Context, _ *ast.Command, _ *interpreter.Result) (*interpreter.Result, error) {
		return interpreter.NewResult("extension"), nil
	})

	// Builtin should still win
	h, _ := reg.Resolve("test")
	r, _ := h(context.Background(), nil, nil)
	if r.String() != "builtin" {
		t.Errorf("expected builtin to win, got %s", r.String())
	}
}
