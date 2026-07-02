package compact

import (
	"sort"
	"strings"
)

// RewriteRule swaps a verbose command for a machine-readable variant before
// execution. Defined in .bsh extensions via !rewrite / !match / !unless / !to.
type RewriteRule struct {
	Name   string   // rule name (for accounting)
	Match  string   // tokenized prefix pattern, e.g. "git status"
	Unless []string // flags that disable the rewrite (the caller asked for a format)
	To     string   // replacement command prefix
}

// Rewriter applies the first matching rewrite rule to a command string.
type Rewriter struct {
	rules []RewriteRule
}

// NewRewriter creates a rewriter. Rules are tried longest pattern first so
// "git diff --cached" wins over "git diff"; among equal lengths, earlier
// registration wins (user rules are registered before defaults).
func NewRewriter(rules []RewriteRule) *Rewriter {
	valid := make([]RewriteRule, 0, len(rules))
	for _, r := range rules {
		if r.Match != "" && r.To != "" {
			valid = append(valid, r)
		}
	}
	sort.SliceStable(valid, func(i, j int) bool {
		return len(strings.Fields(valid[i].Match)) > len(strings.Fields(valid[j].Match))
	})
	return &Rewriter{rules: valid}
}

// Rewrite returns the command to execute and the name of the applied rule
// (empty when no rule applied). Commands with pipes, redirects, chaining, or
// substitutions pass through untouched -- the original string is never
// re-lexed or reconstructed, only spliced after the matched prefix.
func (rw *Rewriter) Rewrite(cmdline string) (string, string) {
	if len(rw.rules) == 0 || !IsSimpleCommand(cmdline) {
		return cmdline, ""
	}
	words := Tokenize(cmdline)
	if len(words) == 0 {
		return cmdline, ""
	}
	for _, r := range rw.rules {
		end, ok := MatchPrefix(words, r.Match)
		if !ok {
			continue
		}
		if hasAnyFlag(words, r.Unless) {
			continue
		}
		return r.To + cmdline[end:], r.Name
	}
	return cmdline, ""
}
