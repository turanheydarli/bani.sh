// Package compact filters command output for token optimization.
// All filters are defined as .bsh extensions with !filter directives.
// This package provides the script runner and basic utilities.
package compact

import (
	"regexp"
	"strings"
)

// Filter transforms raw command output into a compact form.
type Filter func(stdout, stderr string, exitCode int) string

// Registry holds script-based filters loaded from .bsh extensions.
type Registry struct {
	scriptFilters []ScriptFilterDef
}

// NewRegistry creates an empty filter registry.
func NewRegistry() *Registry {
	return &Registry{}
}

// Lookup returns the filter for a command, or nil if none exists.
func (r *Registry) Lookup(cmdName string, args []string) Filter {
	fullCmd := cmdName
	if len(args) > 0 {
		fullCmd = cmdName + " " + strings.Join(args, " ")
	}

	for _, sf := range r.scriptFilters {
		if strings.Contains(fullCmd, sf.Match) {
			return ScriptFilter(sf)
		}
	}
	return nil
}

// --- Utilities available to script filters ---

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// StripANSI removes ANSI escape codes from text.
func StripANSI(s string) string {
	return ansiRe.ReplaceAllString(s, "")
}
