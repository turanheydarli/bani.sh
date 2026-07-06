// Package ast defines the node types that represent the abstract syntax tree
// of a parsed .bsh program.
package ast

import (
	"strings"

	"go.banish.sh/banish/internal/token"
)

// Node is the interface implemented by all AST nodes.
type Node interface {
	TokenLiteral() string
	String() string
}

// Statement is a marker interface for statement nodes.
type Statement interface {
	Node
	statementNode()
}

// Expression is a marker interface for expression nodes.
type Expression interface {
	Node
	expressionNode()
}

// =========================================================================
// Program

// Program is the root node of a .bsh program.
type Program struct {
	Statements []Statement
}

// TokenLiteral returns the literal of the first statement.
func (p *Program) TokenLiteral() string {
	if len(p.Statements) > 0 {
		return p.Statements[0].TokenLiteral()
	}
	return ""
}

// String reconstructs a readable representation of the full program.
func (p *Program) String() string {
	var b strings.Builder
	for i, s := range p.Statements {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(s.String())
	}
	return b.String()
}

// =========================================================================
// Statements

// Pipeline is a sequence of commands connected by operators.
type Pipeline struct {
	Token    token.Token
	Commands []*Command
	Ops      []token.TokenType // operators between commands: |, ;, &, ?, &&, ||
}

func (p *Pipeline) statementNode()       {}
func (p *Pipeline) TokenLiteral() string { return p.Token.Literal }

// String returns a readable representation of the pipeline.
func (p *Pipeline) String() string {
	var b strings.Builder
	for i, cmd := range p.Commands {
		if i > 0 {
			b.WriteByte(' ')
			b.WriteString(opString(p.Ops[i-1]))
			b.WriteByte(' ')
		}
		b.WriteString(cmd.String())
	}
	return b.String()
}

// Assignment represents $name = pipeline.
type Assignment struct {
	Token token.Token
	Name  string // variable name without $
	Value Statement
}

func (a *Assignment) statementNode()       {}
func (a *Assignment) TokenLiteral() string { return a.Token.Literal }

// String returns a readable representation of the assignment.
func (a *Assignment) String() string {
	return "$" + a.Name + " = " + a.Value.String()
}

// Directive represents !name args.
type Directive struct {
	Token token.Token
	Name  string
	Args  []Expression
}

func (d *Directive) statementNode()       {}
func (d *Directive) TokenLiteral() string { return d.Token.Literal }

// String returns a readable representation of the directive.
func (d *Directive) String() string {
	var b strings.Builder
	b.WriteByte('!')
	b.WriteString(d.Name)
	for _, arg := range d.Args {
		b.WriteByte(' ')
		b.WriteString(arg.String())
	}
	return b.String()
}

// =========================================================================
// Command

// Command represents a verb with optional target, modifiers, and redirect.
type Command struct {
	Token     token.Token
	Verb      Expression  // Identifier or MCPCall
	Target    Expression  // optional positional arg
	Modifiers []*Modifier // key:value pairs
	Redirect  *Redirect   // optional -> or <-
}

func (c *Command) statementNode()       {}
func (c *Command) TokenLiteral() string { return c.Token.Literal }

// String returns a readable representation of the command.
func (c *Command) String() string {
	var b strings.Builder
	b.WriteString(c.Verb.String())
	if c.Target != nil {
		b.WriteByte(' ')
		b.WriteString(c.Target.String())
	}
	for _, m := range c.Modifiers {
		b.WriteByte(' ')
		b.WriteString(m.String())
	}
	if c.Redirect != nil {
		b.WriteByte(' ')
		b.WriteString(c.Redirect.String())
	}
	return b.String()
}

// Modifier represents a key:value pair.
type Modifier struct {
	Token token.Token
	Key   string
	Value string
}

func (m *Modifier) expressionNode()      {}
func (m *Modifier) TokenLiteral() string { return m.Token.Literal }

// String returns key:value.
func (m *Modifier) String() string {
	return m.Key + ":" + m.Value
}

// Redirect represents -> path or <- path.
type Redirect struct {
	Token     token.Token
	Direction token.TokenType // ArrowR or ArrowL
	Path      string
}

func (r *Redirect) expressionNode()      {}
func (r *Redirect) TokenLiteral() string { return r.Token.Literal }

// String returns the redirect as -> path or <- path.
func (r *Redirect) String() string {
	if r.Direction == token.ArrowR {
		return "-> " + r.Path
	}
	return "<- " + r.Path
}

// =========================================================================
// Expressions

// Identifier represents a plain identifier (verb names, targets).
type Identifier struct {
	Token token.Token
	Value string
}

func (i *Identifier) expressionNode()      {}
func (i *Identifier) TokenLiteral() string { return i.Token.Literal }
func (i *Identifier) String() string       { return i.Value }

// StringLiteral represents a quoted string value.
type StringLiteral struct {
	Token token.Token
	Value string
}

func (s *StringLiteral) expressionNode()      {}
func (s *StringLiteral) TokenLiteral() string { return s.Token.Literal }
func (s *StringLiteral) String() string       { return "\"" + s.Value + "\"" }

// NumberLiteral represents a numeric value.
type NumberLiteral struct {
	Token token.Token
	Value string
}

func (n *NumberLiteral) expressionNode()      {}
func (n *NumberLiteral) TokenLiteral() string { return n.Token.Literal }
func (n *NumberLiteral) String() string       { return n.Value }

// PathLiteral represents a filesystem path.
type PathLiteral struct {
	Token token.Token
	Value string
}

func (p *PathLiteral) expressionNode()      {}
func (p *PathLiteral) TokenLiteral() string { return p.Token.Literal }
func (p *PathLiteral) String() string       { return p.Value }

// GlobLiteral represents a glob pattern.
type GlobLiteral struct {
	Token token.Token
	Value string
}

func (g *GlobLiteral) expressionNode()      {}
func (g *GlobLiteral) TokenLiteral() string { return g.Token.Literal }
func (g *GlobLiteral) String() string       { return g.Value }

// RawValue returns the underlying value of an expression as written, without
// re-serialization. StringLiteral.String() re-adds surrounding quotes (and does
// not escape), which corrupts a value that is later spliced into a shell command
// or matched against the filesystem; every other literal node's String() already
// returns its raw Value. This is the one accessor target consumers should use.
func RawValue(e Expression) string {
	if e == nil {
		return ""
	}
	if s, ok := e.(*StringLiteral); ok {
		return s.Value
	}
	return e.String()
}

// VariableRef represents a $name reference.
type VariableRef struct {
	Token token.Token
	Name  string // without $
}

func (v *VariableRef) expressionNode()      {}
func (v *VariableRef) TokenLiteral() string { return v.Token.Literal }
func (v *VariableRef) String() string       { return "$" + v.Name }

// MCPCall represents @server verb syntax.
type MCPCall struct {
	Token  token.Token
	Server string // server name without @
	Verb   string
}

func (m *MCPCall) expressionNode()      {}
func (m *MCPCall) TokenLiteral() string { return m.Token.Literal }
func (m *MCPCall) String() string       { return "@" + m.Server + " " + m.Verb }

// =========================================================================
// Helpers

func opString(op token.TokenType) string {
	switch op {
	case token.Pipe:
		return "|"
	case token.Semicolon:
		return ";"
	case token.Ampersand:
		return "&"
	case token.Question:
		return "?"
	case token.And:
		return "&&"
	case token.Or:
		return "||"
	default:
		return "?"
	}
}
