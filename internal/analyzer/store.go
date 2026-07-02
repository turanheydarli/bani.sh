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
	TokenCost int64     `json:"tok"` // legacy combined input+output, kept for old files
	InToks    int64     `json:"in,omitempty"`
	OutToks   int64     `json:"out,omitempty"`
	RawToks   int64     `json:"raw,omitempty"`   // tokens before compaction
	SavedToks int64     `json:"saved,omitempty"` // tokens saved (negative = overhead)
	Rewrites  int64     `json:"rw,omitempty"`    // commands rewritten pre-exec
	LastSeen  time.Time `json:"ts"`
}

// sanitize clamps corrupted token values (overflow from previous versions).
// SavedToks may legitimately be negative: compaction overhead is real data.
func (r *freqRecord) sanitize() {
	clamp := func(v *int64) {
		if *v < 0 || *v > 1000000000 {
			*v = 0
		}
	}
	clamp(&r.TokenCost)
	clamp(&r.InToks)
	clamp(&r.OutToks)
	clamp(&r.RawToks)
	clamp(&r.Rewrites)
	if r.SavedToks < -1000000000 || r.SavedToks > 1000000000 {
		r.SavedToks = 0
	}
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
		r.sanitize()
		a.freq[r.Command] += r.Count
		// Also track base command name
		if parts := strings.Fields(r.Command); len(parts) > 1 {
			a.freq[parts[0]] += r.Count
		}
		in, out := r.InToks, r.OutToks
		if in == 0 && out == 0 && r.TokenCost > 0 {
			// Legacy record: only the combined cost was stored. Attribute it
			// to output (command strings are tiny next to their output).
			out = r.TokenCost
		}
		a.entries = append(a.entries, Entry{
			Timestamp:  r.LastSeen,
			Command:    r.Command,
			InputToks:  in,
			OutputToks: out,
			RawToks:    r.RawToks,
			SavedToks:  r.SavedToks,
			Rewrites:   r.Rewrites,
			loaded:     true,
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

	// Build map from existing, sanitizing corrupted values
	recs := make(map[string]*freqRecord)
	for i := range existing {
		r := &existing[i]
		r.sanitize()
		recs[r.Command] = r
	}

	// Merge current session entries
	a.mu.Lock()
	for _, e := range a.entries {
		if e.Timestamp.IsZero() {
			continue
		}
		// Entries loaded from disk are already in recs; merging them again
		// double-counts their tokens on every run.
		if e.loaded {
			continue
		}
		r, ok := recs[e.Command]
		if !ok {
			r = &freqRecord{Command: e.Command}
			recs[e.Command] = r
		}
		r.Count++
		r.TokenCost += e.InputToks + e.OutputToks
		r.InToks += e.InputToks
		r.OutToks += e.OutputToks
		r.RawToks += e.RawToks
		r.SavedToks += e.SavedToks
		r.Rewrites += e.Rewrites
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
