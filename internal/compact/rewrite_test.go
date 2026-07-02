package compact

import (
	"strings"
	"testing"
)

func testRules() []RewriteRule {
	return []RewriteRule{
		{
			Name:   "git-status",
			Match:  "git status",
			Unless: []string{"--porcelain", "-s", "--short", "--format"},
			To:     "git status --porcelain -b",
		},
		{
			Name:   "git-log",
			Match:  "git log",
			Unless: []string{"--oneline", "-n", "--format", "-p"},
			To:     "git log --oneline -20",
		},
	}
}

func TestRewriteBasic(t *testing.T) {
	rw := NewRewriter(testRules())
	got, rule := rw.Rewrite("git status")
	if got != "git status --porcelain -b" || rule != "git-status" {
		t.Errorf("got %q rule %q", got, rule)
	}
}

func TestRewriteAppendsExtraArgs(t *testing.T) {
	rw := NewRewriter(testRules())
	got, _ := rw.Rewrite("git status src/")
	if got != "git status --porcelain -b src/" {
		t.Errorf("got %q", got)
	}
}

func TestRewritePreservesQuoting(t *testing.T) {
	rw := NewRewriter(testRules())
	got, _ := rw.Rewrite(`git status "my dir/with space"`)
	if got != `git status --porcelain -b "my dir/with space"` {
		t.Errorf("quoting lost: %q", got)
	}
}

func TestRewriteUnlessFlag(t *testing.T) {
	rw := NewRewriter(testRules())
	for _, cmd := range []string{
		"git status --porcelain",
		"git status -s",
		"git status --format=%s",
		"git log --oneline -10",
		"git log -n 5",
		"git log -5", // numeric shorthand counts as -n
	} {
		if got, rule := rw.Rewrite(cmd); rule != "" {
			t.Errorf("%q should not rewrite, got %q", cmd, got)
		}
	}
}

func TestRewriteSkipsNonSimpleCommands(t *testing.T) {
	rw := NewRewriter(testRules())
	for _, cmd := range []string{
		"git status | head -5",
		"git status && echo done",
		"git status > /tmp/out",
		"git status; ls",
		"echo $(git status)",
		"GIT_DIR=x git status", // env prefix: basename is GIT_DIR=x, no match
	} {
		got, rule := rw.Rewrite(cmd)
		if rule != "" {
			t.Errorf("%q should not rewrite", cmd)
		}
		if got != cmd {
			t.Errorf("%q must pass through untouched, got %q", cmd, got)
		}
	}
}

func TestRewriteNoFalseSubstringMatch(t *testing.T) {
	rw := NewRewriter(testRules())
	if _, rule := rw.Rewrite("echo git status"); rule != "" {
		t.Error("substring inside another command must not match")
	}
}

func TestRewriteMatchesPathPrefixedBinary(t *testing.T) {
	rw := NewRewriter(testRules())
	got, rule := rw.Rewrite("/usr/bin/git status")
	if rule != "git-status" {
		t.Fatalf("basename match failed: %q", got)
	}
	if !strings.HasSuffix(got, "--porcelain -b") {
		t.Errorf("got %q", got)
	}
}

func TestTokenizeOffsets(t *testing.T) {
	words := Tokenize(`git commit -m "a message"`)
	if len(words) != 4 {
		t.Fatalf("want 4 words, got %d: %v", len(words), words)
	}
	if words[3].Text != "a message" {
		t.Errorf("quoted word = %q", words[3].Text)
	}
	if words[3].Start != 14 || words[3].End != 25 {
		t.Errorf("offsets = %d,%d", words[3].Start, words[3].End)
	}
}

func TestIsSimpleCommand(t *testing.T) {
	simple := []string{"git status", `echo "a | b"`, `grep -rn 'x;y' src`}
	for _, s := range simple {
		if !IsSimpleCommand(s) {
			t.Errorf("%q should be simple", s)
		}
	}
	complex := []string{"a | b", "a && b", "a > f", "a $(b)", `echo "$(x)"`, "a 'unterminated"}
	for _, s := range complex {
		if IsSimpleCommand(s) {
			t.Errorf("%q should not be simple", s)
		}
	}
}
