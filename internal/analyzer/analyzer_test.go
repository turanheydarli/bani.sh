package analyzer

import (
	"testing"
	"time"
)

func TestTrackAndStats(t *testing.T) {
	a := New()

	a.Track(Entry{
		Timestamp: time.Now(),
		Command:   "ls",
		InputToks: 2,
		RawToks:   8,
		SavedToks: 6,
		Mode:      "bsh",
	})
	a.Track(Entry{
		Timestamp: time.Now(),
		Command:   "ls",
		InputToks: 3,
		RawToks:   10,
		SavedToks: 7,
		Mode:      "bsh",
	})
	a.Track(Entry{
		Timestamp: time.Now(),
		Command:   "fetch",
		InputToks: 4,
		RawToks:   12,
		SavedToks: 8,
		Mode:      "bsh",
	})

	s := a.SessionStats()

	if s.Commands != 3 {
		t.Errorf("commands = %d, want 3", s.Commands)
	}
	if s.SavedTokens != 21 {
		t.Errorf("saved tokens = %d, want 21", s.SavedTokens)
	}
	if s.RawTokens != 30 {
		t.Errorf("raw tokens = %d, want 30", s.RawTokens)
	}
	if len(s.TopVerbs) != 2 {
		t.Fatalf("top verbs = %d, want 2", len(s.TopVerbs))
	}
	// ls should be first (more savings)
	if s.TopVerbs[0].Name != "ls" {
		t.Errorf("top verb = %s, want ls", s.TopVerbs[0].Name)
	}
	if s.TopVerbs[0].Count != 2 {
		t.Errorf("ls count = %d, want 2", s.TopVerbs[0].Count)
	}
}

func TestFrequency(t *testing.T) {
	a := New()
	a.Track(Entry{Command: "ls"})
	a.Track(Entry{Command: "ls"})
	a.Track(Entry{Command: "fetch"})

	if a.Frequency("ls") != 2 {
		t.Errorf("ls freq = %d, want 2", a.Frequency("ls"))
	}
	if a.Frequency("fetch") != 1 {
		t.Errorf("fetch freq = %d, want 1", a.Frequency("fetch"))
	}
	if a.Frequency("unknown") != 0 {
		t.Errorf("unknown freq = %d, want 0", a.Frequency("unknown"))
	}
}

func TestEstimateTokensCharBased(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"", 0},
		{"ls", 1},
		{"find /var/log -name *.log", 6},
		{"ls /var/log ext:log", 4},
	}
	for _, tt := range tests {
		got := EstimateTokensCharBased(tt.input)
		if got != tt.want {
			t.Errorf("EstimateTokensCharBased(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestFormatStats(t *testing.T) {
	s := &Stats{Commands: 5, SavedTokens: 42, SavingsPct: 33.3}
	got := FormatStats(s)
	want := "commands:5 saved:42 tokens (33.3%)"
	if got != want {
		t.Errorf("FormatStats = %q, want %q", got, want)
	}
}

func TestEmptyStats(t *testing.T) {
	a := New()
	s := a.SessionStats()
	if s.Commands != 0 {
		t.Errorf("commands = %d, want 0", s.Commands)
	}
	if s.SavedTokens != 0 {
		t.Errorf("saved = %d, want 0", s.SavedTokens)
	}
}

// Loading the on-disk aggregates and saving again must not re-merge them, or
// token totals double on every run.
func TestSaveDoesNotDoubleCountLoaded(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	a := New()
	a.Track(Entry{Timestamp: time.Now(), Command: "git status", RawToks: 100, SavedToks: 60})
	a.SaveFrequency()

	// Load the aggregate and save again without tracking anything new.
	b := New()
	b.LoadFrequency()
	b.SaveFrequency()

	// Totals must be unchanged, not doubled.
	c := New()
	c.LoadFrequency()
	stats := c.SessionStats()
	if stats.RawTokens != 100 {
		t.Fatalf("RawTokens = %d, want 100 (loaded entry double-counted on save)", stats.RawTokens)
	}
	if stats.SavedTokens != 60 {
		t.Fatalf("SavedTokens = %d, want 60", stats.SavedTokens)
	}
}
