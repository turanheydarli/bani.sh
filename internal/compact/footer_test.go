package compact

import (
	"strings"
	"testing"
)

func TestRenderFooter(t *testing.T) {
	fi := FooterInfo{
		Groups: []DroppedGroup{
			{Filter: "go-test.drop", Lines: 32},
			{Filter: "go-test.max-lines", Lines: 15},
		},
		RawLines:  120,
		RawBytes:  9000,
		EstTokens: 612,
		Recover:   "a1b2c3d4",
	}
	got := RenderFooter(fi)
	want := "--- banish: dropped 47 lines ---\n" +
		"  groups:\n" +
		"    - filter: go-test.drop  lines: 32\n" +
		"    - filter: go-test.max-lines  lines: 15\n" +
		"  recover: banish raw a1b2c3d4 (costs ~612 tokens, only if needed)"
	if got != want {
		t.Errorf("footer:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderFooterTraceOmitsGroups(t *testing.T) {
	fi := FooterInfo{
		Groups:    []DroppedGroup{{Filter: "x.drop", Lines: 50}},
		RawLines:  100,
		RawBytes:  9000,
		EstTokens: 500,
		Recover:   "a1b2c3d4",
		Trace:     true,
	}
	got := RenderFooter(fi)
	if strings.Contains(got, "groups:") {
		t.Errorf("trace footer must omit groups block:\n%s", got)
	}
	if !strings.Contains(got, "recover: banish raw a1b2c3d4 (costs ~500 tokens, only if needed)") {
		t.Errorf("trace footer must keep the recover line:\n%s", got)
	}
}

func TestRenderFooterNoRecoverLine(t *testing.T) {
	fi := FooterInfo{
		Groups:    []DroppedGroup{{Filter: "x.drop", Lines: 50}},
		RawLines:  100,
		RawBytes:  9000,
		EstTokens: 500,
	}
	if got := RenderFooter(fi); strings.Contains(got, "recover:") {
		t.Errorf("footer without hash must omit recover line:\n%s", got)
	}
}

func TestFooterSuppression(t *testing.T) {
	base := FooterInfo{
		Groups:    []DroppedGroup{{Filter: "x.drop", Lines: 50}},
		RawLines:  100,
		RawBytes:  9000,
		EstTokens: 500,
	}
	cases := []struct {
		name   string
		mutate func(*FooterInfo)
	}{
		{"nothing dropped", func(fi *FooterInfo) { fi.Groups = nil }},
		{"small raw line count", func(fi *FooterInfo) { fi.RawLines = footerMinRawLines - 1 }},
		{"small raw byte size", func(fi *FooterInfo) { fi.RawBytes = footerMinRawBytes - 1 }},
		{"trivial savings", func(fi *FooterInfo) { fi.EstTokens = footerMinSavedTokens - 1 }},
	}
	if base.Suppressed() {
		t.Fatal("base info must not be suppressed")
	}
	for _, c := range cases {
		fi := base
		c.mutate(&fi)
		if !fi.Suppressed() {
			t.Errorf("%s: expected suppression", c.name)
		}
		if RenderFooter(fi) != "" {
			t.Errorf("%s: suppressed footer must render empty", c.name)
		}
	}
}

// TestFooterTokenBudget guards the <= 50 token target for a typical footer.
func TestFooterTokenBudget(t *testing.T) {
	fi := FooterInfo{
		Groups: []DroppedGroup{
			{Filter: "npm-install.pipe", Lines: 32},
			{Filter: "npm-install.max-lines", Lines: 15},
		},
		RawLines:  200,
		RawBytes:  20000,
		EstTokens: 1250,
		Recover:   "a1b2c3d4",
	}
	footer := RenderFooter(fi)
	if toks := len(footer) / 4; toks > 50 {
		t.Errorf("footer costs ~%d tokens, budget is 50:\n%s", toks, footer)
	}
}

func TestCountLines(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"a", 1},
		{"a\n", 1},
		{"a\nb", 2},
		{"a\nb\n", 2},
	}
	for _, c := range cases {
		if got := CountLines(c.in); got != c.want {
			t.Errorf("CountLines(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}
