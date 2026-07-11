package counter

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config is the tokenizer section of ~/.banish/config.json. The default is
// the char heuristic so nothing depends on the network or an API key unless
// the user opts in.
//
//	{"tokenizer": "anthropic", "tokenizer_model": "claude-opus-4-8"}
type Config struct {
	Tokenizer      string `json:"tokenizer"`       // "heuristic" (default) or "anthropic"
	TokenizerModel string `json:"tokenizer_model"` // defaults to DefaultModel
}

// LoadConfig reads ~/.banish/config.json. A missing or unparseable file
// yields the defaults.
func LoadConfig() Config {
	cfg := Config{Tokenizer: "heuristic", TokenizerModel: DefaultModel}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return cfg
	}
	data, err := os.ReadFile(filepath.Join(home, ".banish", "config.json"))
	if err != nil {
		return cfg
	}
	var file Config
	if json.Unmarshal(data, &file) != nil {
		return cfg
	}
	if file.Tokenizer != "" {
		cfg.Tokenizer = file.Tokenizer
	}
	if file.TokenizerModel != "" {
		cfg.TokenizerModel = file.TokenizerModel
	}
	return cfg
}

// FromConfig returns the configured counter. It falls back to CharHeuristic
// when the anthropic tokenizer is requested but ANTHROPIC_API_KEY is absent,
// so opting in never breaks offline or keyless runs.
func FromConfig() Counter {
	cfg := LoadConfig()
	if cfg.Tokenizer == "anthropic" {
		if a, err := NewAnthropic(cfg.TokenizerModel); err == nil {
			return a
		}
	}
	return CharHeuristic{}
}
