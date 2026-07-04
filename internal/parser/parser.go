// Package parser implements a recursive descent parser that produces an AST
// from a token stream.
package parser

import (
	"fmt"

	"go.banish.sh/banish/internal/ast"
	"go.banish.sh/banish/internal/lexer"
	"go.banish.sh/banish/internal/token"
)

// Parser consumes tokens from a lexer and produces an AST.
type Parser struct {
	l      *lexer.Lexer
	cur    token.Token
	peek   token.Token
	errors []string
}

// New creates a parser for the given lexer.
func New(l *lexer.Lexer) *Parser {
	p := &Parser{l: l}
	p.next()
	p.next()
	return p
}

// Errors returns the list of parse errors.
func (p *Parser) Errors() []string {
	return p.errors
}

// ParseProgram parses the full token stream into a Program AST node.
func (p *Parser) ParseProgram() *ast.Program {
	prog := &ast.Program{}

	for p.cur.Type != token.EOF {
		p.skipNewlines()
		if p.cur.Type == token.EOF {
			break
		}
		stmt := p.parseStatement()
		if stmt != nil {
			prog.Statements = append(prog.Statements, stmt)
		} else {
			// Avoid infinite loop: skip to next line on failed parse.
			p.recover()
		}
	}

	return prog
}

func (p *Parser) parseStatement() ast.Statement {
	switch {
	case p.cur.Type == token.Dollar:
		return p.parseDollar()
	case p.cur.Type == token.Bang:
		return p.parseDirective()
	default:
		return p.parsePipeline()
	}
}

// parseDollar handles $ which can be assignment ($x = ...) or a variable
// reference starting a pipeline ($x ? ...).
func (p *Parser) parseDollar() ast.Statement {
	tok := p.cur // $
	p.next()     // move to name

	if p.cur.Type != token.Ident {
		p.error("expected variable name after $")
		p.recover()
		return nil
	}

	name := p.cur.Literal
	nameTok := p.cur
	p.next() // move past name

	// If next is =, this is an assignment.
	if p.cur.Type == token.Equals {
		p.next() // skip =
		value := p.parsePipeline()
		if value == nil {
			return nil
		}
		return &ast.Assignment{Token: tok, Name: name, Value: value}
	}

	// Otherwise $name starts a pipeline as a variable reference verb.
	vref := &ast.VariableRef{Token: tok, Name: name}
	cmd := &ast.Command{Token: nameTok, Verb: vref}

	// Parse optional target
	if isTarget(p.cur.Type) && !isModifierStart(p) {
		cmd.Target = p.parseExpression()
	}

	// Parse modifiers
	for isModifierStart(p) {
		mod := p.parseModifier()
		if mod == nil {
			break
		}
		cmd.Modifiers = append(cmd.Modifiers, mod)
	}

	// Parse redirect
	if p.cur.Type == token.ArrowR || p.cur.Type == token.ArrowL {
		cmd.Redirect = p.parseRedirect()
	}

	// Check for pipeline operators
	if !isPipelineOp(p.cur.Type) {
		return cmd
	}

	pipe := &ast.Pipeline{
		Token:    tok,
		Commands: []*ast.Command{cmd},
	}
	for isPipelineOp(p.cur.Type) {
		op := p.cur.Type
		pipe.Ops = append(pipe.Ops, op)
		p.next()
		next := p.parseCommand()
		if next == nil {
			p.error("expected command after operator")
			break
		}
		pipe.Commands = append(pipe.Commands, next)
	}
	return pipe
}

// =========================================================================
// Directive: !name args...

func (p *Parser) parseDirective() ast.Statement {
	tok := p.cur // !
	p.next()     // move to name

	if p.cur.Type != token.Ident {
		p.error("expected directive name after !")
		p.recover()
		return nil
	}

	dir := &ast.Directive{
		Token: tok,
		Name:  p.cur.Literal,
	}
	p.next() // move past name

	// Collect args until newline or EOF.
	// Directive args can be plain expressions or key:value modifiers treated as idents.
	for p.cur.Type != token.Newline && p.cur.Type != token.EOF {
		// Handle key:value as a single identifier "key:value"
		if p.cur.Type == token.Ident && p.peek.Type == token.Colon {
			key := p.cur.Literal
			p.next() // skip key
			p.next() // skip :
			val := p.cur.Literal
			p.next() // skip value
			dir.Args = append(dir.Args, &ast.Identifier{Token: p.cur, Value: key + ":" + val})
			continue
		}
		arg := p.parseExpression()
		if arg == nil {
			// Skip unknown token to avoid infinite loop
			p.next()
			continue
		}
		dir.Args = append(dir.Args, arg)
	}

	return dir
}

// =========================================================================
// Pipeline: command (OP command)*

func (p *Parser) parsePipeline() ast.Statement {
	first := p.parseCommand()
	if first == nil {
		return nil
	}

	pipe := &ast.Pipeline{
		Token:    first.Token,
		Commands: []*ast.Command{first},
	}

	for isPipelineOp(p.cur.Type) {
		op := p.cur.Type
		pipe.Ops = append(pipe.Ops, op)
		p.next() // move past operator
		cmd := p.parseCommand()
		if cmd == nil {
			p.error("expected command after operator")
			break
		}
		pipe.Commands = append(pipe.Commands, cmd)
	}

	// Unwrap single-command pipeline
	if len(pipe.Commands) == 1 {
		return pipe.Commands[0]
	}

	return pipe
}

// =========================================================================
// Command: verb [target] [modifier]* [redirect]

