// Package interpreter implements the tree-walking evaluator that executes
// an AST by resolving verbs and dispatching to handlers.
package interpreter

import (
	"context"
	"fmt"
	"io"
	"os"

	"go.banish.sh/banish/internal/ast"
	"go.banish.sh/banish/internal/lexer"
	"go.banish.sh/banish/internal/parser"
)

// Interpreter walks an AST and executes commands.
type Interpreter struct {
	registry *VerbRegistry
	env      *Environment
	out      io.Writer
	human    bool // true = human-readable output
	verbose  bool
	ctx      context.Context
}

// Option configures the interpreter.
type Option func(*Interpreter)

// WithRegistry sets the verb registry.
func WithRegistry(r *VerbRegistry) Option {
	return func(i *Interpreter) { i.registry = r }
}

// WithOutput sets the output writer.
func WithOutput(w io.Writer) Option {
	return func(i *Interpreter) { i.out = w }
}

// WithContext sets the execution context.
func WithContext(ctx context.Context) Option {
	return func(i *Interpreter) { i.ctx = ctx }
}

// New creates an interpreter with the given options.
func New(opts ...Option) *Interpreter {
	interp := &Interpreter{
		registry: NewVerbRegistry(),
		env:      NewEnvironment(),
		out:      os.Stdout,
		ctx:      context.Background(),
	}
	for _, opt := range opts {
		opt(interp)
	}
	return interp
}

// EvalSource detects whether input is .bsh or bash, then executes accordingly.
// This is the dual input mode entry point.
func (interp *Interpreter) EvalSource(source string) (*Result, error) {
	if Detect(source) == ModeBash {
		return interp.evalShell(source)
	}

	// Detected as .bsh syntactically, but the .bsh command grammar is lossy
	// for shell commands: it keeps only verb + one target + key:value
	// modifiers, so paths like ./... are silently dropped. Only execute the
	// parse tree when every verb in it is a registered .bsh verb; otherwise
	// the input is really a shell command (or a pipeline containing one) and
	// must run verbatim through the shell. Deciding which filter applies is
	// done later on a copy of the string -- never on the command that runs.
	l := lexer.New(source)
	p := parser.New(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		// Not valid .bsh after all; run verbatim through the shell.
		return interp.evalShell(source)
	}
	if !interp.allVerbsRegistered(prog) {
		return interp.evalShell(source)
	}

	return interp.Eval(prog)
}

// evalShell runs the original command string verbatim through the system
// fallback's shell executor, preserving every argument exactly. It falls back
// to .bsh evaluation only when no fallback handler is registered.
func (interp *Interpreter) evalShell(source string) (*Result, error) {
	handler, err := interp.registry.Resolve("__fallback__")
	if err != nil {
		return interp.evalBSH(source)
	}
	cmd := &ast.Command{
		Verb:   &ast.Identifier{Value: "__shell__"},
		Target: &ast.StringLiteral{Value: source},
	}
	return handler(interp.ctx, cmd, nil)
}

// allVerbsRegistered walks the program and reports whether every command
// resolves to a registered .bsh verb. A single unregistered verb anywhere
// (including inside a pipeline stage or assignment) means the input is a
// shell command and must not be executed through the lossy .bsh grammar.
func (interp *Interpreter) allVerbsRegistered(prog *ast.Program) bool {
	for _, stmt := range prog.Statements {
		if !interp.statementVerbsRegistered(stmt) {
			return false
		}
	}
	return true
}

func (interp *Interpreter) statementVerbsRegistered(stmt ast.Statement) bool {
	switch s := stmt.(type) {
	case *ast.Command:
		return interp.commandVerbRegistered(s)
	case *ast.Pipeline:
		for _, cmd := range s.Commands {
			if !interp.commandVerbRegistered(cmd) {
				return false
			}
		}
		return true
	case *ast.Assignment:
		return interp.statementVerbsRegistered(s.Value)
	case *ast.Directive:
		return true // directives are .bsh-only syntax
	default:
		return true
	}
}

