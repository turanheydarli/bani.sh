// Package permissions evaluates a shell command against the agent host's
// configured Bash permission rules and returns a verdict. banish uses the
// verdict to decide whether a command may be auto-approved, must prompt the
// user, or should be left for the host to handle. The verdict is never more
// permissive than the host's own rules: banish only auto-allows what the host
// already allows, and never auto-allows constructs it cannot safely analyze
// (command substitution, file-target redirects).
package permissions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Verdict is the result of checking a command against the host's rules.
type Verdict string

const (
	// Allow: an allow rule matched every segment -- safe to auto-approve.
	Allow Verdict = "allow"
	// Ask: an ask rule matched, or the command cannot be safely analyzed.
	Ask Verdict = "ask"
	// Deny: a deny rule matched -- leave it to the host's deny handling.
	Deny Verdict = "deny"
	// Default: no rule matched -- treat as ask (least privilege).
	Default Verdict = "default"
)

// Host is an agent host whose own permission rules banish consults.
type Host int

const (
	// HostClaudeCode reads Bash(...) rules from the .claude settings files.
	HostClaudeCode Host = iota
	// HostCursor reads Shell(...) rules from ~/.cursor/cli-config.json.
	HostCursor
)

// Check evaluates cmd against the Claude Code rules. It is shorthand for
// CheckFor(cmd, HostClaudeCode).
func Check(cmd string) Verdict {
	return CheckFor(cmd, HostClaudeCode)
}

// CheckFor evaluates cmd against the given host's permission rules.
func CheckFor(cmd string, host Host) Verdict {
	deny, ask, allow := loadRulesFor(host)
	return CheckWithRules(cmd, deny, ask, allow)
}

// CheckWithRules evaluates cmd against explicit rule sets. Precedence is
// Deny > Ask > Allow > Default. A compound command receives Allow only when
// every non-empty segment independently matches an allow rule -- one allowed
// segment must never escalate the whole chain.
func CheckWithRules(cmd string, deny, ask, allow []string) Verdict {
	segments := splitForPermissions(cmd)

	// Deny has the highest priority and pre-empts everything else.
	for _, seg := range segments {
		seg = strings.TrimSpace(seg)
		for _, p := range deny {
			if commandMatchesPattern(seg, p) {
				return Deny
			}
		}
	}

	// Constructs we cannot decompose (substitution, file redirects) must never
	// be auto-allowed.
	if ContainsUnattestable(cmd) {
		return Ask
	}

	anyAsk := false
	allAllowed := true
	sawSegment := false

	for _, seg := range segments {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		sawSegment = true

		if !anyAsk {
			for _, p := range ask {
				if commandMatchesPattern(seg, p) {
					anyAsk = true
					break
				}
			}
		}

		if allAllowed {
			matched := false
			for _, p := range allow {
				if commandMatchesPattern(seg, p) {
					matched = true
					break
				}
			}
			if !matched {
				allAllowed = false
			}
		}
	}

	switch {
	case anyAsk:
		return Ask
	case sawSegment && allAllowed && len(allow) > 0:
		return Allow
	default:
		return Default
	}
}

// ContainsUnattestable reports whether cmd contains a construct that cannot be
// safely reasoned about for auto-approval: command substitution ($(...), back
// ticks, process substitution) or a redirect to a real file.
func ContainsUnattestable(cmd string) bool {
	return containsSubstitution(cmd) || containsFileRedirect(cmd)
}

// containsSubstitution scans for shell substitution, respecting quoting: bash
// expands backticks and $(...) unquoted and inside double quotes, but treats
// single-quoted text literally; <(...) and >(...) are unquoted-only.
func containsSubstitution(cmd string) bool {
	b := []byte(cmd)
	inSingle, inDouble := false, false
	for i := 0; i < len(b); i++ {
		c := b[i]
		switch {
		case c == '\\' && !inSingle:
			i++
			continue
		case c == '\'' && !inDouble:
			inSingle = !inSingle
		case c == '"' && !inSingle:
			inDouble = !inDouble
		case c == '`' && !inSingle:
			return true
		case c == '$' && !inSingle && i+1 < len(b) && b[i+1] == '(':
			return true
		case (c == '<' || c == '>') && !inSingle && !inDouble && i+1 < len(b) && b[i+1] == '(':
			return true
		}
	}
	return false
}

