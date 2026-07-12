// Package rawcache stores the raw output of compacted commands so agents
// can recover dropped content with "banish raw <hash>" instead of
// re-running the command uncompacted. Entries live under
// ~/.banish/cache/raw/, are keyed by a content hash, expire after a TTL,
// and the directory is size-capped. Outputs may contain secrets, so files
// are created 0600 in a 0700 directory and never inside a repository;
// {"cache": {"raw": false}} in ~/.banish/config.json disables the cache.
package rawcache

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"

	"go.banish.sh/banish/internal/config"
)

// minEntries is how many recent entries survive size eviction, so the last
// commands stay recoverable even when individual outputs are large.
const minEntries = 20

var hashRe = regexp.MustCompile(`^[0-9a-f]{8}$`)

// Hash returns the content address for a command's raw output: the first
// 8 hex characters of the SHA-256 over stdout and stderr. It is a pure
// function so measurement paths (bench) can compute the hash a real run
// would produce without touching the cache.
func Hash(stdout, stderr string) string {
	h := sha256.New()
	h.Write([]byte(stdout))
	h.Write([]byte{0})
	h.Write([]byte(stderr))
	return hex.EncodeToString(h.Sum(nil))[:8]
}

// Enabled reports whether the raw cache is on: config allows it and the
// user home resolves.
func Enabled() bool {
	return config.Load().Cache.RawEnabled() && dir() != ""
}

// dir returns the cache directory, or "" when the user home is unknown.
func dir() string {
	base := config.Dir()
	if base == "" {
		return ""
	}
	return filepath.Join(base, "cache", "raw")
}

// Store writes the raw stdout+stderr of a command to the cache and returns
// its hash. The entry file holds stdout followed by stderr, byte for byte.
// Store also prunes expired and over-cap entries.
func Store(stdout, stderr string) (string, error) {
	d := dir()
	if d == "" {
		return "", fmt.Errorf("rawcache: cannot resolve user home")
	}
	if err := os.MkdirAll(d, 0o700); err != nil {
		return "", err
	}
	hash := Hash(stdout, stderr)
	path := filepath.Join(d, hash)
	if err := os.WriteFile(path, []byte(stdout+stderr), 0o600); err != nil {
		return "", err
	}
	// Refresh mtime for repeated identical outputs so TTL restarts.
	now := time.Now()
	os.Chtimes(path, now, now)
	prune(d)
	return hash, nil
}

// Get returns the cached raw output for hash. Expired or unknown hashes
// return an error.
func Get(hash string) ([]byte, error) {
	if !hashRe.MatchString(hash) {
		return nil, fmt.Errorf("invalid hash %q (want 8 hex characters)", hash)
	}
	d := dir()
	if d == "" {
		return nil, fmt.Errorf("cannot resolve user home")
	}
	path := filepath.Join(d, hash)
	fi, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("no cached output for %s (unknown or expired)", hash)
	}
	if time.Since(fi.ModTime()) > ttl() {
		os.Remove(path)
		return nil, fmt.Errorf("no cached output for %s (unknown or expired)", hash)
	}
	return os.ReadFile(path)
}

// Clear removes every cached entry.
func Clear() error {
	d := dir()
	if d == "" {
		return nil
	}
	entries, err := os.ReadDir(d)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		os.Remove(filepath.Join(d, e.Name()))
	}
	return nil
}

func ttl() time.Duration {
	return time.Duration(config.Load().Cache.TTL()) * time.Minute
}

// prune drops expired entries, then evicts oldest-first while the directory
// exceeds the size cap -- always keeping the newest minEntries so recent
// commands stay recoverable.
func prune(d string) {
	entries, err := os.ReadDir(d)
	if err != nil {
		return
	}
	type item struct {
		path string
		mod  time.Time
		size int64
	}
	maxAge := ttl()
	maxBytes := config.Load().Cache.MaxBytes()

	var items []item
	var total int64
	now := time.Now()
	for _, e := range entries {
		fi, err := e.Info()
		if err != nil {
			continue
		}
		p := filepath.Join(d, e.Name())
		if now.Sub(fi.ModTime()) > maxAge {
			os.Remove(p)
			continue
		}
		items = append(items, item{p, fi.ModTime(), fi.Size()})
		total += fi.Size()
	}

	sort.Slice(items, func(i, j int) bool { return items[i].mod.Before(items[j].mod) })
	remaining := len(items)
	for _, it := range items {
		if total <= maxBytes || remaining <= minEntries {
			break
		}
		os.Remove(it.path)
		total -= it.size
		remaining--
	}
}
