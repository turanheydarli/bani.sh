// Package lockfile renders unified-diff hunks of dependency lockfiles as a
// package-level semantic summary. Lockfiles produce catastrophic token bills
// under raw diff (thousands of lines describing transitive shuffle) while
// carrying a signal that is a handful of directly changed packages. Each
// parser walks the hunk stream and returns a PackageDiff; the caller replaces
// the raw file body in the surrounding git-diff render.
//
// Parsers must degrade gracefully: any input they cannot understand returns
// ok=false so the caller falls back to the raw diff. Never lose data.
package lockfile

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Match returns the parser id for a path if it names a supported lockfile,
// or "" if not. Matching is by basename only; callers pass the b/ path from
// a unified diff header.
func Match(path string) string {
	switch filepath.Base(path) {
	case "package-lock.json", "npm-shrinkwrap.json":
		return "package-lock"
	case "yarn.lock":
		return "yarn"
	case "Cargo.lock":
		return "cargo"
	case "go.sum":
		return "go-sum"
	}
	return ""
}

// Render produces a semantic diff summary from unified-diff hunk lines (each
// line still carrying its leading '+', '-', or ' ' sigil). Returns ok=false
// when disabled via BANISH_LOCKFILE_FULL, when the path is not a lockfile,
// or when the parser cannot recognize the format.
func Render(path string, hunk []string) (string, bool) {
	if os.Getenv("BANISH_LOCKFILE_FULL") != "" {
		return "", false
	}
	kind := Match(path)
	if kind == "" {
		return "", false
	}

	var (
		diff PackageDiff
		ok   bool
	)
	switch kind {
	case "package-lock":
		diff, ok = parsePackageLock(hunk)
	case "yarn":
		diff, ok = parseYarnLock(hunk)
	case "cargo":
		diff, ok = parseCargoLock(hunk)
	case "go-sum":
		diff, ok = parseGoSum(hunk)
	}
	if !ok {
		return "", false
	}
	return diff.Format(), true
}

// PackageDiff is a symmetric before/after set of changed packages.
type PackageDiff struct {
	Added    []NameVer
	Upgraded []Upgrade
	Removed  []NameVer
}

// NameVer names a package at a single version.
type NameVer struct {
	Name    string
	Version string
}

// Upgrade names a package with its before and after versions.
type Upgrade struct {
	Name string
	From string
	To   string
}

// Format renders the diff as a stable, human-readable block. The output is
// deterministic (packages sorted by name) so goldens do not churn.
func (d PackageDiff) Format() string {
	sort.Slice(d.Added, func(i, j int) bool { return d.Added[i].Name < d.Added[j].Name })
	sort.Slice(d.Removed, func(i, j int) bool { return d.Removed[i].Name < d.Removed[j].Name })
	sort.Slice(d.Upgraded, func(i, j int) bool { return d.Upgraded[i].Name < d.Upgraded[j].Name })

	var b strings.Builder
	b.WriteString("(semantic diff via banish)\n")
	if len(d.Added) > 0 {
		fmt.Fprintf(&b, "  + added:    %s\n", joinNameVer(d.Added))
	}
	if len(d.Upgraded) > 0 {
		fmt.Fprintf(&b, "  ↑ upgraded: %s\n", joinUpgrades(d.Upgraded))
	}
	if len(d.Removed) > 0 {
		fmt.Fprintf(&b, "  - removed:  %s\n", joinNameVer(d.Removed))
	}
	if len(d.Added)+len(d.Upgraded)+len(d.Removed) == 0 {
		b.WriteString("  (no package changes detected)\n")
	}
	b.WriteString("  [set BANISH_LOCKFILE_FULL=1 for the raw diff]")
	return b.String()
}

func joinNameVer(items []NameVer) string {
	parts := make([]string, len(items))
	for i, nv := range items {
		parts[i] = nv.Name + "@" + nv.Version
	}
	return strings.Join(parts, ", ")
}

func joinUpgrades(items []Upgrade) string {
	parts := make([]string, len(items))
	for i, u := range items {
		parts[i] = fmt.Sprintf("%s %s → %s", u.Name, u.From, u.To)
	}
	return strings.Join(parts, ", ")
}

// hunkSide separates a hunk line into its diff sigil and content. Context
// lines (' ') update both sides; the sigil "" marker signals a caller to
// skip. Empty raw lines carry no signal.
func splitSigil(raw string) (sigil byte, content string, ok bool) {
	if raw == "" {
		return 0, "", false
	}
	c := raw[0]
	if c != '+' && c != '-' && c != ' ' {
		return 0, "", false
	}
	return c, raw[1:], true
}

// diffMaps computes added/removed/upgraded between two name→version maps.
// Same name with different version is an upgrade; new name is added; missing
// name is removed. Order-independent; caller sorts in Format.
func diffMaps(before, after map[string]string) PackageDiff {
	var d PackageDiff
	seen := make(map[string]bool, len(after))
	for name, v := range after {
		seen[name] = true
		if bv, exists := before[name]; exists {
			if bv != v {
				d.Upgraded = append(d.Upgraded, Upgrade{Name: name, From: bv, To: v})
			}
			continue
		}
		d.Added = append(d.Added, NameVer{Name: name, Version: v})
	}
	for name, v := range before {
		if !seen[name] {
			d.Removed = append(d.Removed, NameVer{Name: name, Version: v})
		}
	}
	return d
}
