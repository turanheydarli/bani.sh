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
	InputToks  int64
	OutputToks int64
	RawToks    int64  // tokens before compaction
	SavedToks  int64  // tokens saved by compaction
	BashEquiv  int64  // estimated tokens if done in bash
	Savings    int64  // BashEquiv - InputToks
	Mode       string // "bsh" or "bash"
	HintShown  bool
}

// Stats holds aggregate statistics.
type Stats struct {
	Commands       int        `json:"commands"`
	InputTokens    int64      `json:"input_tokens"`
	OutputTokens   int64      `json:"output_tokens"`
	RawTokens      int64      `json:"raw_tokens"`
	SavedTokens    int64      `json:"saved_tokens"`
	SavingsPct     float64    `json:"savings_pct"`
	TopVerbs       []VerbStat `json:"top_verbs"`
}

// VerbStat tracks per-verb usage.
type VerbStat struct {
	Name     string  `json:"name"`
	Count    int     `json:"count"`
	Saved    int64   `json:"saved"`
	AvgPct   float64 `json:"avg_pct"`
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
		s.InputTokens += e.InputToks
		s.OutputTokens += e.OutputToks
		s.RawTokens += e.RawToks
		s.SavedTokens += e.SavedToks

		base := normalizeCmd(e.Command)
		vs, ok := verbMap[base]
		if !ok {
			vs = &VerbStat{Name: base}
			verbMap[base] = vs
		}
		vs.Count++
		vs.Saved += e.SavedToks
	}

	if s.RawTokens > 0 {
		s.SavingsPct = float64(s.SavedTokens) / float64(s.RawTokens) * 100
	}

	for _, vs := range verbMap {
		if vs.Count > 0 && vs.Saved > 0 {
			vs.AvgPct = float64(vs.Saved) / float64(vs.Count)
		}
		s.TopVerbs = append(s.TopVerbs, *vs)
	}
	sort.Slice(s.TopVerbs, func(i, j int) bool {
		return s.TopVerbs[i].Saved > s.TopVerbs[j].Saved
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
	return fmt.Sprintf("commands:%d saved:%d tokens (%.1f%%)", s.Commands, s.SavedTokens, s.SavingsPct)
}

// EstimateTokens estimates token count from a string (~4 chars per token).
func EstimateTokens(s string) int64 {
	n := int64(len(s)) / 4
	if n == 0 && len(s) > 0 {
		n = 1
	}
	return n
}
