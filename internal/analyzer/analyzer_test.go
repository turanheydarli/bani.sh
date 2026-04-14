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
		BashEquiv: 8,
		Savings:   6,
		Mode:      "bsh",
	})
	a.Track(Entry{
		Timestamp: time.Now(),
		Command:   "ls",
		InputToks: 3,
		BashEquiv: 10,
		Savings:   7,
		Mode:      "bsh",
	})
	a.Track(Entry{
		Timestamp: time.Now(),
		Command:   "fetch",
		InputToks: 4,
		BashEquiv: 12,
		Savings:   8,
		Mode:      "bsh",
	})

	s := a.SessionStats()

	if s.Commands != 3 {
		t.Errorf("commands = %d, want 3", s.Commands)
	}
	if s.TotalSaved != 21 {
		t.Errorf("total saved = %d, want 21", s.TotalSaved)
	}
	if len(s.TopVerbs) != 2 {
		t.Fatalf("top verbs = %d, want 2", len(s.TopVerbs))
	}
	// ls should be first (2 uses vs 1)
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

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"ls", 1},
		{"find /var/log -name *.log", 6},
		{"ls /var/log ext:log", 4},
	}
	for _, tt := range tests {
		got := EstimateTokens(tt.input)
		if got != tt.want {
			t.Errorf("EstimateTokens(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestFormatStats(t *testing.T) {
	s := &Stats{Commands: 5, TotalSaved: 42}
	got := FormatStats(s)
	want := "commands:5 saved:42 tokens"
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
	if s.TotalSaved != 0 {
		t.Errorf("saved = %d, want 0", s.TotalSaved)
	}
}
