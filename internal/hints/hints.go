// Package hints generates _hint fields that suggest shorter banish alternatives
// for bash commands executed via system fallback.
package hints

import (
	"strings"

	"go.bani.sh/banish/internal/interpreter"
)

// Hinter maps bash command patterns to banish equivalents.
type Hinter struct {
	mappings []mapping
}

type mapping struct {
	cmd     string   // bash command name (e.g. "find", "curl", "cat")
	aliases []string // alternative names
	banish  string   // banish equivalent pattern
	why     string   // brief explanation
}

// New creates a Hinter with built-in mappings.
func New() *Hinter {
	return &Hinter{
		mappings: defaultMappings,
	}
}

// Suggest checks if a bash command has a banish builtin equivalent.
// Returns nil if no mapping exists. Always suggests when a builtin exists
// because even same-length commands benefit from structured output.
func (h *Hinter) Suggest(cmdName string, args []string) *interpreter.Hint {
	for _, m := range h.mappings {
		if m.cmd == cmdName || contains(m.aliases, cmdName) {
			shorter := h.buildSuggestion(m, args)
			original := cmdName + " " + strings.Join(args, " ")
			saved := estimateTokens(original) - estimateTokens(shorter)
			if saved < 0 {
				saved = 0
			}
			return &interpreter.Hint{
				Shorter: shorter,
				Saved:   saved,
				Why:     m.why,
			}
		}
	}
	return nil
}

func (h *Hinter) buildSuggestion(m mapping, args []string) string {
	switch m.cmd {
	case "find":
		return h.suggestFind(args)
	case "curl":
		return h.suggestCurl(args)
	case "cat":
		if len(args) > 0 {
			return "read " + args[0]
		}
		return "read"
	case "grep":
		return h.suggestGrep(args)
	case "gzip":
		if len(args) > 0 {
			return "gz " + args[0]
		}
		return "gz"
	case "gunzip":
		if len(args) > 0 {
			return "ungz " + args[0]
		}
		return "ungz"
	case "wc":
		return "count"
	case "mkdir":
		if len(args) > 0 {
			last := args[len(args)-1]
			return "mkdir " + last
		}
		return m.banish
	default:
		return m.banish
	}
}

func (h *Hinter) suggestFind(args []string) string {
	var dir, ext, name string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-name":
			if i+1 < len(args) {
				i++
				pattern := strings.Trim(args[i], "\"'")
				if strings.HasPrefix(pattern, "*.") {
					ext = strings.TrimPrefix(pattern, "*.")
				} else {
					name = pattern
				}
			}
		default:
			if !strings.HasPrefix(args[i], "-") && dir == "" {
				dir = args[i]
			}
		}
	}

	result := "ls"
	if dir != "" {
		result += " " + dir
	}
	if ext != "" {
		result += " ext:" + ext
	}
	if name != "" {
		result += " name:" + name
	}
	return result
}

func (h *Hinter) suggestCurl(args []string) string {
	var url string
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			url = a
			break
		}
	}
	if url != "" {
		return "fetch " + url
	}
	return "fetch"
}

func (h *Hinter) suggestGrep(args []string) string {
	if len(args) >= 2 {
		return "read " + args[len(args)-1] + " ? " + args[0]
	}
	return "? (filter)"
}

var defaultMappings = []mapping{
	{cmd: "find", banish: "ls", why: "structured query vs raw find"},
	{cmd: "cat", banish: "read", why: "read returns structured data"},
	{cmd: "curl", aliases: []string{"wget"}, banish: "fetch", why: "fetch returns status+body+headers as JSON"},
	{cmd: "grep", aliases: []string{"rg", "ag"}, banish: "? (filter)", why: "filter operator on piped data"},
	{cmd: "gzip", banish: "gz", why: "shorter verb name"},
	{cmd: "gunzip", banish: "ungz", why: "shorter verb name"},
	{cmd: "wc", banish: "count", why: "count items or lines"},
	{cmd: "mkdir", aliases: []string{"mkdir"}, banish: "mkdir", why: "same verb, structured output"},
	{cmd: "rm", aliases: []string{"rmdir"}, banish: "rm", why: "same verb, structured output"},
	{cmd: "cp", banish: "cp", why: "same verb, structured output"},
	{cmd: "mv", banish: "mv", why: "same verb, structured output"},
	{cmd: "head", banish: "head", why: "same verb, structured output"},
	{cmd: "tail", banish: "tail", why: "same verb, structured output"},
	{cmd: "sort", banish: "sort", why: "same verb, structured output"},
	{cmd: "uniq", banish: "uniq", why: "same verb, structured output"},
}

// estimateTokens estimates token count using ~4 chars per token heuristic.
func estimateTokens(s string) int {
	n := len(s) / 4
	if n == 0 && len(s) > 0 {
		n = 1
	}
	return n
}

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
