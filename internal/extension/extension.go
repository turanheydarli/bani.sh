// Package extension implements the verb registry and .bsh extension loader.
// Extensions define new verbs that are resolved via a trie-based registry.
package extension

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"go.banish.sh/banish/internal/ast"
	"go.banish.sh/banish/internal/compact"
	"go.banish.sh/banish/internal/interpreter"
	"go.banish.sh/banish/internal/lexer"
	"go.banish.sh/banish/internal/parser"
	"go.banish.sh/banish/internal/shell"
)

// VerbDef is a verb defined in an extension file.
type VerbDef struct {
	Name   string
	Args   []string // argument names, optional marked with ?
	Expand string   // the .bsh code to execute
	Help   string
}

// FilterDef is an output filter defined in an extension file.
// Filters compact command output via a shell pipe (!compact) and/or
// declarative ops (!drop, !keep, !max-lines, !max-line-len, !group-by,
// !per-group, !overflow) that run in-process.
type FilterDef struct {
	Name    string // filter name (for logging)
	Match   string // tokenized command prefix to match
	Compact string // shell command that receives raw output on stdin
	Ops     compact.FilterOps
}

// RewriteDef swaps a command for a machine-readable variant before it runs.
// Defined via !rewrite / !match / !unless / !to.
type RewriteDef struct {
	Name   string
	Match  string   // tokenized command prefix, e.g. "git status"
	Unless []string // flags that disable the rewrite
	To     string   // replacement command prefix
}

// ExtensionMeta holds metadata from the !extension directive.
type ExtensionMeta struct {
	Name     string
	Version  string
	Verbs    []VerbDef
	Filters  []FilterDef
	Rewrites []RewriteDef
}

// Loader reads and parses .bsh extension files.
type Loader struct {
	extensions []ExtensionMeta
}

// NewLoader creates an extension loader.
func NewLoader() *Loader {
	return &Loader{}
}

// LoadDir loads all .bsh files from a directory.
func (l *Loader) LoadDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // dir doesn't exist yet, that's fine
		}
		return fmt.Errorf("extension.LoadDir: %w", err)
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".bsh") {
			continue
		}
		if err := l.LoadFile(filepath.Join(dir, e.Name())); err != nil {
			log.Printf("extension: skip %s: %v", e.Name(), err)
		}
	}

	return nil
}

//go:embed defaults.bsh
var defaultsBSH string

// LoadDefaults parses the embedded default extension: built-in rewrites and
// filters expressed in the same .bsh DSL as user extensions. Load it AFTER
// user directories so user definitions take precedence.
func (l *Loader) LoadDefaults() {
	if err := l.LoadSource("defaults.bsh", defaultsBSH); err != nil {
		log.Printf("extension: embedded defaults: %v", err)
	}
}

// LoadFile parses a single .bsh extension file.
func (l *Loader) LoadFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("extension.LoadFile: %w", err)
	}
	return l.LoadSource(path, string(data))
}

// LoadSource parses .bsh extension source.
func (l *Loader) LoadSource(path, source string) error {
	lex := lexer.New(source)
	p := parser.New(lex)
	prog := p.ParseProgram()

	if errs := p.Errors(); len(errs) > 0 {
		return fmt.Errorf("parse %s: %s", path, errs[0])
	}

	ext := ExtensionMeta{}

	for _, stmt := range prog.Statements {
		dir, ok := stmt.(*ast.Directive)
		if !ok {
			continue
		}

		switch dir.Name {
		case "extension":
			if len(dir.Args) >= 1 {
				ext.Name = dir.Args[0].String()
			}
			if len(dir.Args) >= 2 {
				ext.Version = dir.Args[1].String()
			}

		case "verb":
			verb := VerbDef{}
			if len(dir.Args) >= 1 {
				verb.Name = dir.Args[0].String()
			}
			ext.Verbs = append(ext.Verbs, verb)

		case "filter":
			filter := FilterDef{}
			if len(dir.Args) >= 1 {
				filter.Name = dir.Args[0].String()
			}
			ext.Filters = append(ext.Filters, filter)

		case "rewrite":
			rw := RewriteDef{}
			if len(dir.Args) >= 1 {
				rw.Name = dir.Args[0].String()
			}
			ext.Rewrites = append(ext.Rewrites, rw)
		}
	}

	// Second pass: find expand/args/help for each verb, match/compact/ops for
	// each filter, and match/unless/to for each rewrite.
	parseVerbDetails(prog, &ext)
	parseFilterDetails(prog, &ext)
	parseRewriteDetails(prog, &ext)

	l.extensions = append(l.extensions, ext)
	return nil
}

