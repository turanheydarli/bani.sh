// Package analyzer tracks command execution patterns, token costs, and
// frequency for the adaptive middleware layer.
package analyzer

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Entry records a single command execution.
type Entry struct {
	Timestamp  time.Time
	Command    string
	InputToks  int
	OutputToks int
	BashEquiv  int // estimated tokens if done in bash
	Savings    int // BashEquiv - InputToks
	Mode       string // "bsh" or "bash"
	HintShown  bool
}

// Stats holds aggregate statistics.
type Stats struct {
	Commands       int        `json:"commands"`
	InputSaved     int        `json:"input_saved"`
	OutputSaved    int        `json:"output_saved"`
	TotalSaved     int        `json:"total_saved"`
	TopVerbs       []VerbStat `json:"top_verbs"`
}

// VerbStat tracks per-verb usage.
type VerbStat struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
	Saved int    `json:"saved"`
}

// Analyzer tracks command execution for token accounting.
type Analyzer struct {
	mu      sync.Mutex
	entries []Entry
	freq    map[string]int
}

// New creates an analyzer.
func New() *Analyzer {
	return &Analyzer{
		freq: make(map[string]int),
	}
}

// Track records a command execution.
func (a *Analyzer) Track(e Entry) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.entries = append(a.entries, e)
	a.freq[e.Command]++

	// Also track by normalized base command name.
	base := normalizeCmd(e.Command)
	if base != "" && base != e.Command {
		a.freq[base]++
	}
}

// normalizeCmd extracts the base command name, stripping paths.
func normalizeCmd(cmd string) string {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return ""
	}
	return filepath.Base(parts[0])
}

// SessionStats returns stats for the current session.
func (a *Analyzer) SessionStats() *Stats {
	a.mu.Lock()
	defer a.mu.Unlock()

	s := &Stats{}
	verbMap := make(map[string]*VerbStat)

	for _, e := range a.entries {
		s.Commands++
		s.InputSaved += e.Savings
		s.TotalSaved += e.Savings

		vs, ok := verbMap[e.Command]
		if !ok {
			vs = &VerbStat{Name: e.Command}
			verbMap[e.Command] = vs
		}
		vs.Count++
		vs.Saved += e.Savings
	}

	for _, vs := range verbMap {
		s.TopVerbs = append(s.TopVerbs, *vs)
	}
	sort.Slice(s.TopVerbs, func(i, j int) bool {
		return s.TopVerbs[i].Count > s.TopVerbs[j].Count
	})
	if len(s.TopVerbs) > 10 {
		s.TopVerbs = s.TopVerbs[:10]
	}

	return s
}

// Frequency returns how many times a command has been used.
func (a *Analyzer) Frequency(cmd string) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.freq[cmd]
}

// FormatStats returns a compact string summary.
func FormatStats(s *Stats) string {
	return fmt.Sprintf("commands:%d saved:%d tokens", s.Commands, s.TotalSaved)
}

// EstimateTokens estimates token count from a string (~4 chars per token).
func EstimateTokens(s string) int {
	n := len(s) / 4
	if n == 0 && len(s) > 0 {
		n = 1
	}
	return n
}
