package lockfile

import (
	"regexp"
	"strings"
)

// npm v2/v3 lockfile shape (also npm-shrinkwrap.json):
//
//   "packages": {
//     "": { ...root package... },
//     "node_modules/express": { "version": "4.19.2", ... },
//     "node_modules/foo/node_modules/bar": { "version": "1.0", ... }
//   }
//
// v1 shape (deprecated but still in the wild):
//
//   "dependencies": {
//     "express": { "version": "4.19.2", "dependencies": { ... } }
//   }
//
// Both encode a package as a JSON block whose key is the package name (or
// its node_modules path) and whose "version" field is the resolved version.
// We do not run a full JSON parse; a diff-line walk suffices because the
// keys and version fields have fixed shapes.

var (
	pkgLockKeyRe = regexp.MustCompile(`^\s*"(node_modules/[^"]+)":\s*\{`)
	// v1 nested "name": { under a "dependencies" block. Guard against
	// meta keys ("dependencies", "requires", "engines", ...) by matching
	// only lines whose value opens a JSON object and whose key has no
	// slash (real package names may contain @scope/ but node_modules/ path
	// is handled by pkgLockKeyRe above).
	pkgLockV1KeyRe = regexp.MustCompile(`^\s{4,}"(@?[a-zA-Z0-9][a-zA-Z0-9_.\-/]*?)":\s*\{`)
	pkgLockVerRe   = regexp.MustCompile(`^\s*"version":\s*"([^"]+)"`)
)

// pkgLockMetaKeys are v1 "name": { blocks we must not treat as packages.
var pkgLockMetaKeys = map[string]bool{
	"dependencies": true,
	"devDependencies": true,
	"peerDependencies": true,
	"optionalDependencies": true,
	"requires":     true,
	"engines":      true,
	"scripts":      true,
	"funding":      true,
	"bin":          true,
	"peerDependenciesMeta": true,
}

// parsePackageLock walks a diff hunk and extracts (package, version) tuples
// for each side. Returns ok=true whenever the input looks like it is coming
// from a lockfile at all (so we always render a summary; the caller has
// already established the path is a lockfile).
func parsePackageLock(hunk []string) (PackageDiff, bool) {
	minus := map[string]string{}
	plus := map[string]string{}

	// currentPkg holds the last package name seen on each side. A version
	// line resolves against the side's current package; context lines set
	// both sides so a version bumped inside an otherwise-unchanged block
	// still attributes to the right name.
	var curMinus, curPlus string

	for _, raw := range hunk {
		sigil, content, ok := splitSigil(raw)
		if !ok {
			continue
		}

		if name := packageName(content); name != "" {
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

		if m := pkgLockVerRe.FindStringSubmatch(content); m != nil {
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
			continue
		}
	}

	return diffMaps(minus, plus), true
}

// packageName returns the package name a lockfile block key encodes, or "".
// Handles both v2/v3 "node_modules/..." and v1 nested "<name>": { keys.
// Nested node_modules paths (transitive) collapse to their final segment so
// two versions of the same package at different depths still diff cleanly.
func packageName(content string) string {
	if m := pkgLockKeyRe.FindStringSubmatch(content); m != nil {
		return lastNodeModulesSegment(m[1])
	}
	if m := pkgLockV1KeyRe.FindStringSubmatch(content); m != nil {
		name := m[1]
		if pkgLockMetaKeys[name] {
			return ""
		}
		return name
	}
	return ""
}

// lastNodeModulesSegment extracts the effective package name from a v2/v3
// key like "node_modules/foo/node_modules/@scope/bar" → "@scope/bar".
func lastNodeModulesSegment(key string) string {
	const marker = "node_modules/"
	i := strings.LastIndex(key, marker)
	if i < 0 {
		return key
	}
	return key[i+len(marker):]
}
