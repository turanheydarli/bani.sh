package analyzer

import (
	"fmt"
	"strings"
)

// ExtensionSuggestion is returned when a bash command crosses the usage threshold.
type ExtensionSuggestion struct {
	Reason    string         `json:"reason"`
	Command   string         `json:"command"`
	ArgsSeen  []string       `json:"args_seen"`
	Frequency int            `json:"frequency"`
	TokenCost int            `json:"token_cost"`
	ExtDir    string         `json:"ext_dir"`
	Confirm   bool           `json:"confirm"`
	Guide     ExtensionGuide `json:"guide"`
}

// ExtensionGuide provides the template and rules for the agent to create an extension.
type ExtensionGuide struct {
	Template string   `json:"template"`
	Rules    []string `json:"rules"`
	Example  string   `json:"example"`
}

// Thresholds for triggering extension suggestions.
const (
	CrossSessionThreshold   = 10
	SingleSessionThreshold  = 5
	SingleSessionTokenCost  = 200
)

// SuggestExtension checks if a system-fallback command has crossed the threshold.
// Returns nil if below threshold or if the command is a known builtin.
func (a *Analyzer) SuggestExtension(cmd string, builtins map[string]bool) *ExtensionSuggestion {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Extract base command name
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return nil
	}
	baseName := parts[0]

	// Skip if it is a known builtin
	if builtins[baseName] {
		return nil
	}

	freq := a.freq[baseName]
	if freq < SingleSessionThreshold {
		return nil
	}

	// Calculate total token cost for this command
	var totalCost int
	var argsSeen []string
	seen := make(map[string]bool)
	for _, e := range a.entries {
		cmdParts := strings.Fields(e.Command)
		if len(cmdParts) > 0 && cmdParts[0] == baseName {
			totalCost += e.InputToks + e.OutputToks
			argStr := strings.Join(cmdParts[1:], " ")
			if argStr != "" && !seen[argStr] {
				seen[argStr] = true
				argsSeen = append(argsSeen, argStr)
			}
		}
	}

	if freq < CrossSessionThreshold && totalCost < SingleSessionTokenCost {
		return nil
	}

	return &ExtensionSuggestion{
		Reason:    fmt.Sprintf("%s called %d times (est. %d tokens total)", baseName, freq, totalCost),
		Command:   baseName,
		ArgsSeen:  argsSeen,
		Frequency: freq,
		TokenCost: totalCost,
		ExtDir:    "~/.banish/ext/",
		Confirm:   true,
		Guide: ExtensionGuide{
			Template: fmt.Sprintf("!extension %s v:1.0\n\n!verb <verb_name>\n!args <define based on args_seen>\n!expand exec %s $1\n!help \"<describe what this does>\"", baseName, baseName),
			Rules: []string{
				"Verb name: pick shortest unambiguous name for the command",
				"Args: make the most common argument positional, rest optional with ?",
				"expand: wrap the original command, use $1 for first arg",
				"help: one sentence, used in MCP tool descriptions",
				fmt.Sprintf("File: save as ~/.banish/ext/%s.bsh", baseName),
			},
			Example: "!extension lint v:1.0\n\n!verb lint\n!args path\n!expand exec golangci-lint run $1\n!help \"Run Go linter on packages\"",
		},
	}
}