func (interp *Interpreter) commandVerbRegistered(cmd *ast.Command) bool {
	switch cmd.Verb.(type) {
	case *ast.MCPCall, *ast.VariableRef:
		return true // .bsh-only constructs
	}
	name := interp.verbName(cmd)
	if name == "" || name == "__shell__" {
		return true
	}
	return interp.registry.Has(name)
}

func (interp *Interpreter) evalBSH(source string) (*Result, error) {
	l := lexer.New(source)
	p := parser.New(l)
	prog := p.ParseProgram()

	if errs := p.Errors(); len(errs) > 0 {
		return nil, fmt.Errorf("%s", errs[0])
	}

	return interp.Eval(prog)
}

// Eval executes a parsed program and returns the result of the last statement.
func (interp *Interpreter) Eval(prog *ast.Program) (*Result, error) {
	var last *Result
	for _, stmt := range prog.Statements {
		r, err := interp.evalStatement(stmt)
		if err != nil {
			return nil, err
		}
		last = r
	}
	return last, nil
}

func (interp *Interpreter) evalStatement(stmt ast.Statement) (*Result, error) {
	switch s := stmt.(type) {
	case *ast.Command:
		return interp.evalCommand(s, nil)
	case *ast.Pipeline:
		return interp.evalPipeline(s)
	case *ast.Assignment:
		return interp.evalAssignment(s)
	case *ast.Directive:
		return interp.evalDirective(s)
	default:
		return nil, fmt.Errorf("unknown statement type: %T", stmt)
	}
}

func (interp *Interpreter) evalCommand(cmd *ast.Command, input *Result) (*Result, error) {
	name := interp.verbName(cmd)

	// Resolve target if it is a variable reference.
	if vref, ok := cmd.Target.(*ast.VariableRef); ok {
		if val, found := interp.env.Get(vref.Name); found {
			input = val
		}
	}

	handler, err := interp.registry.Resolve(name)
	if err != nil {
		return nil, fmt.Errorf("%d:%d: %w", cmd.Token.Line, cmd.Token.Col, err)
	}

	return handler(interp.ctx, cmd, input)
}

// evalPipeline is implemented in pipeline.go

func (interp *Interpreter) evalAssignment(a *ast.Assignment) (*Result, error) {
	val, err := interp.evalStatement(a.Value)
	if err != nil {
		return nil, fmt.Errorf("assignment $%s: %w", a.Name, err)
	}
	interp.env.Set(a.Name, val)
	return val, nil
}

func (interp *Interpreter) evalDirective(d *ast.Directive) (*Result, error) {
	switch d.Name {
	case "human":
		interp.human = true
		return NewResult("output: human"), nil
	case "verbose":
		interp.verbose = true
		return NewResult("verbose: on"), nil
	case "quiet":
		interp.human = false
		interp.verbose = false
		return NewResult("quiet: on"), nil
	default:
		return NewResult(fmt.Sprintf("directive: %s", d.Name)), nil
	}
}

// verbName extracts the verb name string from a command.
func (interp *Interpreter) verbName(cmd *ast.Command) string {
	switch v := cmd.Verb.(type) {
	case *ast.Identifier:
		return v.Value
	case *ast.MCPCall:
		return "@" + v.Server + ":" + v.Verb
	case *ast.VariableRef:
		// $var used as verb: look up and return its string value as the verb.
		if val, ok := interp.env.Get(v.Name); ok {
			return fmt.Sprintf("%v", val.Data)
		}
		return "$" + v.Name
	default:
		return ""
	}
}

// Human returns whether human-readable output mode is active.
func (interp *Interpreter) Human() bool {
	return interp.human
}

// Registry returns the verb registry for external registration.
func (interp *Interpreter) Registry() *VerbRegistry {
	return interp.registry
}
