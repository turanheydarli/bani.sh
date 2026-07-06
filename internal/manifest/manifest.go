// Package manifest parses BANISH project manifests and BANISH.md server
// manifests that map MCP tools to compact verb syntax.
package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.banish.sh/banish/internal/ast"
	"go.banish.sh/banish/internal/compact"
	"go.banish.sh/banish/internal/lexer"
	"go.banish.sh/banish/internal/parser"
)

// BanishFile represents a parsed BANISH project manifest.
type BanishFile struct {
	Path     string
	Servers  []ServerDecl
	Mappings []VerbMapping
	Verbs    []VerbDef
	Filters  []FilterDef
	Rewrites []RewriteDef
	Config   ProjectConfig
	Examples []string
}

// FilterDef is a project-specific output filter.
type FilterDef struct {
	Name    string
	Match   string // tokenized command prefix
	Compact string // shell command for filtering (receives stdin)
	Ops     compact.FilterOps
}

// RewriteDef swaps a command for a machine-readable variant before it runs.
type RewriteDef struct {
	Name   string
	Match  string
	Unless []string
	To     string
}

// ServerDecl declares an MCP server.
type ServerDecl struct {
	Name    string
	Command string
	Auth    string
}

// VerbMapping maps a banish verb to an MCP tool.
type VerbMapping struct {
	Verb     string
	Server   string
	Tool     string
	Args     string
	Defaults string
}

// VerbDef is a project-specific verb definition.
type VerbDef struct {
	Name   string
	Args   string
	Expand string
	Help   string
}

// ProjectConfig holds project-level configuration.
type ProjectConfig struct {
	Timeout string
	Output  string
}

// FindBanishFile walks up from dir to find a BANISH file.
// Stops at filesystem root or a .git directory.
// Returns empty string if not found (not an error).
func FindBanishFile(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}

	for {
		candidate := filepath.Join(abs, "BANISH")
		// A manifest must be a regular file. A directory (or socket, fifo, ...)
		// named BANISH is not a match: skip it and keep walking up, exactly as
		// if nothing was found at this level.
		if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
			return candidate
		}

		// Stop at .git boundary
		if _, err := os.Stat(filepath.Join(abs, ".git")); err == nil {
			return ""
		}

		parent := filepath.Dir(abs)
		if parent == abs {
			return "" // filesystem root
		}
		abs = parent
	}
}

// LoadBanishFile parses a BANISH file and returns its contents.
func LoadBanishFile(path string) (*BanishFile, error) {
	// Guard against callers passing a non-regular file (FindBanishFile already
	// filters these; this keeps other entry points from repeating the mistake).
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("manifest.Load: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("manifest.Load: %s is not a regular file", path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("manifest.Load: %w", err)
	}

	return ParseBanishFile(string(data), path)
}