// Register registers all loaded extension verbs into the interpreter registry.
func (l *Loader) Register(reg *interpreter.VerbRegistry) {
	for _, ext := range l.extensions {
		for _, verb := range ext.Verbs {
			if verb.Name == "" {
				continue
			}
			handler := makeExtensionHandler(verb)
			reg.RegisterExtension(verb.Name, handler)
		}
	}
}

// Extensions returns all loaded extension metadata.
func (l *Loader) Extensions() []ExtensionMeta {
	return l.extensions
}

// MakeVerbHandler creates a VerbHandler from a name and expand string.
func MakeVerbHandler(name string, expand string) interpreter.VerbHandler {
	return makeExtensionHandler(VerbDef{Name: name, Expand: expand})
}

// makeExtensionHandler creates a VerbHandler from a verb definition.
// The expand string is executed via shell with argument substitution.
func makeExtensionHandler(v VerbDef) interpreter.VerbHandler {
	return func(ctx context.Context, cmd *ast.Command, input *interpreter.Result) (*interpreter.Result, error) {
		if v.Expand == "" {
			return interpreter.NewResult(fmt.Sprintf("verb %s: no expand defined", v.Name)), nil
		}

		// Build the expansion with argument substitution.
		expand := v.Expand
		if cmd.Target != nil {
			expand = strings.ReplaceAll(expand, "$1", cmd.Target.String())
		}
		for _, m := range cmd.Modifiers {
			expand = strings.ReplaceAll(expand, "$"+m.Key, m.Value)
		}

		// Strip leading "exec " prefix if present -- it is a hint that
		// the expansion should run as a shell command.
		script := strings.TrimPrefix(expand, "exec ")

		// Execute via the OS-appropriate shell.
		name, args := shell.Args(script)
		c := exec.CommandContext(ctx, name, args...)
		c.Stderr = os.Stderr
		out, err := c.Output()
		if err != nil {
			return nil, fmt.Errorf("verb %s: %w", v.Name, err)
		}

		return interpreter.NewResult(strings.TrimRight(string(out), "\n")), nil
	}
}

// directiveArgs returns each directive arg as its own string, unquoting
// string literals. Used by two-argument ops like !sub and !tally.
func directiveArgs(dir *ast.Directive) []string {
	var parts []string
	for _, arg := range dir.Args {
		if sl, ok := arg.(*ast.StringLiteral); ok {
			parts = append(parts, sl.Value)
		} else {
			parts = append(parts, arg.String())
		}
	}
	return parts
}

// directiveValue joins directive args into a single string, unquoting
// string literals so regexes and shell pipes survive intact.
func directiveValue(dir *ast.Directive) string {
	return strings.Join(directiveArgs(dir), " ")
}

// joinRe accumulates repeated regex directives into one alternation.
func joinRe(existing, add string) string {
	if existing == "" {
		return add
	}
	return existing + "|" + add
}

