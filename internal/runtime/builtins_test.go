package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"go.bani.sh/banish/internal/ast"
	"go.bani.sh/banish/internal/interpreter"
	"go.bani.sh/banish/internal/token"
)

func cmd(verb string, target string, mods ...string) *ast.Command {
	c := &ast.Command{
		Token: token.Token{Type: token.Ident, Literal: verb},
		Verb:  &ast.Identifier{Value: verb},
	}
	if target != "" {
		c.Target = &ast.Identifier{Value: target}
	}
	for i := 0; i+1 < len(mods); i += 2 {
		c.Modifiers = append(c.Modifiers, &ast.Modifier{Key: mods[i], Value: mods[i+1]})
	}
	return c
}

func TestEcho(t *testing.T) {
	reg := interpreter.NewVerbRegistry()
	RegisterBuiltins(reg)

	h, _ := reg.Resolve("echo")
	r, err := h(context.Background(), cmd("echo", "hello"), nil)
	if err != nil {
		t.Fatalf("echo error: %v", err)
	}
	if r.String() != "hello" {
		t.Errorf("echo = %q, want hello", r.String())
	}
}

func TestLs(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(dir, "b.log"), []byte("b"), 0644)
	os.WriteFile(filepath.Join(dir, "c.txt"), []byte("c"), 0644)

	reg := interpreter.NewVerbRegistry()
	RegisterBuiltins(reg)

	h, _ := reg.Resolve("ls")
	r, err := h(context.Background(), cmd("ls", dir), nil)
	if err != nil {
		t.Fatalf("ls error: %v", err)
	}

	entries, ok := r.Data.([]fileEntry)
	if !ok {
		t.Fatalf("ls result type = %T, want []fileEntry", r.Data)
	}
	if len(entries) != 3 {
		t.Errorf("ls count = %d, want 3", len(entries))
	}
}

func TestLsWithExtFilter(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(dir, "b.log"), []byte("b"), 0644)
	os.WriteFile(filepath.Join(dir, "c.txt"), []byte("c"), 0644)

	reg := interpreter.NewVerbRegistry()
	RegisterBuiltins(reg)

	h, _ := reg.Resolve("ls")
	r, err := h(context.Background(), cmd("ls", dir, "ext", "txt"), nil)
	if err != nil {
		t.Fatalf("ls error: %v", err)
	}

	entries := r.Data.([]fileEntry)
	if len(entries) != 2 {
		t.Errorf("ls ext:txt count = %d, want 2", len(entries))
	}
}

func TestReadWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	reg := interpreter.NewVerbRegistry()
	RegisterBuiltins(reg)

	// Write
	wh, _ := reg.Resolve("write")
	_, err := wh(context.Background(), cmd("write", path), interpreter.NewResult("hello banish"))
	if err != nil {
		t.Fatalf("write error: %v", err)
	}

	// Read
	rh, _ := reg.Resolve("read")
	r, err := rh(context.Background(), cmd("read", path), nil)
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	if r.String() != "hello banish" {
		t.Errorf("read = %q, want hello banish", r.String())
	}
}

func TestMkdirRm(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sub", "dir")

	reg := interpreter.NewVerbRegistry()
	RegisterBuiltins(reg)

	mkh, _ := reg.Resolve("mkdir")
	_, err := mkh(context.Background(), cmd("mkdir", dir), nil)
	if err != nil {
		t.Fatalf("mkdir error: %v", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("dir not created: %v", err)
	}

	rmh, _ := reg.Resolve("rm")
	_, err = rmh(context.Background(), cmd("rm", dir), nil)
	if err != nil {
		t.Fatalf("rm error: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("dir not removed")
	}
}

func TestCpMv(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")
	mv := filepath.Join(dir, "moved.txt")

	os.WriteFile(src, []byte("data"), 0644)

	reg := interpreter.NewVerbRegistry()
	RegisterBuiltins(reg)

	// cp
	cph, _ := reg.Resolve("cp")
	_, err := cph(context.Background(), cmd("cp", src, "to", dst), nil)
	if err != nil {
		t.Fatalf("cp error: %v", err)
	}
	data, _ := os.ReadFile(dst)
	if string(data) != "data" {
		t.Errorf("cp result = %q, want data", data)
	}

	// mv
	mvh, _ := reg.Resolve("mv")
	_, err = mvh(context.Background(), cmd("mv", dst, "to", mv), nil)
	if err != nil {
		t.Fatalf("mv error: %v", err)
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Error("mv: src still exists")
	}
	data, _ = os.ReadFile(mv)
	if string(data) != "data" {
		t.Errorf("mv result = %q, want data", data)
	}
}

func TestHeadTail(t *testing.T) {
	reg := interpreter.NewVerbRegistry()
	RegisterBuiltins(reg)

	input := interpreter.NewResult("line1\nline2\nline3\nline4\nline5")

	hh, _ := reg.Resolve("head")
	r, err := hh(context.Background(), cmd("head", "", "n", "2"), input)
	if err != nil {
		t.Fatalf("head error: %v", err)
	}
	if r.String() != "line1\nline2" {
		t.Errorf("head n:2 = %q, want line1\\nline2", r.String())
	}

	th, _ := reg.Resolve("tail")
	r, err = th(context.Background(), cmd("tail", "", "n", "2"), input)
	if err != nil {
		t.Fatalf("tail error: %v", err)
	}
	if r.String() != "line4\nline5" {
		t.Errorf("tail n:2 = %q, want line4\\nline5", r.String())
	}
}

func TestEnv(t *testing.T) {
	os.Setenv("BANISH_TEST_VAR", "testval")
	defer os.Unsetenv("BANISH_TEST_VAR")

	reg := interpreter.NewVerbRegistry()
	RegisterBuiltins(reg)

	h, _ := reg.Resolve("env")
	r, err := h(context.Background(), cmd("env", "BANISH_TEST_VAR"), nil)
	if err != nil {
		t.Fatalf("env error: %v", err)
	}
	if r.String() != "testval" {
		t.Errorf("env = %q, want testval", r.String())
	}
}

func TestCount(t *testing.T) {
	reg := interpreter.NewVerbRegistry()
	RegisterBuiltins(reg)

	h, _ := reg.Resolve("count")

	r, _ := h(context.Background(), cmd("count", ""), interpreter.NewResult("a\nb\nc"))
	if r.Data != 3 {
		t.Errorf("count = %v, want 3", r.Data)
	}

	r, _ = h(context.Background(), cmd("count", ""), nil)
	if r.Data != 0 {
		t.Errorf("count nil = %v, want 0", r.Data)
	}
}

func TestCatAlias(t *testing.T) {
	reg := interpreter.NewVerbRegistry()
	RegisterBuiltins(reg)

	// cat should resolve to the same handler as read
	_, err := reg.Resolve("cat")
	if err != nil {
		t.Errorf("cat not registered: %v", err)
	}
}
