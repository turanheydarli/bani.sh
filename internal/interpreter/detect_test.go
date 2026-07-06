package interpreter

import "testing"

func TestDetectQuoteAwareFlags(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  InputMode
	}{
		// A dash-word inside a quoted target is part of the target, not a bash
		// flag - it must stay .bsh so the verb runs (regression for the MCP
		// quoting path where read "notes -draft.txt" was misrouted to the shell).
		{"quoted dash-word target", `read "/tmp/notes -draft.txt"`, ModeBSH},
		{"quoted url target", `fetch "https://host/x?a=1"`, ModeBSH},
		// Real bash flags outside quotes still route to the shell.
		{"real single-dash flag", `ls -la`, ModeBash},
		{"real double-dash flag", `git status --porcelain`, ModeBash},
		// A dashed flag after a quoted arg is still detected.
		{"flag after quoted arg", `grep "foo" -r`, ModeBash},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Detect(c.input); got != c.want {
				t.Errorf("Detect(%q) = %v, want %v", c.input, got, c.want)
			}
		})
	}
}

func TestQuoteAwareFields(t *testing.T) {
	got := quoteAwareFields(`read "/tmp/a -b.txt" x`)
	want := []string{"read", `"/tmp/a -b.txt"`, "x"}
	if len(got) != len(want) {
		t.Fatalf("fields = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("field %d = %q, want %q", i, got[i], want[i])
		}
	}
}