// parseFilterDetails extracts match, compact, and declarative ops from
// statements following !filter.
func parseFilterDetails(prog *ast.Program, ext *ExtensionMeta) {
	if len(ext.Filters) == 0 {
		return
	}

	currentFilter := -1
	for _, stmt := range prog.Statements {
		dir, ok := stmt.(*ast.Directive)
		if !ok {
			continue
		}

		switch dir.Name {
		case "filter":
			currentFilter++
		case "verb", "extension", "rewrite":
			// These break filter context
			currentFilter = -1
		default:
			if currentFilter >= 0 && currentFilter < len(ext.Filters) {
				val := directiveValue(dir)
				f := &ext.Filters[currentFilter]

				switch dir.Name {
				case "match":
					f.Match = val
				case "compact":
					f.Compact = val
				case "drop":
					f.Ops.Drop = joinRe(f.Ops.Drop, val)
				case "keep":
					f.Ops.Keep = joinRe(f.Ops.Keep, val)
				case "sub":
					if args := directiveArgs(dir); len(args) >= 1 {
						r := compact.SubRule{Pattern: args[0]}
						if len(args) >= 2 {
							r.Replace = args[1]
						}
						f.Ops.Sub = append(f.Ops.Sub, r)
					}
				case "tally":
					if args := directiveArgs(dir); len(args) >= 2 {
						f.Ops.Tally = append(f.Ops.Tally, compact.TallyRule{Pattern: args[0], Template: args[1]})
					}
				case "group-by":
					f.Ops.GroupBy = val
				case "overflow":
					f.Ops.Overflow = val
				case "per-group":
					f.Ops.PerGroup = atoiSafe(val)
				case "max-lines":
					f.Ops.MaxLines = atoiSafe(val)
				case "max-line-len":
					f.Ops.MaxLineLen = atoiSafe(val)
				}
			}
		}
	}
}

// parseRewriteDetails extracts match, unless, and to from statements
// following !rewrite.
func parseRewriteDetails(prog *ast.Program, ext *ExtensionMeta) {
	if len(ext.Rewrites) == 0 {
		return
	}

	current := -1
	for _, stmt := range prog.Statements {
		dir, ok := stmt.(*ast.Directive)
		if !ok {
			continue
		}

		switch dir.Name {
		case "rewrite":
			current++
		case "verb", "extension", "filter":
			current = -1
		default:
			if current >= 0 && current < len(ext.Rewrites) {
				rw := &ext.Rewrites[current]
				switch dir.Name {
				case "match":
					rw.Match = directiveValue(dir)
				case "to":
					rw.To = directiveValue(dir)
				case "unless":
					rw.Unless = append(rw.Unless, strings.Fields(directiveValue(dir))...)
				}
			}
		}
	}
}

// atoiSafe parses a non-negative integer, returning 0 on any error.
func atoiSafe(s string) int {
	n := 0
	for _, c := range strings.TrimSpace(s) {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// Filters returns all loaded filter definitions across all extensions.
func (l *Loader) Filters() []FilterDef {
	var all []FilterDef
	for _, ext := range l.extensions {
		all = append(all, ext.Filters...)
	}
	return all
}

// Rewrites returns all loaded rewrite definitions across all extensions.
func (l *Loader) Rewrites() []RewriteDef {
	var all []RewriteDef
	for _, ext := range l.extensions {
		all = append(all, ext.Rewrites...)
	}
	return all
}

// parseVerbDetails extracts expand, args, help from statements following !verb.
func parseVerbDetails(prog *ast.Program, ext *ExtensionMeta) {
	if len(ext.Verbs) == 0 {
		return
	}

	currentVerb := -1
	for _, stmt := range prog.Statements {
		dir, ok := stmt.(*ast.Directive)
		if !ok {
			continue
		}

		switch dir.Name {
		case "verb":
			currentVerb++
		case "filter", "rewrite", "extension":
			currentVerb = -1
		default:
			if currentVerb >= 0 && currentVerb < len(ext.Verbs) {
				// Join all args into a single string for the property value.
				var parts []string
				for _, arg := range dir.Args {
					parts = append(parts, arg.String())
				}
				val := strings.Join(parts, " ")

				switch {
				case strings.HasPrefix(dir.Name, "args"):
					ext.Verbs[currentVerb].Args = append(ext.Verbs[currentVerb].Args, parts...)
				case dir.Name == "expand":
					ext.Verbs[currentVerb].Expand = val
				case dir.Name == "help":
					ext.Verbs[currentVerb].Help = strings.Trim(val, "\"")
				}
			}
		}
	}
}
