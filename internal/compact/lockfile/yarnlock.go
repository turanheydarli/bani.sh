package lockfile

import (
	"regexp"
	"strings"
)

// yarn.lock v1 shape:
//
//   express@^4.19.0, express@^4.19.2:
//     version "4.19.2"
//     resolved "..."
//     integrity ...
//
//   "@types/node@^20.0.0":
//     version "20.0.0"
//
// Entries start at column 0 and end with a colon; nested fields are indented.
// The first "name@spec" identifies the package; a leading @ marks a scope.
// A quoted key wraps the whole spec list. Version lives inside as
// `  version "X.Y.Z"`.
//
// yarn.lock lists every resolved package (direct plus transitive) — the diff
// is already at the package level; direct vs transitive is not distinguishable
// from the lockfile alone. The summary treats all entries uniformly.

var (
	yarnVersionRe = regexp.MustCompile(`^\s+version\s+"([^"]+)"`)
)

func parseYarnLock(hunk []string) (PackageDiff, bool) {
	minus := map[string]string{}
	plus := map[string]string{}
	var curMinus, curPlus string

	for _, raw := range hunk {
		sigil, content, ok := splitSigil(raw)
		if !ok {
			continue
		}

		if name := yarnKeyName(content); name != "" {
			switch sigil {
			case '+':
				curPlus = name
			case '-':
				curMinus = name
			case ' ':
				curMinus, curPlus = name, name
			}
			continue
		}

		if m := yarnVersionRe.FindStringSubmatch(content); m != nil {
			ver := m[1]
			switch sigil {
			case '+':
				if curPlus != "" {
					plus[curPlus] = ver
				}
			case '-':
				if curMinus != "" {
					minus[curMinus] = ver
				}
			}
		}
	}

	return diffMaps(minus, plus), true
}

// yarnKeyName returns the package name from a yarn.lock entry header like
// `express@^4.19.0, express@^4.19.2:` or `"@types/node@^20.0.0":`, or "".
// Only column-0 entries ending in `:` are keys; indented lines are fields.
func yarnKeyName(content string) string {
	if content == "" || content[0] == ' ' || content[0] == '\t' {
		return ""
	}
	trimmed := strings.TrimRight(content, " ")
	if !strings.HasSuffix(trimmed, ":") {
		return ""
	}
	head := strings.TrimSuffix(trimmed, ":")

	// Take the first comma-separated spec; strip surrounding quotes.
	first := head
	if i := strings.Index(head, ","); i >= 0 {
		first = head[:i]
	}
	first = strings.TrimSpace(first)
	first = strings.TrimPrefix(first, `"`)
	first = strings.TrimSuffix(first, `"`)

	// Split name@spec. Scoped packages start with @, so skip the first char
	// when locating the split.
	if first == "" {
		return ""
	}
	sep := strings.LastIndex(first, "@")
	if sep <= 0 {
		return ""
	}
	name := first[:sep]
	if name == "" {
		return ""
	}
	return name
}
