package compact

import (
	"path/filepath"
	"strings"
)

// Word is a shell word with its byte offsets in the original command string.
// Text has surrounding quotes removed; offsets always refer to the raw input
// so callers can splice the original string without re-quoting.
type Word struct {
	Text  string
	Start int
	End   int
}

// shellMeta lists characters that make a command non-simple: pipes,
// redirects, substitutions, chaining. Commands containing any of these are
// never rewritten and only matched by their leading words.
const shellMeta = "|&;<>$`(){}\n"

// IsSimpleCommand reports whether cmdline is a single plain invocation with
// no shell metacharacters outside quotes.
func IsSimpleCommand(cmdline string) bool {
	inSingle, inDouble := false, false
	for i := 0; i < len(cmdline); i++ {
		c := cmdline[i]
		switch {
		case c == '\\' && !inSingle:
			i++ // skip escaped char
		case c == '\'' && !inDouble:
			inSingle = !inSingle
		case c == '"' && !inSingle:
			inDouble = !inDouble
		case !inSingle && !inDouble && strings.IndexByte(shellMeta, c) >= 0:
			return false
		case inDouble && (c == '$' || c == '`'):
			return false // expansion inside double quotes
		}
	}
	return !inSingle && !inDouble
}

// Tokenize splits cmdline into shell words, respecting quotes and backslash
// escapes. It is used for inspection only -- rewrites splice the original
// string via word offsets, never by rejoining tokens.
func Tokenize(cmdline string) []Word {
	var words []Word
	var cur strings.Builder
	start := -1
	inSingle, inDouble := false, false

	flush := func(end int) {
		if start >= 0 {
			words = append(words, Word{Text: cur.String(), Start: start, End: end})
			cur.Reset()
			start = -1
		}
	}

	for i := 0; i < len(cmdline); i++ {
		c := cmdline[i]
		switch {
		case c == '\\' && !inSingle && i+1 < len(cmdline):
			if start < 0 {
				start = i
			}
			i++
			cur.WriteByte(cmdline[i])
		case c == '\'' && !inDouble:
			if start < 0 {
				start = i
			}
			inSingle = !inSingle
		case c == '"' && !inSingle:
			if start < 0 {
				start = i
			}
			inDouble = !inDouble
		case (c == ' ' || c == '\t') && !inSingle && !inDouble:
			flush(i)
		default:
			if start < 0 {
				start = i
			}
			cur.WriteByte(c)
		}
	}
	flush(len(cmdline))
	return words
}

// MatchPrefix reports whether the command words match the space-separated
// pattern (e.g. "git status"). The first pattern word is compared against
// the basename of the command, remaining words must follow consecutively.
// Returns the byte offset just past the last matched word, for splicing.
func MatchPrefix(words []Word, pattern string) (int, bool) {
	pat := strings.Fields(pattern)
	if len(pat) == 0 || len(words) < len(pat) {
		return 0, false
	}
	if filepath.Base(words[0].Text) != pat[0] {
		return 0, false
	}
	for i := 1; i < len(pat); i++ {
		if words[i].Text != pat[i] {
			return 0, false
		}
	}
	return words[len(pat)-1].End, true
}

// hasAnyFlag reports whether any word matches one of the given flags,
// either exactly or as a "--flag=value" prefix. The flag "-n" additionally
// matches git-style numeric shorthands like "-5".
func hasAnyFlag(words []Word, flags []string) bool {
	for _, w := range words {
		for _, f := range flags {
			if w.Text == f || strings.HasPrefix(w.Text, f+"=") {
				return true
			}
			if f == "-n" && isDashNumber(w.Text) {
				return true
			}
		}
	}
	return false
}

// isDashNumber reports whether s is a numeric shorthand flag like "-5".
func isDashNumber(s string) bool {
	if len(s) < 2 || s[0] != '-' {
		return false
	}
	for i := 1; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
