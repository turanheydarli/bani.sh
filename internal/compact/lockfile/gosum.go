package lockfile

import (
	"regexp"
	"strings"
)

// go.sum shape:
//
//   github.com/foo/bar v1.2.3 h1:abc123=
//   github.com/foo/bar v1.2.3/go.mod h1:xyz789=
//
// Each module appears twice: once for the module itself, once for its go.mod.
// The version on the /go.mod line has "/go.mod" suffixed. Both lines carry
// the same module@version, so idempotent map assignment collapses them.
//
// go.sum has no notion of direct vs transitive without reading go.mod. The
// summary treats every module change uniformly; distinguishing direct/
// transitive is a follow-up (see issue #18 open questions).

var goSumEntryRe = regexp.MustCompile(`^(\S+) (\S+) h1:`)

func parseGoSum(hunk []string) (PackageDiff, bool) {
	minus := map[string]string{}
	plus := map[string]string{}

	for _, raw := range hunk {
		sigil, content, ok := splitSigil(raw)
		if !ok {
			continue
		}
		if sigil == ' ' {
			continue
		}
		m := goSumEntryRe.FindStringSubmatch(content)
		if m == nil {
			continue
		}
		module := m[1]
		version := strings.TrimSuffix(m[2], "/go.mod")
		switch sigil {
		case '+':
			plus[module] = version
		case '-':
			minus[module] = version
		}
	}

	return diffMaps(minus, plus), true
}
