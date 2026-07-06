package compact

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// EnvNoRewrite is the environment variable that disables all rewrites when
// set to any non-empty value. Provides a hard-off switch for callers that
// need the invocation to reach the tool byte-for-byte -- CI comparators,
// pipe-sensitive scripts, or agents debugging why a filter surprised them.
const EnvNoRewrite = "BANISH_NO_REWRITE"

// AnnounceRewrite returns the audit line surfaced to the caller whenever a
// rewrite fired, or "" when no rewrite happened. Deliberately compact: the
// agent typed the original, so the useful signal is what banish *ran*.
// Single-line and clearly delimited so it round-trips through terminals and
// can be spliced out mechanically. Once the structured audit-footer work
// (see issue #13) lands, callers will absorb this into that footer.
func AnnounceRewrite(originalCmd, executedCmd string) string {
	if originalCmd == "" || executedCmd == "" || originalCmd == executedCmd {
		return ""
	}
	return fmt.Sprintf("[banish → %s]", executedCmd)
}

// RewriteRule swaps a verbose command for a machine-readable variant before
// execution. Defined in .bsh extensions via !rewrite / !match / !unless / !to.
//
// Announce controls whether AnnounceRewrite surfaces this rewrite in the
// output. Off by default: rewrites that only tweak formatting flags (e.g.
// git status --porcelain -b) are implementation details the agent does not
// need to see. On for rewrites that fundamentally change output shape
// (e.g. kubectl get -o json), where silence would confuse the caller.
type RewriteRule struct {
	Name     string   // rule name (for accounting)
	Match    string   // tokenized prefix pattern, e.g. "git status"
	Unless   []string // flags that disable the rewrite (the caller asked for a format)
	To       string   // replacement command prefix
	Announce bool     // surface a [banish → ...] line when this rule fires
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
//
// BANISH_NO_REWRITE=1 in the process environment disables every rule. This
// is the hard-off switch: MCP callers, CI comparators, and scripts sensitive
// to exact command bytes must be able to opt out with one env var.
func (rw *Rewriter) Rewrite(cmdline string) (string, string) {
	executed, _, name := rw.RewriteRule(cmdline)
	return executed, name
}

// RewriteRule applies the first matching rule and returns the executed
// command, the rule (or nil), and the rule name. Callers that need to know
// whether to surface the audit line (see Announce) reach for this variant.
func (rw *Rewriter) RewriteRule(cmdline string) (string, *RewriteRule, string) {
	if os.Getenv(EnvNoRewrite) != "" {
		return cmdline, nil, ""
	}
	if len(rw.rules) == 0 || !IsSimpleCommand(cmdline) {
		return cmdline, nil, ""
	}
	words := Tokenize(cmdline)
	if len(words) == 0 {
		return cmdline, nil, ""
	}
	for i := range rw.rules {
		r := &rw.rules[i]
		end, ok := MatchPrefix(words, r.Match)
		if !ok {
			continue
		}
		if hasAnyFlag(words, r.Unless) {
			continue
		}
		return r.To + cmdline[end:], r, r.Name
	}
	return cmdline, nil, ""
}
