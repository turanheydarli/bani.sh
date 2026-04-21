package compact

import (
	"testing"
)

func TestStripANSI(t *testing.T) {
	input := "\x1b[32mhello\x1b[0m world"
	got := StripANSI(input)
	if got != "hello world" {
		t.Errorf("StripANSI = %q, want %q", got, "hello world")
	}
}

func TestRegistryLookupEmpty(t *testing.T) {
	r := NewRegistry()
	if f := r.Lookup("git", []string{"status"}); f != nil {
		t.Error("empty registry should return nil")
	}
}

func TestRegistryWithScriptFilter(t *testing.T) {
	r := NewRegistry()
	r.RegisterScriptFilters([]ScriptFilterDef{
		{Name: "test", Match: "echo hello", Compact: "tr a-z A-Z"},
	})

	f := r.Lookup("echo", []string{"hello", "world"})
	if f == nil {
		t.Fatal("should find filter matching 'echo hello'")
	}

	got := f("hello world", "", 0)
	if got != "HELLO WORLD" {
		t.Errorf("filter output = %q, want %q", got, "HELLO WORLD")
	}
}

func TestRegistryNoMatch(t *testing.T) {
	r := NewRegistry()
	r.RegisterScriptFilters([]ScriptFilterDef{
		{Name: "test", Match: "echo hello", Compact: "tr a-z A-Z"},
	})

	if f := r.Lookup("git", []string{"status"}); f != nil {
		t.Error("should not match unrelated command")
	}
}

func TestScriptFilterFallback(t *testing.T) {
	// Script that fails should return raw output
	f := ScriptFilter(ScriptFilterDef{
		Name: "bad", Match: "test", Compact: "exit 1",
	})
	got := f("raw output", "", 0)
	if got != "raw output" {
		t.Errorf("failed script should return raw, got %q", got)
	}
}

func TestScriptFilterSkipsEmpty(t *testing.T) {
	r := NewRegistry()
	r.RegisterScriptFilters([]ScriptFilterDef{
		{Name: "no-match", Match: "", Compact: "tr a-z A-Z"},
		{Name: "no-compact", Match: "echo", Compact: ""},
	})
	// Both should be skipped (empty match or empty compact)
	if f := r.Lookup("echo", []string{"hi"}); f != nil {
		t.Error("should skip filters with empty match or compact")
	}
}
