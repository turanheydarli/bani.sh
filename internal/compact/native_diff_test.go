package compact

import (
	"strings"
	"testing"
)

const sampleDiff = `diff --git a/internal/foo.go b/internal/foo.go
index abc1234..def5678 100644
--- a/internal/foo.go
+++ b/internal/foo.go
@@ -10,7 +10,8 @@ func foo() {
 	unchanged := 1
-	old := 2
+	new := 2
+	added := 3
 	alsoUnchanged := 4
diff --git a/README.md b/README.md
index 111..222 100644
--- a/README.md
+++ b/README.md
@@ -1,3 +1,2 @@
-gone line
 kept context
`

func TestRenderGitDiff(t *testing.T) {
	got, ok := renderGitDiff(sampleDiff, "", 0)
	if !ok {
		t.Fatal("should render unified diff")
	}
	for _, want := range []string{
		"=== internal/foo.go +2 -1",
		"=== README.md +0 -1",
		"@ 10",
		"+	new := 2",
		"-gone line",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{"index ", "unchanged := 1", "kept context", "diff --git"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("should have dropped %q", unwanted)
		}
	}
}

func TestRenderGitDiffFallsThroughOnNonDiff(t *testing.T) {
	if _, ok := renderGitDiff("just some text\nno diff here", "", 0); ok {
		t.Error("non-diff output must fall through")
	}
	if _, ok := renderGitDiff("", "", 0); ok {
		t.Error("empty output must fall through")
	}
}

func TestRegistryCompactCascade(t *testing.T) {
	r := NewRegistry()
	// Native renderer wins for git diff.
	out, handler := r.Compact("git diff", sampleDiff, "", 0)
	if handler != "git-diff" {
		t.Fatalf("handler = %q", handler)
	}
	if !strings.Contains(out, "=== internal/foo.go") {
		t.Errorf("unexpected output: %q", out)
	}

	// Script filter applies when no native renderer matches.
	r.RegisterScriptFilters([]ScriptFilterDef{
		{Name: "caps", Match: "lister", Ops: FilterOps{MaxLines: 1, Overflow: "+{n}"}},
	})
	out, handler = r.Compact("lister -a", "a\nb\nc", "", 0)
	if handler != "caps" || out != "a\n+2" {
		t.Errorf("handler=%q out=%q", handler, out)
	}

	// Nothing matches: raw signalled by empty handler.
	if _, handler = r.Compact("unknowncmd", "x", "", 0); handler != "" {
		t.Errorf("expected raw fallthrough, got %q", handler)
	}
}

func TestRegistryLongestPatternWins(t *testing.T) {
	r := NewRegistry()
	r.RegisterScriptFilters([]ScriptFilterDef{
		{Name: "short", Match: "git", Ops: FilterOps{MaxLines: 1}},
		{Name: "long", Match: "git remote", Ops: FilterOps{MaxLines: 2}},
	})
	_, handler := r.Compact("git remote -v", "a\nb\nc", "", 0)
	if handler != "long" {
		t.Errorf("longest pattern should win, got %q", handler)
	}
}