// containsFileRedirect scans for a redirect whose target is a real file (not an
// fd duplication like 2>&1 or a redirect to /dev/null), respecting quoting.
func containsFileRedirect(cmd string) bool {
	b := []byte(cmd)
	inSingle, inDouble := false, false
	for i := 0; i < len(b); i++ {
		c := b[i]
		switch {
		case c == '\\' && !inSingle:
			i++
			continue
		case c == '\'' && !inDouble:
			inSingle = !inSingle
			continue
		case c == '"' && !inSingle:
			inDouble = !inDouble
			continue
		}
		if inSingle || inDouble {
			continue
		}
		if c != '>' && c != '<' {
			continue
		}

		// Consume the redirect operator (>, >>, <, etc.).
		j := i
		for j < len(b) && (b[j] == '>' || b[j] == '<') {
			j++
		}
		// fd duplication: >&N, >&-, N>&M -- not a file target.
		if j < len(b) && b[j] == '&' {
			k := j + 1
			start := k
			for k < len(b) && (b[k] >= '0' && b[k] <= '9' || b[k] == '-') {
				k++
			}
			if k > start {
				i = k - 1
				continue
			}
		}
		// Skip whitespace and a leading & before the target token.
		k := j
		for k < len(b) && (b[k] == ' ' || b[k] == '\t' || b[k] == '&') {
			k++
		}
		start := k
		for k < len(b) && !isSegmentBreak(b[k]) {
			k++
		}
		target := string(b[start:k])
		if target == "/dev/null" {
			i = k - 1
			continue
		}
		return true
	}
	return false
}

func isSegmentBreak(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == ';' || c == '|' || c == '&' || c == '(' || c == ')'
}

// splitForPermissions splits a compound command into segments on the shell
// operators &&, ||, ;, |, &, newline, and subshell parentheses, and truncates
// each segment at its first redirect. Quoting is respected so operators inside
// quotes do not split. Callers must gate on ContainsUnattestable first.
func splitForPermissions(cmd string) []string {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return nil
	}

	var segments []string
	inSingle, inDouble := false, false
	segStart := 0
	segEnd := -1 // first redirect offset in the current segment, -1 if none

	flush := func(end int) {
		if segEnd >= 0 && segEnd < end {
			end = segEnd
		}
		if s := strings.TrimSpace(cmd[segStart:end]); s != "" {
			segments = append(segments, s)
		}
		segEnd = -1
	}

	b := []byte(cmd)
	for i := 0; i < len(b); i++ {
		c := b[i]
		switch {
		case c == '\\' && !inSingle:
			i++
			continue
		case c == '\'' && !inDouble:
			inSingle = !inSingle
			continue
		case c == '"' && !inSingle:
			inDouble = !inDouble
			continue
		}
		if inSingle || inDouble {
			continue
		}

		switch {
		case c == '>' || c == '<':
			// Truncate the segment at the first redirect, and consume the
			// whole redirect operator (including an fd-dup tail like >&1) so a
			// literal '&' in 2>&1 is not mistaken for a background separator.
			if segEnd < 0 {
				segEnd = i
			}
			j := i + 1
			for j < len(b) && (b[j] == '>' || b[j] == '<') {
				j++
			}
			if j < len(b) && b[j] == '&' {
				k := j + 1
				for k < len(b) && (b[k] >= '0' && b[k] <= '9' || b[k] == '-') {
					k++
				}
				if k > j+1 {
					i = k - 1
					continue
				}
			}
			i = j - 1
		case c == '&' && i+1 < len(b) && b[i+1] == '&',
			c == '|' && i+1 < len(b) && b[i+1] == '|':
			flush(i)
			i++
			segStart = i + 1
		case c == ';' || c == '|' || c == '&' || c == '\n' || c == '(' || c == ')':
			flush(i)
			segStart = i + 1
		}
	}
	flush(len(b))

	return segments
}

// commandMatchesPattern reports whether cmd matches a host permission pattern.
//
// Pattern forms:
//   - "*"                       matches everything
//   - "prefix:*" / "prefix *"   trailing wildcard, prefix match with word boundary
//   - "* suffix" / "pre * suf"  glob: * matches any sequence of characters
//   - "pattern"                 exact match or prefix match (cmd == pattern or
//     cmd starts with "pattern ")
func commandMatchesPattern(cmd, pattern string) bool {
	if pattern == "*" {
		return true
	}

	if p, ok := strings.CutSuffix(pattern, "*"); ok {
		prefix := strings.TrimRight(p, ":")
		prefix = strings.TrimRight(prefix, " \t")
		if prefix == "" || prefix == "*" {
			return true
		}
		if !strings.Contains(prefix, "*") {
			return cmd == prefix || strings.HasPrefix(cmd, prefix+" ")
		}
	}

	if strings.Contains(pattern, "*") {
		return globMatches(cmd, pattern)
	}

	return cmd == pattern || strings.HasPrefix(cmd, pattern+" ")
}

