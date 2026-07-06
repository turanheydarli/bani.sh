package compact

import (
	"strings"
	"testing"
)

// TestRewriteRespectsEnvKillSwitch guards the BANISH_NO_REWRITE hard-off.
// CI comparators, pipe-sensitive scripts, and agents debugging surprising
// output rely on this being an unambiguous escape hatch.
func TestRewriteRespectsEnvKillSwitch(t *testing.T) {
	t.Setenv(EnvNoRewrite, "1")
	rw := NewRewriter(testRules())
	got, rule := rw.Rewrite("git status")
	if got != "git status" || rule != "" {
		t.Errorf("kill switch ignored: got %q rule %q", got, rule)
	}
}

// TestRewriteEnvKillSwitchEmptyValue asserts empty means off. Users expect
// unset≡"" to leave rewrites enabled; matches conventions elsewhere in the
// binary.
func TestRewriteEnvKillSwitchEmptyValue(t *testing.T) {
	t.Setenv(EnvNoRewrite, "")
	rw := NewRewriter(testRules())
	got, rule := rw.Rewrite("git status")
	if rule == "" || got == "git status" {
		t.Errorf("empty env value should not disable rewrite: got %q rule %q", got, rule)
	}
}

func TestAnnounceRewrite(t *testing.T) {
	cases := []struct {
		orig, exec string
		want       string
	}{
		{"git status", "git status --porcelain -b", "[banish → git status --porcelain -b]"},
		{"git status", "git status", ""}, // no rewrite fired
		{"", "anything", ""},             // guard: empty original
		{"kubectl get pods", "", ""},     // guard: empty executed
	}
	for _, c := range cases {
		got := AnnounceRewrite(c.orig, c.exec)
		if got != c.want {
			t.Errorf("AnnounceRewrite(%q,%q) = %q, want %q", c.orig, c.exec, got, c.want)
		}
	}
}

// TestCascadeWithJSONRewrite asserts the full cascade: rewrite adds -o json,
// native renderer handles the JSON output, script filter never runs.
func TestCascadeWithJSONRewrite(t *testing.T) {
	rw := NewRewriter([]RewriteRule{
		{Name: "kubectl-get-json", Match: "kubectl get", Unless: []string{"-o"}, To: "kubectl get -o json"},
	})
	executed, rule := rw.Rewrite("kubectl get pods")
	if rule != "kubectl-get-json" || !strings.Contains(executed, "-o json") {
		t.Fatalf("rewrite did not fire: executed=%q rule=%q", executed, rule)
	}

	r := NewRegistry()
	out, handler := r.Compact(executed, kubePodListJSON, "", 0)
	if handler != "kubectl-get-json" {
		t.Fatalf("native renderer did not claim output; handler=%q", handler)
	}
	// All rows in kubePodListJSON share the default namespace, so ns/
	// prefixes collapse -- verify plain "api-7f8b" survives.
	if !strings.Contains(out, "READY") || !strings.Contains(out, "api-7f8b") {
		t.Errorf("compact output missing kubectl table:\n%s", out)
	}
}
