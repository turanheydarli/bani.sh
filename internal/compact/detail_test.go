package compact

import (
	"strings"
	"testing"
)

func TestApplyDetailDropAccounting(t *testing.T) {
	ops := FilterOps{Drop: "^noise"}
	in := "keep 1\nnoise a\nnoise b\nkeep 2\nnoise c"
	out, groups := ops.ApplyDetail(in, "f", false)
	if out != "keep 1\nkeep 2" {
		t.Errorf("output = %q", out)
	}
	if len(groups) != 1 || groups[0].Filter != "f.drop" || groups[0].Lines != 3 {
		t.Errorf("groups = %+v, want [{f.drop 3}]", groups)
	}
}

func TestApplyDetailTraceAnnotations(t *testing.T) {
	ops := FilterOps{Drop: "^noise"}
	in := "keep 1\nnoise a\nnoise b\nkeep 2\nnoise c"
	out, groups := ops.ApplyDetail(in, "f", true)
	want := "keep 1\n[banish: dropped 2 lines via f.drop]\nkeep 2\n[banish: dropped 1 lines via f.drop]"
	if out != want {
		t.Errorf("trace output:\n%s\nwant:\n%s", out, want)
	}
	// Accounting is identical with or without trace.
	if TotalDropped(groups) != 3 {
		t.Errorf("dropped = %d, want 3", TotalDropped(groups))
	}
}

func TestApplyDetailMaxLinesAccounting(t *testing.T) {
	ops := FilterOps{MaxLines: 2}
	out, groups := ops.ApplyDetail("a\nb\nc\nd", "f", false)
	if !strings.Contains(out, "+2 more") {
		t.Errorf("overflow marker missing: %q", out)
	}
	if len(groups) != 1 || groups[0].Filter != "f.max-lines" || groups[0].Lines != 2 {
		t.Errorf("groups = %+v, want [{f.max-lines 2}]", groups)
	}
}

func TestApplyDetailPerGroupAccounting(t *testing.T) {
	ops := FilterOps{GroupBy: `^(\w+):`, PerGroup: 1}
	out, groups := ops.ApplyDetail("a:1\na:2\na:3\nb:1", "f", false)
	if !strings.Contains(out, "+2 more") {
		t.Errorf("overflow marker missing: %q", out)
	}
	if len(groups) != 1 || groups[0].Filter != "f.per-group" || groups[0].Lines != 2 {
		t.Errorf("groups = %+v, want [{f.per-group 2}]", groups)
	}
}

func TestScriptFilterDetailPipeAccounting(t *testing.T) {
	def := ScriptFilterDef{
		Name:    "npm-install",
		Match:   "npm install",
		Compact: "grep -v '^npm warn'",
	}
	stdout := "added 5 packages\nnpm warn deprecated a\nnpm warn deprecated b\ndone"
	out, groups := ScriptFilterDetail(def, stdout, "", 0, false)
	if strings.Contains(out, "npm warn") {
		t.Errorf("warns not dropped: %q", out)
	}
	if len(groups) != 1 || groups[0].Filter != "npm-install.pipe" || groups[0].Lines != 2 {
		t.Errorf("groups = %+v, want [{npm-install.pipe 2}]", groups)
	}
}

func TestScriptFilterDetailPipeTraceAnnotation(t *testing.T) {
	def := ScriptFilterDef{
		Name:    "npm-install",
		Match:   "npm install",
		Compact: "grep -v '^npm warn'",
	}
	stdout := "added 5 packages\nnpm warn deprecated a\ndone"
	out, _ := ScriptFilterDetail(def, stdout, "", 0, true)
	if !strings.Contains(out, "[banish: dropped 1 lines via npm-install.pipe]") {
		t.Errorf("pipe trace annotation missing:\n%s", out)
	}
}

func TestCompactDetailScriptFilter(t *testing.T) {
	r := NewRegistry()
	r.RegisterScriptFilters([]ScriptFilterDef{{
		Name:  "go-test",
		Match: "go test",
		Ops:   FilterOps{Drop: `^ok\s`},
	}})
	d := r.CompactDetail("go test ./...", "ok pkg1\nok pkg2\nFAIL pkg3", "", 1, false)
	if d.Handler != "go-test" {
		t.Fatalf("handler = %q", d.Handler)
	}
	if TotalDropped(d.Groups) != 2 {
		t.Errorf("dropped = %d, want 2", TotalDropped(d.Groups))
	}
}

func TestCompactDetailPassthrough(t *testing.T) {
	r := NewRegistry()
	d := r.CompactDetail("some-unknown-tool", "hello", "", 0, false)
	if d.Handler != "" || d.Text != "" || d.Groups != nil {
		t.Errorf("passthrough must be empty: %+v", d)
	}
}

func TestCompactDetailNativeRendererDiff(t *testing.T) {
	r := NewRegistry()
	// git diff native renderer condenses hunks; drop accounting is the
	// raw-vs-rendered line diff attributed to the renderer name.
	stdout := "diff --git a/f.txt b/f.txt\nindex 000..111 100644\n--- a/f.txt\n+++ b/f.txt\n@@ -1,3 +1,3 @@\n context\n-old line\n+new line\n context\n"
	d := r.CompactDetail("git diff", stdout, "", 0, false)
	if d.Handler != "git-diff" {
		t.Skipf("native renderer did not engage (handler=%q)", d.Handler)
	}
	rendered := CountLines(d.Text)
	raw := CountLines(stdout)
	if want := raw - rendered; want > 0 && TotalDropped(d.Groups) != want {
		t.Errorf("native diff accounting = %d, want %d", TotalDropped(d.Groups), want)
	}
}

func TestCompactUnchangedByDetail(t *testing.T) {
	// The legacy Compact API must return exactly what CompactDetail's Text is.
	r := NewRegistry()
	r.RegisterScriptFilters([]ScriptFilterDef{{
		Name:  "go-test",
		Match: "go test",
		Ops:   FilterOps{Drop: `^ok\s`},
	}})
	text, handler := r.Compact("go test ./...", "ok pkg1\nFAIL pkg2", "", 1)
	d := r.CompactDetail("go test ./...", "ok pkg1\nFAIL pkg2", "", 1, false)
	if text != d.Text || handler != d.Handler {
		t.Errorf("Compact (%q, %q) != CompactDetail (%q, %q)", text, handler, d.Text, d.Handler)
	}
}
