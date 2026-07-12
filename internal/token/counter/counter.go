// Package counter measures the token cost of strings. It is distinct from
// internal/token, which holds the .bsh lexer's token types.
//
// Two implementations exist: CharHeuristic, a fast offline chars/4 estimate,
// and Anthropic, which asks Anthropic's count_tokens endpoint for the real
// number and falls back to the heuristic when offline or unauthorized.
package counter

// Counter measures the token cost of a string.
type Counter interface {
	// Count returns the token count and whether it is exact (measured by a
	// real tokenizer) or a heuristic estimate.
	Count(s string) (n int64, exact bool)
	// Name identifies the tokenizer for display in stats and bench output.
	Name() string
}

// CharHeuristic estimates tokens as len(s)/4. That is a rule of thumb, not
// Claude's tokenizer: it is systematically wrong for non-ASCII content,
// long identifiers, whitespace runs, and box-drawing tables, with errors
// plausibly around +/-30% depending on content. Every count it returns is
// an estimate and is labeled as such.
type CharHeuristic struct{}

// Count implements Counter. The result is never exact.
func (CharHeuristic) Count(s string) (int64, bool) {
	n := int64(len(s)) / 4
	if n == 0 && len(s) > 0 {
		n = 1
	}
	return n, false
}

// Name implements Counter.
func (CharHeuristic) Name() string { return "char-based estimate (~4 chars/token, approx)" }
