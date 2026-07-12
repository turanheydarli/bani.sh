package counter

import (
	"go.banish.sh/banish/internal/config"
)

// Config is the tokenizer section of ~/.banish/config.json. The default is
// the char heuristic so nothing depends on the network or an API key unless
// the user opts in.
//
//	{"tokenizer": "anthropic", "tokenizer_model": "claude-opus-4-8"}
type Config struct {
	Tokenizer      string // "heuristic" (default) or "anthropic"
	TokenizerModel string // defaults to DefaultModel
}

// LoadConfig reads the tokenizer settings via the shared config loader
// (internal/config). Missing fields yield the defaults.
func LoadConfig() Config {
	cfg := Config{Tokenizer: "heuristic", TokenizerModel: DefaultModel}
	file := config.Load()
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
