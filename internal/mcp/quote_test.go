package mcp

import (
	"testing"

	"go.banish.sh/banish/internal/lexer"
	"go.banish.sh/banish/internal/token"
)

// bshQuote must produce a .bsh string literal that the lexer decodes back to the
// exact input, for every byte - this is what keeps MCP argument values intact
// through the MCP -> .bsh -> builtin path. fmt %q does not have this property
// (it emits escapes the lexer does not decode), which is why bshQuote exists.
func TestBshQuoteRoundTrip(t *testing.T) {
	inputs := []string{
		"https://host/path?a=1&b=2", // colon + query: must not become a modifier
		"/tmp/notes -draft.txt",     // space + dash-word
		`a"b`,                       // embedded double quote
		`a\b`,                       // embedded backslash
		"tab\there",                 // tab
		"line\nbreak",               // newline
		"carriage\rreturn",          // CR: the exact byte %q would corrupt
		"plain",
		"",
	}
	for _, in := range inputs {
		quoted := bshQuote(in)
		lex := lexer.New(quoted)
		tok := lex.NextToken()
		if tok.Type != token.String {
			t.Errorf("bshQuote(%q) = %q lexed as %v, want String", in, quoted, tok.Type)
			continue
		}
		if tok.Literal != in {
			t.Errorf("round-trip mismatch: bshQuote(%q) decoded to %q", in, tok.Literal)
		}
		if next := lex.NextToken(); next.Type != token.EOF {
			t.Errorf("bshQuote(%q) = %q produced trailing token %v (not a single string)", in, quoted, next.Type)
		}
	}
}
