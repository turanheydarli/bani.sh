package compact

import (
	"fmt"
	"strings"
)

// DroppedGroup accounts for lines a single filter stage removed from the
// output. Filter is the stage label (filter name plus the op that dropped,
// e.g. "go-test.drop", "npm-install.pipe", or a native renderer name).
type DroppedGroup struct {
	Filter string
	Lines  int
}

// TotalDropped sums the dropped lines across groups.
func TotalDropped(groups []DroppedGroup) int {
	n := 0
	for _, g := range groups {
		n += g.Lines
	}
	return n
}

// Footer suppression thresholds. Small outputs carry the full content
// anyway, and a footer on them would cost more tokens than it protects;
// the same goes for trivial savings.
const (
	// footerMinRawLines is the minimum raw line count before a footer
	// is worth emitting.
	footerMinRawLines = 40
	// footerMinRawBytes is the minimum raw byte size before a footer is
	// worth emitting; below ~2.5 KB the footer's own cost eats a visible
	// share of what compaction saved.
	footerMinRawBytes = 2500
	// footerMinSavedTokens is the minimum estimated dropped-token count
	// before a footer is worth emitting.
	footerMinSavedTokens = 50
)

// FooterInfo carries everything needed to render the audit footer.
type FooterInfo struct {
	Groups    []DroppedGroup
	RawLines  int    // line count of the raw output
	RawBytes  int    // byte size of the raw output
	EstTokens int64  // estimated tokens dropped (char-based heuristic)
	Recover   string // recover hash; "" omits the recover line
	Trace     bool   // BANISH_TRACE mode: groups are annotated inline instead
}

// Suppressed reports whether the footer should be omitted: nothing was
// dropped, the raw output was already small, or the savings are trivial.
func (fi FooterInfo) Suppressed() bool {
	return TotalDropped(fi.Groups) == 0 ||
		fi.RawLines < footerMinRawLines ||
		fi.RawBytes < footerMinRawBytes ||
		fi.EstTokens < footerMinSavedTokens
}

// RenderFooter renders the structured audit footer appended to compacted
// output, or "" when suppressed. In trace mode the per-group breakdown is
// omitted because each group is already annotated inline. The token estimate
// rides on the recover line as its price tag: recovering re-reads all the
// dropped content, so agents should reach for it only when the compacted
// output is actually missing something they need.
func RenderFooter(fi FooterInfo) string {
	if fi.Suppressed() {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "--- banish: dropped %d lines ---", TotalDropped(fi.Groups))
	if !fi.Trace {
		b.WriteString("\n  groups:")
		for _, g := range fi.Groups {
			fmt.Fprintf(&b, "\n    - filter: %s  lines: %d", g.Filter, g.Lines)
		}
	}
	if fi.Recover != "" {
		fmt.Fprintf(&b, "\n  recover: banish raw %s (costs ~%d tokens, only if needed)",
			fi.Recover, fi.EstTokens)
	} else {
		fmt.Fprintf(&b, "\n  est. %d tokens", fi.EstTokens)
	}
	return b.String()
}

// EstDroppedTokens estimates the token count of content dropped between raw
// and compacted text (~4 chars per token). Never a network counter -- this
// runs on the hot path.
func EstDroppedTokens(raw, out string) int64 {
	diff := len(raw) - len(out)
	if diff <= 0 {
		return 0
	}
	return int64(diff) / 4
}

// CountLines counts newline-separated lines in s ("" = 0 lines). A single
// trailing newline does not start a new line.
func CountLines(s string) int {
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

// traceAnnotation is the inline marker replacing a dropped group in
// BANISH_TRACE mode.
func traceAnnotation(n int, label string) string {
	return fmt.Sprintf("[banish: dropped %d lines via %s]", n, label)
}
