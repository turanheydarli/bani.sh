package counter

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const (
	defaultBaseURL = "https://api.anthropic.com"
	// DefaultModel is the model whose tokenizer is used when the config does
	// not name one. Counting is model-specific; this matches the model the
	// gain command prices savings against.
	DefaultModel = "claude-opus-4-8"

	// cacheMaxEntries bounds the on-disk cache. Entries are evicted least
	// recently used once the cap is exceeded.
	cacheMaxEntries = 8192
)

// Anthropic counts tokens with Anthropic's count_tokens endpoint. Results
// are cached on disk keyed by content hash so repeated strings (bench
// fixtures, common command output) cost one request ever. Any failure -
// missing key, network, non-200 - falls back to CharHeuristic for that call.
type Anthropic struct {
	model   string
	apiKey  string
	baseURL string
	client  *http.Client

	mu    sync.Mutex
	cache map[string]*cacheEntry
	dirty bool
	path  string // cache file; empty disables persistence
}

type cacheEntry struct {
	Tokens int64 `json:"t"`
	Used   int64 `json:"u"` // unix seconds, for LRU eviction
}

// NewAnthropic builds a counter for the given model (empty = DefaultModel).
// It returns an error when ANTHROPIC_API_KEY is not set, so callers can fall
// back to the heuristic explicitly rather than paying a doomed request per
// count.
func NewAnthropic(model string) (*Anthropic, error) {
	key := os.Getenv("ANTHROPIC_API_KEY")
	if key == "" {
		return nil, fmt.Errorf("counter: ANTHROPIC_API_KEY is not set")
	}
	if model == "" {
		model = DefaultModel
	}
	a := &Anthropic{
		model:   model,
		apiKey:  key,
		baseURL: defaultBaseURL,
		client:  &http.Client{Timeout: 15 * time.Second},
		cache:   map[string]*cacheEntry{},
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		a.path = filepath.Join(home, ".banish", "tokcache.json")
		a.loadCache()
	}
	return a, nil
}

// Name implements Counter.
func (a *Anthropic) Name() string { return "anthropic tokenizer (" + a.model + ")" }

// Count implements Counter. Cache hits and successful endpoint responses are
// exact; on any error the char heuristic answers and exact is false.
func (a *Anthropic) Count(s string) (int64, bool) {
	if s == "" {
		return 0, true
	}
	sum := sha256.Sum256([]byte(a.model + "\x00" + s))
	key := hex.EncodeToString(sum[:])

	a.mu.Lock()
	if e, ok := a.cache[key]; ok {
		e.Used = time.Now().Unix()
		a.dirty = true
		a.mu.Unlock()
		return e.Tokens, true
	}
	a.mu.Unlock()

	n, err := a.countRemote(s)
	if err != nil {
		return CharHeuristic{}.Count(s)
	}

	a.mu.Lock()
	a.cache[key] = &cacheEntry{Tokens: n, Used: time.Now().Unix()}
	a.dirty = true
	a.evictLocked()
	a.mu.Unlock()
	return n, true
}

// countRemote posts to /v1/messages/count_tokens and returns input_tokens.
// The endpoint counts the whole request, so the constant per-message
// envelope overhead (a few tokens) is included; for savings percentages it
// cancels out between raw and compacted counts.
func (a *Anthropic) countRemote(s string) (int64, error) {
	body, err := json.Marshal(map[string]any{
		"model": a.model,
		"messages": []map[string]string{
			{"role": "user", "content": s},
		},
	})
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequest(http.MethodPost, a.baseURL+"/v1/messages/count_tokens", bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("x-api-key", a.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("counter: count_tokens returned %s", resp.Status)
	}
	var out struct {
		InputTokens int64 `json:"input_tokens"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, err
	}
	return out.InputTokens, nil
}

// Flush persists the cache when it changed. Call once per process, after
// counting is done; losing the cache only costs re-counting.
func (a *Anthropic) Flush() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.dirty || a.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(a.path), 0755); err != nil {
		return err
	}
	data, err := json.Marshal(a.cache)
	if err != nil {
		return err
	}
	if err := os.WriteFile(a.path, data, 0644); err != nil {
		return err
	}
	a.dirty = false
	return nil
}

func (a *Anthropic) loadCache() {
	data, err := os.ReadFile(a.path)
	if err != nil {
		return
	}
	// A corrupt cache is discarded, not fatal.
	_ = json.Unmarshal(data, &a.cache)
	if a.cache == nil {
		a.cache = map[string]*cacheEntry{}
	}
}

// evictLocked drops the least recently used entries once the cache exceeds
// its cap. Caller holds a.mu.
func (a *Anthropic) evictLocked() {
	if len(a.cache) <= cacheMaxEntries {
		return
	}
	type kv struct {
		key  string
		used int64
	}
	all := make([]kv, 0, len(a.cache))
	for k, e := range a.cache {
		all = append(all, kv{k, e.Used})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].used < all[j].used })
	for _, e := range all[:len(all)-cacheMaxEntries] {
		delete(a.cache, e.key)
	}
}
