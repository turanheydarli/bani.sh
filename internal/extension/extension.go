// Package extension implements the verb registry and .bsh extension loader.
// Extensions define new verbs that are resolved via a trie-based registry.
package extension

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"go.bani.sh/banish/internal/ast"
	"go.bani.sh/banish/internal/interpreter"
	"go.bani.sh/banish/internal/lexer"
	"go.bani.sh/banish/internal/parser"
	"go.bani.sh/banish/internal/shell"
)

// VerbDef is a verb defined in an extension file.
type VerbDef struct {
	Name   string
	Args   []string // argument names, optional marked with ?
	Expand string   // the .bsh code to execute
	Help   string
}

// FilterDef is an output filter defined in an extension file.
// Filters compact command output by piping it through a shell script.
type FilterDef struct {
	Name    string // filter name (for logging)
	Match   string // command pattern to match (substring or glob)
	Compact string // shell command that receives raw output on stdin
}

// ExtensionMeta holds metadata from the !extension directive.
type ExtensionMeta struct {
	Name    string
	Version string
	Verbs   []VerbDef
	Filters []FilterDef
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

// LoadFile parses a single .bsh extension file.
func (l *Loader) LoadFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("extension.LoadFile: %w", err)
	}

	lex := lexer.New(string(data))
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
		}
	}

	// Second pass: find expand/args/help for each verb, and match/compact for each filter.
	parseVerbDetails(prog, &ext)
	parseFilterDetails(prog, &ext)

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

// parseFilterDetails extracts match and compact from statements following !filter.
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
		case "verb", "extension":
			// These break filter context
			currentFilter = -1
		default:
			if currentFilter >= 0 && currentFilter < len(ext.Filters) {
				var parts []string
				for _, arg := range dir.Args {
					// Use raw value for string literals (strip quotes)
					if sl, ok := arg.(*ast.StringLiteral); ok {
						parts = append(parts, sl.Value)
					} else {
						parts = append(parts, arg.String())
					}
				}
				val := strings.Join(parts, " ")

				switch dir.Name {
				case "match":
					ext.Filters[currentFilter].Match = val
				case "compact":
					ext.Filters[currentFilter].Compact = val
				}
			}
		}
	}
}

// Filters returns all loaded filter definitions across all extensions.
func (l *Loader) Filters() []FilterDef {
	var all []FilterDef
	for _, ext := range l.extensions {
		all = append(all, ext.Filters...)
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
