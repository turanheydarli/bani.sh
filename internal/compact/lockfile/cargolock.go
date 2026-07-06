package lockfile

import "regexp"

// Cargo.lock shape:
//
//   [[package]]
//   name = "serde"
//   version = "1.0.150"
//   source = "..."
//   dependencies = [ ... ]
//
// Package blocks are separated by [[package]] headers. Every block carries
// a name and a version. Diffs typically flip only the version line while
// name and header stay as context; the diff-line walk relies on context
// lines to attribute a version change to the right package.
//
// A block-boundary line ([[package]]) resets the current package on the
// side it appears on so a wholesale add/remove of a package does not leak
// its name into an adjacent block.

var (
	cargoPkgHeaderRe = regexp.MustCompile(`^\[\[package\]\]\s*$`)
	cargoNameRe      = regexp.MustCompile(`^name = "([^"]+)"`)
	cargoVersionRe   = regexp.MustCompile(`^version = "([^"]+)"`)
)

func parseCargoLock(hunk []string) (PackageDiff, bool) {
	minus := map[string]string{}
	plus := map[string]string{}
	var curMinus, curPlus string

	for _, raw := range hunk {
		sigil, content, ok := splitSigil(raw)
		if !ok {
			continue
		}

		if cargoPkgHeaderRe.MatchString(content) {
			switch sigil {
			case '+':
				curPlus = ""
			case '-':
				curMinus = ""
			case ' ':
				curMinus, curPlus = "", ""
			}
			continue
		}

		if m := cargoNameRe.FindStringSubmatch(content); m != nil {
			switch sigil {
			case '+':
				curPlus = m[1]
			case '-':
				curMinus = m[1]
			case ' ':
				curMinus, curPlus = m[1], m[1]
			}
			continue
		}

		if m := cargoVersionRe.FindStringSubmatch(content); m != nil {
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
