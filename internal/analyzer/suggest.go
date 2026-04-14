package analyzer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
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

// SuggestionState tracks per-command suggestion lifecycle.
type SuggestionState struct {
	Command      string     `json:"cmd"`
	State        string     `json:"state"` // "pending", "suggested", "accepted", "dismissed"
	DismissCount int        `json:"dismiss_count"`
	LastShownAt  *time.Time `json:"last_shown,omitempty"`
	AcceptedAt   *time.Time `json:"accepted,omitempty"`
	NextShowAt   *time.Time `json:"next_show,omitempty"`
}

// Thresholds for triggering extension suggestions.
const (
	CrossSessionThreshold  = 10
	SingleSessionThreshold = 5
	SingleSessionTokenCost = 200
)

// SuggestExtension checks if a system-fallback command should get a suggestion.
// Returns nil if below threshold, already handled, in cooldown, or already accepted.
func (a *Analyzer) SuggestExtension(cmd string, builtins map[string]bool) *ExtensionSuggestion {
	a.mu.Lock()
	defer a.mu.Unlock()

	baseName := normalizeCommand(cmd)
	if baseName == "" {
		return nil
	}

	// Skip builtins
	if builtins[baseName] {
		return nil
	}

	// Skip if an extension file already exists for this command
	if extensionExists(baseName) {
		return nil
	}

	// Check suggestion state (cooldown, accepted, dismissed)
	states := loadSuggestionStates()
	state := states[baseName]

	if state != nil {
		// Already accepted -- never suggest again
		if state.State == "accepted" {
			return nil
		}

		// In cooldown -- check if cooldown expired
		if state.NextShowAt != nil && time.Now().Before(*state.NextShowAt) {
			return nil
		}
	}

	// Check frequency threshold
	freq := a.freq[baseName]
	if freq < SingleSessionThreshold {
		return nil
	}

	// Calculate total token cost
	var totalCost int
	var argsSeen []string
	seen := make(map[string]bool)
	for _, e := range a.entries {
		entryBase := normalizeCommand(e.Command)
		if entryBase == baseName {
			totalCost += e.InputToks + e.OutputToks
			cmdParts := strings.Fields(e.Command)
			if len(cmdParts) > 1 {
				argStr := strings.Join(cmdParts[1:], " ")
				if !seen[argStr] {
					seen[argStr] = true
					argsSeen = append(argsSeen, argStr)
				}
			}
		}
	}

	if freq < CrossSessionThreshold && totalCost < SingleSessionTokenCost {
		return nil
	}

	// Mark as suggested with cooldown for next time
	now := time.Now()
	if state == nil {
		state = &SuggestionState{Command: baseName}
	}
	state.State = "suggested"
	state.LastShownAt = &now

	// Exponential backoff: 1h * 2^dismissCount, capped at 168h (7 days)
	backoffHours := 1 << uint(state.DismissCount)
	if backoffHours > 168 {
		backoffHours = 168
	}
	nextShow := now.Add(time.Duration(backoffHours) * time.Hour)
	state.NextShowAt = &nextShow
	state.DismissCount++ // assume dismissed unless MarkAccepted is called

	states[baseName] = state
	saveSuggestionStates(states)

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

// MarkAccepted records that the user accepted a suggestion (never suggest again).
func MarkAccepted(cmd string) {
	baseName := normalizeCommand(cmd)
	states := loadSuggestionStates()
	now := time.Now()
	states[baseName] = &SuggestionState{
		Command:    baseName,
		State:      "accepted",
		AcceptedAt: &now,
	}
	saveSuggestionStates(states)
}

// normalizeCommand extracts the base command name, stripping paths and redirects.
func normalizeCommand(cmd string) string {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return ""
	}

	base := parts[0]

	// Strip absolute path: /usr/bin/git -> git
	base = filepath.Base(base)

	// Skip empty or shell builtins
	if base == "" || base == "." || base == "sh" || base == "bash" {
		return ""
	}

	return base
}

// extensionExists checks if a .bsh extension file exists for the command.
func extensionExists(baseName string) bool {
	home, _ := os.UserHomeDir()
	if home == "" {
		return false
	}

	extDir := filepath.Join(home, ".banish", "ext")
	entries, err := os.ReadDir(extDir)
	if err != nil {
		return false
	}

	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".bsh") {
			name := strings.TrimSuffix(e.Name(), ".bsh")
			if name == baseName {
				return true
			}
		}
	}

	return false
}

// suggestStatePath returns the path to the suggestion state file.
func suggestStatePath() string {
	home, _ := os.UserHomeDir()
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".banish", "suggestions.json")
}

func loadSuggestionStates() map[string]*SuggestionState {
	path := suggestStatePath()
	if path == "" {
		return make(map[string]*SuggestionState)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return make(map[string]*SuggestionState)
	}

	var states map[string]*SuggestionState
	if err := json.Unmarshal(data, &states); err != nil {
		return make(map[string]*SuggestionState)
	}
	return states
}

func saveSuggestionStates(states map[string]*SuggestionState) {
	path := suggestStatePath()
	if path == "" {
		return
	}

	os.MkdirAll(filepath.Dir(path), 0755)
	data, _ := json.Marshal(states)
	os.WriteFile(path, data, 0644)
}
