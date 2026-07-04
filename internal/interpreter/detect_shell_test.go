package interpreter

import (
	"context"
	"testing"

	"go.banish.sh/banish/internal/ast"
	"go.banish.sh/banish/internal/lexer"
	"go.banish.sh/banish/internal/parser"
)

func TestAllVerbsRegistered(t *testing.T) {
	interp := New()
	noop := func(ctx context.Context, cmd *ast.Command, in *Result) (*Result, error) {
		return NewResult(""), nil
	}
	interp.registry.RegisterBuiltin("count", noop)
	interp.registry.RegisterBuiltin("ls", noop)

	cases := []struct {
		source string
		want   bool
	}{
		{"ls /tmp | count", true},              // all registered: genuine .bsh
		{"go test ./scaffold/", false},         // unregistered verb: shell
		{"go test ./scaffold/ | count", false}, // mixed pipeline: shell wins
		{"ls /tmp", true},
	}
	for _, c := range cases {
		l := lexer.New(c.source)
		p := parser.New(l)
		prog := p.ParseProgram()
		if len(p.Errors()) > 0 {
			t.Fatalf("parse %q: %v", c.source, p.Errors()[0])
		}
		if got := interp.allVerbsRegistered(prog); got != c.want {
			t.Errorf("allVerbsRegistered(%q) = %v, want %v", c.source, got, c.want)
		}
	}
}