// ParseBanishFile parses BANISH file content from a string.
func ParseBanishFile(content string, path string) (*BanishFile, error) {
	l := lexer.New(content)
	p := parser.New(l)
	prog := p.ParseProgram()

	if errs := p.Errors(); len(errs) > 0 {
		return nil, fmt.Errorf("manifest parse: %s", errs[0])
	}

	bf := &BanishFile{Path: path}

	for i := 0; i < len(prog.Statements); i++ {
		dir, ok := prog.Statements[i].(*ast.Directive)
		if !ok {
			continue
		}

		switch dir.Name {
		case "server":
			s := ServerDecl{}
			if len(dir.Args) >= 1 {
				s.Name = dir.Args[0].String()
			}
			// Look ahead for indented key:value properties
			for j := i + 1; j < len(prog.Statements); j++ {
				prop, ok := prog.Statements[j].(*ast.Directive)
				if !ok {
					break
				}
				if isSectionName(prop.Name) {
					break
				}
				val := argsString(prop.Args)
				switch prop.Name {
				case "command":
					s.Command = val
				case "auth":
					s.Auth = val
				}
			}
			bf.Servers = append(bf.Servers, s)

		case "map":
			m := VerbMapping{}
			args := flatArgs(dir.Args)
			// Format: !map verb -> server:tool
			if len(args) >= 1 {
				m.Verb = args[0]
			}
			// Find -> and parse server:tool
			for k := 1; k < len(args); k++ {
				if args[k] == "->" && k+1 < len(args) {
					parts := strings.SplitN(args[k+1], ":", 2)
					if len(parts) == 2 {
						m.Server = parts[0]
						m.Tool = parts[1]
					} else {
						m.Tool = args[k+1]
					}
				}
			}
			// Look ahead for args, defaults
			for j := i + 1; j < len(prog.Statements); j++ {
				prop, ok := prog.Statements[j].(*ast.Directive)
				if !ok {
					break
				}
				if isSectionName(prop.Name) {
					break
				}
				val := argsString(prop.Args)
				switch prop.Name {
				case "args":
					m.Args = val
				case "defaults":
					m.Defaults = val
				}
			}
			bf.Mappings = append(bf.Mappings, m)

		case "verb":
			v := VerbDef{}
			if len(dir.Args) >= 1 {
				v.Name = dir.Args[0].String()
			}
			for j := i + 1; j < len(prog.Statements); j++ {
				prop, ok := prog.Statements[j].(*ast.Directive)
				if !ok {
					break
				}
				if isSectionName(prop.Name) {
					break
				}
				val := argsString(prop.Args)
				switch prop.Name {
				case "args":
					v.Args = val
				case "expand":
					v.Expand = val
				case "help":
					v.Help = strings.Trim(val, "\"")
				}
			}
			bf.Verbs = append(bf.Verbs, v)

		case "filter":
			f := FilterDef{}
			if len(dir.Args) >= 1 {
				f.Name = dir.Args[0].String()
			}
			for j := i + 1; j < len(prog.Statements); j++ {
				prop, ok := prog.Statements[j].(*ast.Directive)
				if !ok {
					break
				}
				if isSectionName(prop.Name) {
					break
				}
				val := unquotedArgsString(prop.Args)
				switch prop.Name {
				case "match":
					f.Match = val
				case "compact":
					f.Compact = val
				case "drop":
					f.Ops.Drop = joinRe(f.Ops.Drop, val)
				case "keep":
					f.Ops.Keep = joinRe(f.Ops.Keep, val)
				case "sub":
					if args := unquotedArgs(prop.Args); len(args) >= 1 {
						r := compact.SubRule{Pattern: args[0]}
						if len(args) >= 2 {
							r.Replace = args[1]
						}
						f.Ops.Sub = append(f.Ops.Sub, r)
					}
				case "tally":
					if args := unquotedArgs(prop.Args); len(args) >= 2 {
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
			bf.Filters = append(bf.Filters, f)

		case "rewrite":
			rw := RewriteDef{}
			if len(dir.Args) >= 1 {
				rw.Name = dir.Args[0].String()
			}
			for j := i + 1; j < len(prog.Statements); j++ {
				prop, ok := prog.Statements[j].(*ast.Directive)
				if !ok {
					break
				}
				if isSectionName(prop.Name) {
					break
				}
				val := unquotedArgsString(prop.Args)
				switch prop.Name {
				case "match":
					rw.Match = val
				case "to":
					rw.To = val
				case "unless":
					rw.Unless = append(rw.Unless, strings.Fields(val)...)
				}
			}
			bf.Rewrites = append(bf.Rewrites, rw)

		case "config":
			for j := i + 1; j < len(prog.Statements); j++ {
				prop, ok := prog.Statements[j].(*ast.Directive)
				if !ok {
					break
				}
				if isSectionName(prop.Name) {
					break
				}
				val := strings.Trim(argsString(prop.Args), "\"")
				switch prop.Name {
				case "timeout":
					bf.Config.Timeout = val
				case "output":
					bf.Config.Output = val
				}
			}

		case "examples":
			for j := i + 1; j < len(prog.Statements); j++ {
				stmt := prog.Statements[j]
				if d, ok := stmt.(*ast.Directive); ok {
					if isSectionName(d.Name) {
						break
					}
				}
				bf.Examples = append(bf.Examples, stmt.String())
			}
		}
	}

	return bf, nil
}

// isSectionName reports whether a directive starts a new top-level section,
// ending the lookahead of the section being parsed.
func isSectionName(name string) bool {
	switch name {
	case "server", "map", "verb", "filter", "rewrite", "config", "examples":
		return true
	}
	return false
}

// unquotedArgs returns each directive arg as its own string, unquoting
// string literals. Used by two-argument ops like !sub and !tally.
func unquotedArgs(args []ast.Expression) []string {
	var parts []string
	for _, a := range args {
		if sl, ok := a.(*ast.StringLiteral); ok {
			parts = append(parts, sl.Value)
		} else {
			parts = append(parts, a.String())
		}
	}
	return parts
}

// unquotedArgsString joins directive args, unquoting string literals so
// regexes and shell pipes survive intact.
func unquotedArgsString(args []ast.Expression) string {
	return strings.Join(unquotedArgs(args), " ")
}

// joinRe accumulates repeated regex directives into one alternation.
func joinRe(existing, add string) string {
	if existing == "" {
		return add
	}
	return existing + "|" + add
}

// atoiSafe parses a non-negative integer, returning 0 on any error.
func atoiSafe(s string) int {
	n := 0
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}

func argsString(args []ast.Expression) string {
	var parts []string
	for _, a := range args {
		parts = append(parts, a.String())
	}
	return strings.Join(parts, " ")
}

func flatArgs(args []ast.Expression) []string {
	var out []string
	for _, a := range args {
		out = append(out, a.String())
	}
	return out
}
