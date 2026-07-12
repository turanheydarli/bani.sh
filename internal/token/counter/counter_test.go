package counter

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func TestCharHeuristic(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"", 0},
		{"a", 1},        // rounds up to 1 for non-empty input
		{"abcd", 1},     // 4 chars
		{"abcdefgh", 2}, // 8 chars
	}
	for _, c := range cases {
		got, exact := CharHeuristic{}.Count(c.in)
		if got != c.want {
			t.Errorf("Count(%q) = %d, want %d", c.in, got, c.want)
		}
		if exact {
			t.Errorf("Count(%q) claims to be exact; the heuristic never is", c.in)
		}
	}
}

func newTestAnthropic(t *testing.T, url string) *Anthropic {
	t.Helper()
	return &Anthropic{
		model:   DefaultModel,
		apiKey:  "test-key",
		baseURL: url,
		client:  http.DefaultClient,
		cache:   map[string]*cacheEntry{},
		path:    filepath.Join(t.TempDir(), "tokcache.json"),
	}
}

func TestAnthropicCountAndCache(t *testing.T) {
	var calls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/v1/messages/count_tokens" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "test-key" {
			t.Errorf("missing x-api-key header")
		}
		if r.Header.Get("anthropic-version") == "" {
			t.Errorf("missing anthropic-version header")
		}
		var body struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body.Model != DefaultModel || len(body.Messages) != 1 {
			t.Errorf("unexpected request body: %+v", body)
		}
		fmt.Fprintf(w, `{"input_tokens": %d}`, len(body.Messages[0].Content))
	}))
	defer srv.Close()

	a := newTestAnthropic(t, srv.URL)

	n, exact := a.Count("hello world")
	if n != int64(len("hello world")) || !exact {
		t.Fatalf("Count = %d exact=%v, want %d exact=true", n, exact, len("hello world"))
	}
	// Second count of the same string must come from the cache.
	if n2, _ := a.Count("hello world"); n2 != n {
		t.Fatalf("cached Count = %d, want %d", n2, n)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("endpoint called %d times, want 1 (cache miss only)", got)
	}

	// Cache survives a flush/reload cycle.
	if err := a.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	b := newTestAnthropic(t, srv.URL)
	b.path = a.path
	b.loadCache()
	if n3, exact3 := b.Count("hello world"); n3 != n || !exact3 {
		t.Fatalf("reloaded Count = %d exact=%v, want %d exact=true", n3, exact3, n)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("endpoint called %d times after reload, want still 1", got)
	}
}

func TestAnthropicFallsBackOnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"type":"error"}`, http.StatusInternalServerError)
	}))
	defer srv.Close()

	a := newTestAnthropic(t, srv.URL)
	in := "abcdefgh"
	n, exact := a.Count(in)
	want, _ := CharHeuristic{}.Count(in)
	if n != want || exact {
		t.Fatalf("Count = %d exact=%v, want heuristic %d exact=false", n, exact, want)
	}
}

func TestAnthropicEmptyString(t *testing.T) {
	a := newTestAnthropic(t, "http://127.0.0.1:1") // must not be contacted
	if n, exact := a.Count(""); n != 0 || !exact {
		t.Fatalf("Count(\"\") = %d exact=%v, want 0 exact=true", n, exact)
	}
}

func TestNewAnthropicRequiresKey(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	if _, err := NewAnthropic(""); err == nil {
		t.Fatal("NewAnthropic succeeded without ANTHROPIC_API_KEY")
	}
}

func TestEvict(t *testing.T) {
	a := newTestAnthropic(t, "http://127.0.0.1:1")
	for i := 0; i < cacheMaxEntries+10; i++ {
		a.cache[fmt.Sprintf("k%d", i)] = &cacheEntry{Tokens: 1, Used: int64(i)}
	}
	a.evictLocked()
	if len(a.cache) != cacheMaxEntries {
		t.Fatalf("cache has %d entries after evict, want %d", len(a.cache), cacheMaxEntries)
	}
	if _, ok := a.cache["k0"]; ok {
		t.Fatal("oldest entry survived eviction")
	}
}

func TestLoadConfigDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := LoadConfig()
	if cfg.Tokenizer != "heuristic" || cfg.TokenizerModel != DefaultModel {
		t.Fatalf("defaults = %+v", cfg)
	}

	dir := filepath.Join(home, ".banish")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	content := `{"tokenizer": "anthropic", "tokenizer_model": "claude-sonnet-5"}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	cfg = LoadConfig()
	if cfg.Tokenizer != "anthropic" || cfg.TokenizerModel != "claude-sonnet-5" {
		t.Fatalf("loaded config = %+v", cfg)
	}
}

func TestFromConfigFallsBackWithoutKey(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ANTHROPIC_API_KEY", "")
	dir := filepath.Join(home, ".banish")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"tokenizer":"anthropic"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if _, ok := FromConfig().(CharHeuristic); !ok {
		t.Fatal("FromConfig did not fall back to CharHeuristic without an API key")
	}
}
