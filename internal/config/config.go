// Package config manages banish configuration: the ~/.banish/ directory
// layout and the optional ~/.banish/config.json file. The file is optional;
// every field has a working zero-value default so callers never need to
// distinguish "no file" from "empty file".
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Cache configures the raw output cache backing "banish raw".
type Cache struct {
	// Raw enables the raw output cache. Nil means enabled (the default);
	// {"cache": {"raw": false}} disables caching entirely.
	Raw *bool `json:"raw"`
	// TTLMinutes is how long cached raw outputs stay recoverable.
	// Zero means the default (60 minutes).
	TTLMinutes int `json:"ttl_minutes"`
	// MaxMB caps the total size of the raw cache directory.
	// Zero means the default (50 MB).
	MaxMB int `json:"max_mb"`
}

// RawEnabled reports whether the raw output cache is on.
func (c Cache) RawEnabled() bool {
	return c.Raw == nil || *c.Raw
}

// TTL returns the configured cache TTL in minutes, defaulted.
func (c Cache) TTL() int {
	if c.TTLMinutes > 0 {
		return c.TTLMinutes
	}
	return 60
}

// MaxBytes returns the configured cache size cap in bytes, defaulted.
func (c Cache) MaxBytes() int64 {
	if c.MaxMB > 0 {
		return int64(c.MaxMB) * 1024 * 1024
	}
	return 50 * 1024 * 1024
}

// Config is the parsed ~/.banish/config.json.
type Config struct {
	// Channel selects the release channel for update checks
	// ("stable" or "beta").
	Channel string `json:"channel"`
	// Cache configures the raw output cache.
	Cache Cache `json:"cache"`
	// Tokenizer selects the token counter: "heuristic" (default) or
	// "anthropic" (see internal/token/counter).
	Tokenizer string `json:"tokenizer"`
	// TokenizerModel is the model whose tokenizer the anthropic counter
	// queries. Empty means the counter package's default.
	TokenizerModel string `json:"tokenizer_model"`
}

// Load reads ~/.banish/config.json. A missing or unparseable file yields
// the zero Config -- configuration is always best-effort.
func Load() Config {
	var cfg Config
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return cfg
	}
	data, err := os.ReadFile(filepath.Join(home, ".banish", "config.json"))
	if err != nil {
		return cfg
	}
	if json.Unmarshal(data, &cfg) != nil {
		return Config{}
	}
	return cfg
}

// Dir returns the banish home directory (~/.banish), or "" when the user
// home cannot be resolved.
func Dir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".banish")
}