// globMatches matches cmd against a pattern where * matches any character
// sequence. Colon-wildcard syntax is normalized: "sudo:*" -> "sudo *".
func globMatches(cmd, pattern string) bool {
	normalized := strings.ReplaceAll(pattern, ":*", " *")
	normalized = strings.ReplaceAll(normalized, "*:", "* ")
	parts := strings.Split(normalized, "*")

	allEmpty := true
	for _, p := range parts {
		if p != "" {
			allEmpty = false
			break
		}
	}
	if allEmpty {
		return true
	}

	searchFrom := 0
	for i, part := range parts {
		if part == "" {
			continue
		}
		switch {
		case i == 0:
			if !strings.HasPrefix(cmd, part) {
				return false
			}
			searchFrom = len(part)
		case i == len(parts)-1:
			if searchFrom > len(cmd) || !strings.HasSuffix(cmd[searchFrom:], part) {
				return false
			}
		default:
			if searchFrom > len(cmd) {
				return false
			}
			remaining := cmd[searchFrom:]
			if pos := strings.Index(remaining, part); pos >= 0 {
				searchFrom += pos + len(part)
			} else {
				trimmed := strings.TrimRight(part, " \t")
				if trimmed != "" && strings.HasSuffix(remaining, trimmed) {
					searchFrom += len(remaining)
				} else {
					return false
				}
			}
		}
	}
	return true
}

// loadRulesFor returns the deny/ask/allow rule sets for a host.
func loadRulesFor(host Host) (deny, ask, allow []string) {
	switch host {
	case HostCursor:
		return loadCursorRules()
	default:
		return loadClaudeRules()
	}
}

// loadClaudeRules reads Bash deny/ask/allow patterns from the Claude Code
// settings files, in order: project .claude/settings.json and
// settings.local.json, then the user's ~/.claude equivalents. Missing or
// malformed files are skipped.
func loadClaudeRules() (deny, ask, allow []string) {
	for _, path := range settingsPaths() {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var cfg struct {
			Permissions struct {
				Deny  []string `json:"deny"`
				Ask   []string `json:"ask"`
				Allow []string `json:"allow"`
			} `json:"permissions"`
		}
		if json.Unmarshal(data, &cfg) != nil {
			continue
		}
		deny = append(deny, extractWrappedRules(cfg.Permissions.Deny, "Bash(")...)
		ask = append(ask, extractWrappedRules(cfg.Permissions.Ask, "Bash(")...)
		allow = append(allow, extractWrappedRules(cfg.Permissions.Allow, "Bash(")...)
	}
	return deny, ask, allow
}

// loadCursorRules reads Shell deny/allow patterns from the global Cursor CLI
// config (~/.cursor/cli-config.json). Cursor has no ask tier. A bare "Shell"
// rule (no parentheses) means everything. Only the global config is read, so
// banish is never more permissive than Cursor's own project trust allows.
func loadCursorRules() (deny, ask, allow []string) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil, nil, nil
	}
	data, err := os.ReadFile(filepath.Join(home, ".cursor", "cli-config.json"))
	if err != nil {
		return nil, nil, nil
	}
	var cfg struct {
		Permissions struct {
			Deny  []string `json:"deny"`
			Allow []string `json:"allow"`
		} `json:"permissions"`
	}
	if json.Unmarshal(data, &cfg) != nil {
		return nil, nil, nil
	}
	deny = extractWrappedRules(cfg.Permissions.Deny, "Shell(")
	allow = extractWrappedRules(cfg.Permissions.Allow, "Shell(")
	return deny, nil, allow
}

// extractWrappedRules keeps only rules wrapped in the given prefix (for example
// "Bash(" or "Shell(") and returns the inner pattern. A bare prefix word with no
// parentheses (for example "Shell") matches everything.
func extractWrappedRules(rules []string, prefix string) []string {
	bare := strings.TrimSuffix(prefix, "(")
	var out []string
	for _, r := range rules {
		switch {
		case r == bare:
			out = append(out, "*")
		case strings.HasPrefix(r, prefix) && strings.HasSuffix(r, ")"):
			out = append(out, r[len(prefix):len(r)-1])
		}
	}
	return out
}

// settingsPaths returns the host settings files to read, project first.
func settingsPaths() []string {
	var paths []string
	if root := projectRoot(); root != "" {
		paths = append(paths,
			filepath.Join(root, ".claude", "settings.json"),
			filepath.Join(root, ".claude", "settings.local.json"),
		)
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		paths = append(paths,
			filepath.Join(home, ".claude", "settings.json"),
			filepath.Join(home, ".claude", "settings.local.json"),
		)
	}
	return paths
}

// projectRoot walks up from the working directory to the nearest ancestor with
// a .claude directory, excluding the home directory (whose ~/.claude is the
// global scope, handled separately). It returns "" when none is found.
func projectRoot() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	home, _ := os.UserHomeDir()
	dir := cwd
	for {
		if dir == home {
			return ""
		}
		if fi, err := os.Stat(filepath.Join(dir, ".claude")); err == nil && fi.IsDir() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
