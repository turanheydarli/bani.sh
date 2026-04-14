package analyzer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// freqRecord is what we persist to disk.
type freqRecord struct {
	Command   string    `json:"cmd"`
	Count     int       `json:"n"`
	TokenCost int       `json:"tok"`
	LastSeen  time.Time `json:"ts"`
}

// storePath returns the path to the frequency store.
func storePath() string {
	home, _ := os.UserHomeDir()
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".banish", "freq.json")
}

// LoadFrequency loads persisted frequency data from disk.
func (a *Analyzer) LoadFrequency() {
	path := storePath()
	if path == "" {
		return
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return
	}

	var records []freqRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	// Prune records older than 90 days
	cutoff := time.Now().AddDate(0, 0, -90)
	for _, r := range records {
		if r.LastSeen.Before(cutoff) {
			continue
		}
		a.freq[r.Command] += r.Count
		// Also track base command name
		if parts := strings.Fields(r.Command); len(parts) > 1 {
			a.freq[parts[0]] += r.Count
		}
		a.entries = append(a.entries, Entry{
			Timestamp: r.LastSeen,
			Command:   r.Command,
			InputToks: r.TokenCost / 2,
			OutputToks: r.TokenCost / 2,
		})
	}
}

// SaveFrequency persists current frequency data to disk.
func (a *Analyzer) SaveFrequency() {
	path := storePath()
	if path == "" {
		return
	}

	// Ensure directory exists
	os.MkdirAll(filepath.Dir(path), 0755)

	// Load existing records first to merge
	var existing []freqRecord
	if data, err := os.ReadFile(path); err == nil {
		json.Unmarshal(data, &existing)
	}

	// Build map from existing
	recs := make(map[string]*freqRecord)
	for i := range existing {
		recs[existing[i].Command] = &existing[i]
	}

	// Merge current session entries
	a.mu.Lock()
	for _, e := range a.entries {
		if e.Timestamp.IsZero() {
			continue
		}
		r, ok := recs[e.Command]
		if !ok {
			r = &freqRecord{Command: e.Command}
			recs[e.Command] = r
		}
		r.Count++
		r.TokenCost += e.InputToks + e.OutputToks
		if e.Timestamp.After(r.LastSeen) {
			r.LastSeen = e.Timestamp
		}
	}
	a.mu.Unlock()

	// Prune old records
	cutoff := time.Now().AddDate(0, 0, -90)
	var out []freqRecord
	for _, r := range recs {
		if r.LastSeen.After(cutoff) {
			out = append(out, *r)
		}
	}

	data, err := json.Marshal(out)
	if err != nil {
		return
	}
	os.WriteFile(path, data, 0644)
}
