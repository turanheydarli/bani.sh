package ast

import (
	"testing"

	"go.banish.sh/banish/internal/token"
)

func TestProgramString(t *testing.T) {
	prog := &Program{
		Statements: []Statement{
			&Pipeline{
				Token: token.Token{Type: token.Ident, Literal: "ls"},
				Commands: []*Command{
					{
						Token: token.Token{Type: token.Ident, Literal: "ls"},
						Verb:  &Identifier{Value: "ls"},
						Target: &PathLiteral{Value: "/var/log"},
						Modifiers: []*Modifier{
							{Key: "ext", Value: "log"},
						},
					},
					{
						Token: token.Token{Type: token.Ident, Literal: "gz"},
						Verb:  &Identifier{Value: "gz"},
					},
				},
				Ops: []token.TokenType{token.Pipe},
			},
		},
	}

	want := "ls /var/log ext:log | gz"
	got := prog.String()
	if got != want {
		t.Errorf("Program.String() = %q, want %q", got, want)
	}
}

func TestAssignmentString(t *testing.T) {
	a := &Assignment{
		Name: "issues",
		Value: &Pipeline{
			Token: token.Token{Type: token.At, Literal: "@"},
			Commands: []*Command{
				{
					Token: token.Token{Type: token.At, Literal: "@"},
					Verb:  &MCPCall{Server: "github", Verb: "issues"},
					Modifiers: []*Modifier{
						{Key: "repo", Value: "acme/api"},
					},
				},
			},
		},
	}

	want := "$issues = @github issues repo:acme/api"
	got := a.String()
	if got != want {
		t.Errorf("Assignment.String() = %q, want %q", got, want)
	}
}

func TestDirectiveString(t *testing.T) {
	d := &Directive{
		Name: "human",
	}
	if got := d.String(); got != "!human" {
		t.Errorf("Directive.String() = %q, want %q", got, "!human")
	}

	d2 := &Directive{
		Name: "extension",
		Args: []Expression{
			&Identifier{Value: "deploy"},
			&Identifier{Value: "v:1.0"},
		},
	}
	want := "!extension deploy v:1.0"
	if got := d2.String(); got != want {
		t.Errorf("Directive.String() = %q, want %q", got, want)
	}
}

func TestCommandWithRedirect(t *testing.T) {
	c := &Command{
		Token: token.Token{Type: token.Ident, Literal: "ls"},
		Verb:  &Identifier{Value: "ls"},
		Target: &PathLiteral{Value: "/tmp"},
		Redirect: &Redirect{
			Direction: token.ArrowR,
			Path:      "out.txt",
		},
	}

	want := "ls /tmp -> out.txt"
	got := c.String()
	if got != want {
		t.Errorf("Command.String() = %q, want %q", got, want)
	}
}

func TestVariableRefString(t *testing.T) {
	v := &VariableRef{Name: "x"}
	if got := v.String(); got != "$x" {
		t.Errorf("VariableRef.String() = %q, want %q", got, "$x")
	}
}

func TestMCPCallString(t *testing.T) {
	m := &MCPCall{Server: "k8s", Verb: "pods"}
	want := "@k8s pods"
	if got := m.String(); got != want {
		t.Errorf("MCPCall.String() = %q, want %q", got, want)
	}
}

func TestStringLiteralString(t *testing.T) {
	s := &StringLiteral{Value: "hello world"}
	want := "\"hello world\""
	if got := s.String(); got != want {
		t.Errorf("StringLiteral.String() = %q, want %q", got, want)
	}
}

func TestPipelineParallel(t *testing.T) {
	p := &Pipeline{
		Token: token.Token{Type: token.At, Literal: "@"},
		Commands: []*Command{
			{
				Token: token.Token{Type: token.At, Literal: "@"},
				Verb:  &MCPCall{Server: "db", Verb: "ping"},
			},
			{
				Token: token.Token{Type: token.At, Literal: "@"},
				Verb:  &MCPCall{Server: "redis", Verb: "ping"},
			},
		},
		Ops: []token.TokenType{token.Ampersand},
	}

	want := "@db ping & @redis ping"
	got := p.String()
	if got != want {
		t.Errorf("Pipeline.String() = %q, want %q", got, want)
	}
}

func TestEmptyProgram(t *testing.T) {
	p := &Program{}
	if got := p.String(); got != "" {
		t.Errorf("empty Program.String() = %q, want %q", got, "")
	}
	if got := p.TokenLiteral(); got != "" {
		t.Errorf("empty Program.TokenLiteral() = %q, want %q", got, "")
	}
}
