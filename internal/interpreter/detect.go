package interpreter

import (
	"strings"

	"go.bani.sh/banish/internal/lexer"
	"go.bani.sh/banish/internal/parser"
)

// InputMode indicates whether input was detected as .bsh or bash.
type InputMode int

const (
	ModeBSH  InputMode = iota // valid .bsh syntax
	ModeBash                  // bash/shell command
)

// bashKeywords are tokens that only appear in bash, never in .bsh.
// If the input starts with one of these, skip .bsh parsing entirely.
var bashKeywords = map[string]bool{
	"if": true, "then": true, "else": true, "elif": true, "fi": true,
	"for": true, "do": true, "done": true, "while": true, "until": true,
	"case": true, "esac": true, "function": true,
	"export": true, "source": true, "alias": true, "unalias": true,
	"set": true, "unset": true, "declare": true, "local": true,
	"eval": true, "trap": true, "shift": true,
}

// Detect determines if input is .bsh or bash syntax.
func Detect(input string) InputMode {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return ModeBSH
	}

	// Fast check: if first word is a bash keyword, skip .bsh parsing.
	firstWord := firstToken(trimmed)
	if bashKeywords[firstWord] {
		return ModeBash
	}

	// Check for bash-style flags (--flag or -flag with letters).
	// If the input contains --something or -x style flags, it is likely bash.
	if containsBashFlags(trimmed) {
		return ModeBash
	}

	// Try .bsh parse. If it succeeds with zero errors, it is .bsh.
	l := lexer.New(input)
	p := parser.New(l)
	prog := p.ParseProgram()

	if len(p.Errors()) == 0 && len(prog.Statements) > 0 {
		return ModeBSH
	}

	return ModeBash
}

func firstToken(s string) string {
	for i, ch := range s {
		if ch == ' ' || ch == '\t' || ch == '\n' {
			return s[:i]
		}
	}
	return s
}

// containsBashFlags checks for --flag or -x patterns that indicate bash syntax.
// Banish uses key:value modifiers, not dashed flags.
func containsBashFlags(s string) bool {
	parts := strings.Fields(s)
	for _, p := range parts[1:] { // skip first word (the command name)
		if strings.HasPrefix(p, "--") {
			return true
		}
		// Single-dash flags: -x, -rf, -name (but not ->)
		if len(p) >= 2 && p[0] == '-' && p[1] != '>' && isLetter(p[1]) {
			return true
		}
	}
	return false
}

func isLetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}
