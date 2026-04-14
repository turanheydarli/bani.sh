package hints

import (
	"testing"
)

func TestSuggestFind(t *testing.T) {
	h := New()

	hint := h.Suggest("find", []string{"/var/log", "-name", "*.log"})
	if hint == nil {
		t.Fatal("expected hint for find")
	}
	if hint.Shorter != "ls /var/log ext:log" {
		t.Errorf("shorter = %q, want %q", hint.Shorter, "ls /var/log ext:log")
	}
	if hint.Saved <= 0 {
		t.Errorf("saved = %d, want > 0", hint.Saved)
	}
}

func TestSuggestCurl(t *testing.T) {
	h := New()

	hint := h.Suggest("curl", []string{"-s", "https://api.example.com"})
	if hint == nil {
		t.Fatal("expected hint for curl")
	}
	if hint.Shorter != "fetch https://api.example.com" {
		t.Errorf("shorter = %q, want %q", hint.Shorter, "fetch https://api.example.com")
	}
}

func TestSuggestCat(t *testing.T) {
	h := New()

	hint := h.Suggest("cat", []string{"file.txt"})
	if hint == nil {
		t.Fatal("expected hint for cat")
	}
	if hint.Shorter != "read file.txt" {
		t.Errorf("shorter = %q, want %q", hint.Shorter, "read file.txt")
	}
}

func TestSuggestGzip(t *testing.T) {
	h := New()

	hint := h.Suggest("gzip", []string{"file.log"})
	if hint == nil {
		t.Fatal("expected hint for gzip")
	}
	if hint.Shorter != "gz file.log" {
		t.Errorf("shorter = %q, want %q", hint.Shorter, "gz file.log")
	}
}

func TestSuggestWc(t *testing.T) {
	h := New()

	hint := h.Suggest("wc", []string{"-l"})
	if hint == nil {
		t.Fatal("expected hint for wc")
	}
	if hint.Shorter != "count" {
		t.Errorf("shorter = %q, want count", hint.Shorter)
	}
}

func TestSuggestGrep(t *testing.T) {
	h := New()

	hint := h.Suggest("grep", []string{"error", "app.log"})
	if hint == nil {
		t.Fatal("expected hint for grep")
	}
	if hint.Shorter != "read app.log ? error" {
		t.Errorf("shorter = %q, want %q", hint.Shorter, "read app.log ? error")
	}
}

func TestNoSuggestionForUnknown(t *testing.T) {
	h := New()

	hint := h.Suggest("docker", []string{"ps"})
	if hint != nil {
		t.Errorf("expected nil hint for docker, got %+v", hint)
	}
}

func TestSuggestionEvenWhenSameLength(t *testing.T) {
	h := New()

	// rm -> rm, same verb, but builtin gives structured output.
	hint := h.Suggest("rm", []string{"x"})
	if hint == nil {
		t.Fatal("expected hint for rm (structured output benefit)")
	}
	if hint.Saved != 0 {
		t.Errorf("saved = %d, want 0 (same input length)", hint.Saved)
	}
}

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"ls", 1},
		{"find /var/log -name *.log", 6},
		{"ls /var/log ext:log", 4},
	}

	for _, tt := range tests {
		got := estimateTokens(tt.input)
		if got != tt.want {
			t.Errorf("estimateTokens(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestAliases(t *testing.T) {
	h := New()

	hint := h.Suggest("wget", []string{"https://example.com"})
	if hint == nil {
		t.Fatal("expected hint for wget (alias of curl)")
	}

	hint = h.Suggest("rg", []string{"pattern", "file.txt"})
	if hint == nil {
		t.Fatal("expected hint for rg (alias of grep)")
	}
}
