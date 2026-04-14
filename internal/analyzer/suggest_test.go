package analyzer

import (
	"testing"
)

func TestSuggestBelowThreshold(t *testing.T) {
	a := New()
	for i := 0; i < 3; i++ {
		a.Track(Entry{Command: "staticcheck ./...", InputToks: 5})
	}

	builtins := map[string]bool{"ls": true, "echo": true}
	s := a.SuggestExtension("staticcheck ./...", builtins)
	if s != nil {
		t.Error("expected nil below threshold")
	}
}

func TestSuggestAtThreshold(t *testing.T) {
	a := New()
	for i := 0; i < 6; i++ {
		a.Track(Entry{Command: "staticcheck ./...", InputToks: 5, OutputToks: 40})
	}

	builtins := map[string]bool{"ls": true}
	s := a.SuggestExtension("staticcheck ./...", builtins)
	if s == nil {
		t.Fatal("expected suggestion at threshold")
	}
	if s.Command != "staticcheck" {
		t.Errorf("command = %q, want staticcheck", s.Command)
	}
	if s.Frequency != 6 {
		t.Errorf("frequency = %d, want 6", s.Frequency)
	}
	if !s.Confirm {
		t.Error("confirm should always be true")
	}
	if len(s.Guide.Rules) == 0 {
		t.Error("guide should have rules")
	}
}

func TestSuggestSkipsBuiltins(t *testing.T) {
	a := New()
	for i := 0; i < 10; i++ {
		a.Track(Entry{Command: "ls /tmp", InputToks: 3})
	}

	builtins := map[string]bool{"ls": true}
	s := a.SuggestExtension("ls /tmp", builtins)
	if s != nil {
		t.Error("should not suggest extension for builtin")
	}
}

func TestSuggestArgsSeen(t *testing.T) {
	a := New()
	a.Track(Entry{Command: "staticcheck ./...", InputToks: 5, OutputToks: 40})
	a.Track(Entry{Command: "staticcheck ./cmd/...", InputToks: 5, OutputToks: 40})
	a.Track(Entry{Command: "staticcheck ./...", InputToks: 5, OutputToks: 40})
	a.Track(Entry{Command: "staticcheck -checks=all ./...", InputToks: 7, OutputToks: 40})
	a.Track(Entry{Command: "staticcheck ./...", InputToks: 5, OutputToks: 40})
	a.Track(Entry{Command: "staticcheck ./internal/...", InputToks: 6, OutputToks: 40})

	builtins := map[string]bool{}
	s := a.SuggestExtension("staticcheck ./...", builtins)
	if s == nil {
		t.Fatal("expected suggestion")
	}

	if len(s.ArgsSeen) < 3 {
		t.Errorf("args_seen = %d, want >= 3 distinct patterns", len(s.ArgsSeen))
	}
}

func TestSuggestGuideTemplate(t *testing.T) {
	a := New()
	for i := 0; i < 5; i++ {
		a.Track(Entry{Command: "mycheck ./...", InputToks: 4, OutputToks: 40})
	}

	s := a.SuggestExtension("mycheck ./...", map[string]bool{})
	if s == nil {
		t.Fatal("expected suggestion")
	}

	if s.Guide.Template == "" {
		t.Error("template is empty")
	}
	if s.Guide.Example == "" {
		t.Error("example is empty")
	}
	if s.ExtDir != "~/.banish/ext/" {
		t.Errorf("ext_dir = %q, want ~/.banish/ext/", s.ExtDir)
	}
}