func (p *Parser) parseCommand() *ast.Command {
	// Handle modifier-only commands (e.g. priority:high after ? operator).
	// If cur is IDENT and peek is COLON but there's no verb before it,
	// treat the whole thing as a command with the key as verb and value as target.
	if isModifierStart(p) {
		return p.parseModifierAsCommand()
	}

	verb := p.parseVerb()
	if verb == nil {
		return nil
	}

	cmd := &ast.Command{
		Token: p.cur,
		Verb:  verb,
	}

	// Optional target (positional arg)
	if isTarget(p.cur.Type) && !isModifierStart(p) {
		cmd.Target = p.parseExpression()
	}

	// Modifiers: key:value
	for isModifierStart(p) {
		mod := p.parseModifier()
		if mod == nil {
			break
		}
		cmd.Modifiers = append(cmd.Modifiers, mod)
	}

	// Optional redirect
	if p.cur.Type == token.ArrowR || p.cur.Type == token.ArrowL {
		cmd.Redirect = p.parseRedirect()
	}

	return cmd
}

// parseModifierAsCommand handles cases like `priority:high` after a ? operator
// where there's no explicit verb -- the key:value IS the predicate.
func (p *Parser) parseModifierAsCommand() *ast.Command {
	cmd := &ast.Command{Token: p.cur}
	for isModifierStart(p) {
		mod := p.parseModifier()
		if mod == nil {
			break
		}
		cmd.Modifiers = append(cmd.Modifiers, mod)
	}
	// Use first modifier key as a synthetic verb for String() output.
	if len(cmd.Modifiers) > 0 {
		cmd.Verb = &ast.Identifier{Token: cmd.Token, Value: ""}
	}
	return cmd
}

func (p *Parser) parseVerb() ast.Expression {
	switch p.cur.Type {
	case token.At:
		return p.parseMCPCall()
	case token.Ident:
		id := &ast.Identifier{
			Token: p.cur,
			Value: p.cur.Literal,
		}
		p.next()
		return id
	default:
		return nil
	}
}

func (p *Parser) parseMCPCall() ast.Expression {
	tok := p.cur // @
	p.next()     // move to server name

	if p.cur.Type != token.Ident {
		p.error("expected server name after @")
		return nil
	}
	server := p.cur.Literal
	p.next() // move to verb

	if p.cur.Type != token.Ident {
		p.error("expected verb after @%s", server)
		return nil
	}
	verb := p.cur.Literal
	p.next()

	return &ast.MCPCall{
		Token:  tok,
		Server: server,
		Verb:   verb,
	}
}

// =========================================================================
// Modifier: key:value

func (p *Parser) parseModifier() *ast.Modifier {
	if p.cur.Type != token.Ident || p.peek.Type != token.Colon {
		return nil
	}

	mod := &ast.Modifier{
		Token: p.cur,
		Key:   p.cur.Literal,
	}
	p.next() // move past key
	p.next() // move past :

	// Value can be ident, path, number, glob, string, or an operator-prefixed value
	if p.cur.Type == token.EOF || p.cur.Type == token.Newline {
		p.error("expected value after %s:", mod.Key)
		return nil
	}
	mod.Value = p.cur.Literal
	p.next()

	return mod
}

// =========================================================================
// Redirect: -> path | <- path

func (p *Parser) parseRedirect() *ast.Redirect {
	r := &ast.Redirect{
		Token:     p.cur,
		Direction: p.cur.Type,
	}
	p.next() // move past arrow

	if p.cur.Type != token.Ident && p.cur.Type != token.Path && p.cur.Type != token.String {
		p.error("expected path after redirect")
		return nil
	}
	r.Path = p.cur.Literal
	p.next()

	return r
}

// =========================================================================
// Expression parsing

func (p *Parser) parseExpression() ast.Expression {
	switch p.cur.Type {
	case token.Ident:
		e := &ast.Identifier{Token: p.cur, Value: p.cur.Literal}
		p.next()
		return e
	case token.String:
		e := &ast.StringLiteral{Token: p.cur, Value: p.cur.Literal}
		p.next()
		return e
	case token.Number:
		e := &ast.NumberLiteral{Token: p.cur, Value: p.cur.Literal}
		p.next()
		return e
	case token.Path:
		e := &ast.PathLiteral{Token: p.cur, Value: p.cur.Literal}
		p.next()
		return e
	case token.Glob:
		e := &ast.GlobLiteral{Token: p.cur, Value: p.cur.Literal}
		p.next()
		return e
	case token.Dollar:
		tok := p.cur
		p.next()
		if p.cur.Type != token.Ident {
			p.error("expected variable name after $")
			return nil
		}
		e := &ast.VariableRef{Token: tok, Name: p.cur.Literal}
		p.next()
		return e
	default:
		return nil
	}
}

// =========================================================================
// Helpers

func (p *Parser) next() {
	p.cur = p.peek
	p.peek = p.l.NextToken()
}

func (p *Parser) skipNewlines() {
	for p.cur.Type == token.Newline {
		p.next()
	}
}

func (p *Parser) error(format string, args ...any) {
	msg := fmt.Sprintf("%d:%d: %s", p.cur.Line, p.cur.Col, fmt.Sprintf(format, args...))
	p.errors = append(p.errors, msg)
}

func (p *Parser) recover() {
	for p.cur.Type != token.Newline && p.cur.Type != token.EOF {
		p.next()
	}
}

func isPipelineOp(t token.TokenType) bool {
	switch t {
	case token.Pipe, token.Semicolon, token.Ampersand,
		token.Question, token.And, token.Or:
		return true
	}
	return false
}

func isTarget(t token.TokenType) bool {
	switch t {
	case token.Ident, token.String, token.Number,
		token.Path, token.Glob, token.Dollar:
		return true
	}
	return false
}

func isModifierStart(p *Parser) bool {
	return p.cur.Type == token.Ident && p.peek.Type == token.Colon
}
