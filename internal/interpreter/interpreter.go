// Package interpreter implements the tree-walking evaluator that executes
// an AST by resolving verbs and dispatching to handlers.
package interpreter

import (
	"context"
	"fmt"
	"io"
	"os"

	"go.bani.sh/banish/internal/ast"
	"go.bani.sh/banish/internal/lexer"
	"go.bani.sh/banish/internal/parser"
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

	// Detected as .bsh syntactically, but a single bare command whose verb is
	// not a registered .bsh verb is really a plain shell command. Executing it
	// through the .bsh command grammar would drop positional arguments (the
	// grammar keeps only verb + one target + key:value modifiers), so run the
	// original string verbatim through the shell instead. This is what makes
	// commands like `go test ./internal/scaffold/` keep their path.
	l := lexer.New(source)
	p := parser.New(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		// Not valid .bsh after all; run verbatim through the shell.
		return interp.evalShell(source)
	}
	if interp.isPlainShellCommand(prog) {
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

// isPlainShellCommand reports whether prog is a single bare command whose verb
// is not a registered .bsh verb -- i.e. a shell command that would only reach
// the system fallback, where the lossy verb+target grammar drops arguments.
func (interp *Interpreter) isPlainShellCommand(prog *ast.Program) bool {
	if len(prog.Statements) != 1 {
		return false
	}
	cmd, ok := prog.Statements[0].(*ast.Command)
	if !ok {
		return false
	}
	name := interp.verbName(cmd)
	return name != "" && name != "__shell__" && !interp.registry.Has(name)
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
