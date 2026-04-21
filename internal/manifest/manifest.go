// Package manifest parses BANISH project manifests and BANISH.md server
// manifests that map MCP tools to compact verb syntax.
package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.bani.sh/banish/internal/ast"
	"go.bani.sh/banish/internal/lexer"
	"go.bani.sh/banish/internal/parser"
)

// BanishFile represents a parsed BANISH project manifest.
type BanishFile struct {
	Path     string
	Servers  []ServerDecl
	Mappings []VerbMapping
	Verbs    []VerbDef
	Filters  []FilterDef
	Config   ProjectConfig
	Examples []string
}

// FilterDef is a project-specific output filter.
type FilterDef struct {
	Name    string
	Match   string // command pattern
	Compact string // shell command for filtering (receives stdin)
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
		if _, err := os.Stat(candidate); err == nil {
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
				if prop.Name == "server" || prop.Name == "map" || prop.Name == "verb" ||
					prop.Name == "config" || prop.Name == "examples" {
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
				if prop.Name == "server" || prop.Name == "map" || prop.Name == "verb" ||
					prop.Name == "config" || prop.Name == "examples" {
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
				if prop.Name == "server" || prop.Name == "map" || prop.Name == "verb" ||
					prop.Name == "config" || prop.Name == "examples" {
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
				if prop.Name == "server" || prop.Name == "map" || prop.Name == "verb" ||
					prop.Name == "filter" || prop.Name == "config" || prop.Name == "examples" {
					break
				}
				val := argsString(prop.Args)
				switch prop.Name {
				case "match":
					f.Match = val
				case "compact":
					f.Compact = val
				}
			}
			bf.Filters = append(bf.Filters, f)

		case "config":
			for j := i + 1; j < len(prog.Statements); j++ {
				prop, ok := prog.Statements[j].(*ast.Directive)
				if !ok {
					break
				}
				if prop.Name == "server" || prop.Name == "map" || prop.Name == "verb" ||
					prop.Name == "config" || prop.Name == "examples" {
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
					if d.Name == "server" || d.Name == "map" || d.Name == "verb" ||
						d.Name == "config" || d.Name == "examples" {
						break
					}
				}
				bf.Examples = append(bf.Examples, stmt.String())
			}
		}
	}

	return bf, nil
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
