// Package compact filters command output for token optimization.
// Compaction runs in three stages: pre-exec command rewrites (rewrite.go),
// native structured renderers (native*.go), and .bsh-defined filters with
// declarative ops (script_filter.go, ops.go).
package compact

import (
	"regexp"
	"sort"
	"strings"
)

// Filter transforms raw command output into a compact form.
type Filter func(stdout, stderr string, exitCode int) string

// Registry resolves output compaction for executed commands: native
// renderers first, then script filters loaded from .bsh extensions.
type Registry struct {
	scriptFilters []ScriptFilterDef
	sorted        bool
}

// NewRegistry creates an empty filter registry.
func NewRegistry() *Registry {
	return &Registry{}
}

// Compact runs the full output cascade for cmdline: a native renderer when
// one matches and recognizes the output, else the best script filter, else
// raw. Returns the compacted text and the name of what handled it ("" = raw).
func (r *Registry) Compact(cmdline, stdout, stderr string, exitCode int) (string, string) {
	words := Tokenize(cmdline)

	if nr := lookupNative(words); nr != nil {
		if out, ok := nr.render(stdout, stderr, exitCode); ok {
			return out, nr.name
		}
	}

	if f, name := r.lookupScript(words); f != nil {
		return f(stdout, stderr, exitCode), name
	}

	return "", ""
}

// Lookup returns the script filter for a command, or nil if none exists.
// Kept for direct filter access; Compact is the main entry point.
func (r *Registry) Lookup(cmdName string, args []string) Filter {
	cmdline := cmdName
	if len(args) > 0 {
		cmdline += " " + strings.Join(args, " ")
	}
	f, _ := r.lookupScript(Tokenize(cmdline))
	return f
}

// lookupScript finds the longest-pattern script filter whose Match
// prefix-matches the command words.
func (r *Registry) lookupScript(words []Word) (Filter, string) {
	if !r.sorted {
		sort.SliceStable(r.scriptFilters, func(i, j int) bool {
			return len(strings.Fields(r.scriptFilters[i].Match)) > len(strings.Fields(r.scriptFilters[j].Match))
		})
		r.sorted = true
	}
	for _, sf := range r.scriptFilters {
		if _, ok := MatchPrefix(words, sf.Match); ok {
			return ScriptFilter(sf), sf.Name
		}
	}
	return nil, ""
}

// --- Utilities available to script filters ---

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// StripANSI removes ANSI escape codes from text.
func StripANSI(s string) string {
	return ansiRe.ReplaceAllString(s, "")
}
