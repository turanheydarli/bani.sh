package rawcache

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func setHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

func TestStoreAndGetRoundTrip(t *testing.T) {
	setHome(t)
	stdout := "line one\nline two\n"
	stderr := "warning: something\n"

	hash, err := Store(stdout, stderr)
	if err != nil {
		t.Fatal(err)
	}
	if len(hash) != 8 {
		t.Fatalf("hash %q, want 8 hex chars", hash)
	}
	if hash != Hash(stdout, stderr) {
		t.Errorf("Store hash %q != Hash() %q", hash, Hash(stdout, stderr))
	}

	data, err := Get(hash)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != stdout+stderr {
		t.Errorf("Get = %q, want exact raw bytes %q", data, stdout+stderr)
	}
}

func TestFilePermissions(t *testing.T) {
	home := setHome(t)
	hash, err := Store("secret output", "")
	if err != nil {
		t.Fatal(err)
	}
	d := filepath.Join(home, ".banish", "cache", "raw")
	di, err := os.Stat(d)
	if err != nil {
		t.Fatal(err)
	}
	if perm := di.Mode().Perm(); perm != 0o700 {
		t.Errorf("cache dir perm %o, want 0700", perm)
	}
	fi, err := os.Stat(filepath.Join(d, hash))
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("cache file perm %o, want 0600", perm)
	}
}

func TestGetUnknownHash(t *testing.T) {
	setHome(t)
	if _, err := Get("deadbeef"); err == nil {
		t.Error("expected error for unknown hash")
	}
	if _, err := Get("../../etc"); err == nil || !strings.Contains(err.Error(), "invalid hash") {
		t.Errorf("path-like hash must be rejected, got %v", err)
	}
	if _, err := Get("DEADBEEF"); err == nil {
		t.Error("uppercase hash must be rejected")
	}
}

func TestTTLExpiry(t *testing.T) {
	home := setHome(t)
	hash, err := Store("old output", "")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".banish", "cache", "raw", hash)
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	if _, err := Get(hash); err == nil {
		t.Error("expected expiry error for entry older than TTL")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("expired entry should be removed on Get")
	}
}

func TestPruneExpired(t *testing.T) {
	home := setHome(t)
	hash, err := Store("stale", "")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".banish", "cache", "raw", hash)
	old := time.Now().Add(-2 * time.Hour)
	os.Chtimes(path, old, old)

	// Any Store prunes expired entries.
	if _, err := Store("fresh", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("expired entry should be pruned by Store")
	}
}

func TestSizeEviction(t *testing.T) {
	home := setHome(t)
	// Cap the cache at 1 MB via config.
	banishDir := filepath.Join(home, ".banish")
	os.MkdirAll(banishDir, 0o755)
	os.WriteFile(filepath.Join(banishDir, "config.json"),
		[]byte(`{"cache": {"max_mb": 1}}`), 0o644)

	// 30 entries of ~100 KB = ~3 MB, well over the 1 MB cap.
	big := strings.Repeat("x", 100*1024)
	var hashes []string
	for i := 0; i < 30; i++ {
		h, err := Store(big+string(rune('a'+i)), "")
		if err != nil {
			t.Fatal(err)
		}
		hashes = append(hashes, h)
		// Space out mtimes so eviction order is deterministic.
		p := filepath.Join(banishDir, "cache", "raw", h)
		mt := time.Now().Add(time.Duration(i-30) * time.Minute)
		os.Chtimes(p, mt, mt)
	}
	// One more store triggers the final prune.
	Store("trigger", "")

	entries, err := os.ReadDir(filepath.Join(banishDir, "cache", "raw"))
	if err != nil {
		t.Fatal(err)
	}
	var total int64
	for _, e := range entries {
		fi, _ := e.Info()
		total += fi.Size()
	}
	// minEntries newest survive even though they exceed the cap.
	if len(entries) < minEntries {
		t.Errorf("eviction kept %d entries, want at least %d", len(entries), minEntries)
	}
	if len(entries) >= 31 {
		t.Errorf("eviction removed nothing: %d entries", len(entries))
	}
	// The oldest entry must be gone, the newest kept.
	if _, err := os.Stat(filepath.Join(banishDir, "cache", "raw", hashes[0])); !os.IsNotExist(err) {
		t.Error("oldest entry should have been evicted")
	}
	if _, err := Get(hashes[len(hashes)-1]); err != nil {
		t.Errorf("newest entry should survive eviction: %v", err)
	}
}

func TestClear(t *testing.T) {
	home := setHome(t)
	hash, err := Store("data", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := Clear(); err != nil {
		t.Fatal(err)
	}
	if _, err := Get(hash); err == nil {
		t.Error("expected error after Clear")
	}
	entries, _ := os.ReadDir(filepath.Join(home, ".banish", "cache", "raw"))
	if len(entries) != 0 {
		t.Errorf("cache not empty after Clear: %d entries", len(entries))
	}
}

func TestDisabledViaConfig(t *testing.T) {
	home := setHome(t)
	banishDir := filepath.Join(home, ".banish")
	os.MkdirAll(banishDir, 0o755)
	os.WriteFile(filepath.Join(banishDir, "config.json"),
		[]byte(`{"cache": {"raw": false}}`), 0o644)
	if Enabled() {
		t.Error("cache should be disabled by config")
	}

	os.WriteFile(filepath.Join(banishDir, "config.json"), []byte(`{}`), 0o644)
	if !Enabled() {
		t.Error("cache should default to enabled")
	}
}
