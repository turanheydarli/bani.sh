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

// TestRenderGitDiffLockfileSummary asserts a lockfile diff surfaces as the
// semantic package-level summary rather than the raw hunk lines.
func TestRenderGitDiffLockfileSummary(t *testing.T) {
	diff := `diff --git a/package-lock.json b/package-lock.json
index abc..def 100644
--- a/package-lock.json
+++ b/package-lock.json
@@ -100,7 +100,7 @@
     "node_modules/express": {
-      "version": "4.19.0",
+      "version": "4.19.2",
       "resolved": "https://..."
`
	out, ok := renderGitDiff(diff, "", 0)
	if !ok {
		t.Fatal("renderGitDiff ok=false")
	}
	for _, want := range []string{
		"=== package-lock.json",
		"semantic diff via banish",
		"express 4.19.0 → 4.19.2",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	// Raw version lines must not survive; that is the entire point.
	if strings.Contains(out, `"version": "4.19.0"`) {
		t.Errorf("raw version line leaked through:\n%s", out)
	}
}

// TestRenderGitDiffLockfileEnvOverride asserts BANISH_LOCKFILE_FULL=1 falls
// back to the raw diff renderer (parser opts out, body stays as-is).
func TestRenderGitDiffLockfileEnvOverride(t *testing.T) {
	t.Setenv("BANISH_LOCKFILE_FULL", "1")
	diff := `diff --git a/package-lock.json b/package-lock.json
--- a/package-lock.json
+++ b/package-lock.json
@@ -100,7 +100,7 @@
-      "version": "4.19.0",
+      "version": "4.19.2",
`
	out, ok := renderGitDiff(diff, "", 0)
	if !ok {
		t.Fatal("renderGitDiff ok=false")
	}
	if strings.Contains(out, "semantic diff via banish") {
		t.Errorf("BANISH_LOCKFILE_FULL should suppress summary:\n%s", out)
	}
	if !strings.Contains(out, `"version": "4.19.2"`) {
		t.Errorf("raw diff should pass through:\n%s", out)
	}
}

// TestRenderGitDiffMixedFiles asserts lockfile compaction is per-file: a
// non-lockfile in the same diff still gets normal condensed rendering.
func TestRenderGitDiffMixedFiles(t *testing.T) {
	diff := `diff --git a/src/foo.go b/src/foo.go
--- a/src/foo.go
+++ b/src/foo.go
@@ -1,3 +1,3 @@
-old := 1
+new := 1
diff --git a/go.sum b/go.sum
--- a/go.sum
+++ b/go.sum
@@ -1,2 +1,2 @@
-github.com/foo/bar v1.0.0 h1:aaa=
-github.com/foo/bar v1.0.0/go.mod h1:bbb=
+github.com/foo/bar v1.1.0 h1:ccc=
+github.com/foo/bar v1.1.0/go.mod h1:ddd=
`
	out, ok := renderGitDiff(diff, "", 0)
	if !ok {
		t.Fatal("renderGitDiff ok=false")
	}
	if !strings.Contains(out, "=== src/foo.go") || !strings.Contains(out, "-old := 1") {
		t.Errorf("non-lockfile file lost normal rendering:\n%s", out)
	}
	if !strings.Contains(out, "github.com/foo/bar v1.0.0 → v1.1.0") {
		t.Errorf("go.sum did not get semantic summary:\n%s", out)
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
