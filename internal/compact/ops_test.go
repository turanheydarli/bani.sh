package compact

import (
	"strings"
	"testing"
)

func TestOpsDropKeep(t *testing.T) {
	ops := FilterOps{Drop: "^noise", Keep: "keep"}
	got := ops.Apply("keep me\nnoise keep\nother line\nkeep too")
	want := "keep me\nkeep too"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestOpsMaxLinesOverflow(t *testing.T) {
	ops := FilterOps{MaxLines: 2, Overflow: "+{n} more entries"}
	got := ops.Apply("a\nb\nc\nd\ne")
	want := "a\nb\n+3 more entries"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestOpsMaxLineLen(t *testing.T) {
	ops := FilterOps{MaxLineLen: 10}
	got := ops.Apply("short\n" + strings.Repeat("x", 30))
	lines := strings.Split(got, "\n")
	if lines[0] != "short" {
		t.Errorf("short line changed: %q", lines[0])
	}
	if len(lines[1]) != 10 || !strings.HasSuffix(lines[1], "...") {
		t.Errorf("clamp failed: %q", lines[1])
	}
}

func TestOpsGroupBy(t *testing.T) {
	ops := FilterOps{GroupBy: `^([^:]+):\d+:`, PerGroup: 2, Overflow: "+{n} more"}
	input := "a.go:1:x\na.go:2:x\na.go:3:x\na.go:4:x\nb.go:1:y\nplain line"
	got := ops.Apply(input)
	want := "a.go:1:x\na.go:2:x\n+2 more\nb.go:1:y\nplain line"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestOpsGroupByInterleaved(t *testing.T) {
	ops := FilterOps{GroupBy: `^(\w+):`, PerGroup: 1, Overflow: "+{n}"}
	got := ops.Apply("b:1\nb:2\na:1\na:2\na:3\nb:3")
	// b truncates first (at its 2nd line), then a; counts must match groups.
	want := "b:1\n+2\na:1\n+2"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestOpsSub(t *testing.T) {
	ops := FilterOps{Sub: []SubRule{{Pattern: `^commit (\w{7})\w+`, Replace: "$1"}}}
	got := ops.Apply("commit abcdef1234567890\nother")
	want := "abcdef1\nother"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestOpsTally(t *testing.T) {
	ops := FilterOps{Tally: []TallyRule{{Pattern: `^ok\s`, Template: "== {n} ok"}}, Drop: `^ok\s`}
	got := ops.Apply("ok  pkg1 0.1s\nok  pkg2 0.2s\nFAIL pkg3")
	want := "FAIL pkg3\n== 2 ok"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestOpsTallyZeroSuppressed(t *testing.T) {
	ops := FilterOps{Tally: []TallyRule{{Pattern: `^ok\s`, Template: "== {n} ok"}}, MaxLines: 10}
	got := ops.Apply("FAIL pkg")
	if got != "FAIL pkg" {
		t.Errorf("zero tally must not append: %q", got)
	}
}

func TestOpsInvalidRegexIsNoop(t *testing.T) {
	ops := FilterOps{Drop: "([bad", MaxLines: 10}
	got := ops.Apply("a\nb")
	if got != "a\nb" {
		t.Errorf("invalid regex must not eat output: %q", got)
	}
}
