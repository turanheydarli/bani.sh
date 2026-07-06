package compact

import (
	"fmt"
	"strings"

	"go.banish.sh/banish/internal/compact/lockfile"
)

const (
	diffMaxLinesPerFile = 60
	diffMaxLinesTotal   = 400
)

// renderGitDiff condenses unified diff output: per file, a "=== path +a -d"
// header followed by only the changed lines with "@ line" position markers.
// Context lines and diff plumbing (index, ---/+++, diff --git) are dropped.
// Output that does not look like a unified diff falls through.
func renderGitDiff(stdout, stderr string, exitCode int) (string, bool) {
	text := strings.TrimRight(stdout, "\n")
	if text == "" {
		return "", false
	}
	if !strings.Contains(text, "diff --git") && !strings.Contains(text, "\n@@ ") && !strings.HasPrefix(text, "@@ ") {
		return "", false
	}

	var files []*fileDiff
	var cur *fileDiff

	for _, l := range strings.Split(text, "\n") {
		switch {
		case strings.HasPrefix(l, "diff --git "):
			cur = &fileDiff{path: parseDiffPath(l)}
			cur.lockfile = lockfile.Match(cur.path) != ""
			files = append(files, cur)
		case strings.HasPrefix(l, "index ") || strings.HasPrefix(l, "--- ") ||
			strings.HasPrefix(l, "new file mode") || strings.HasPrefix(l, "deleted file mode") ||
			strings.HasPrefix(l, "old mode") || strings.HasPrefix(l, "new mode") ||
			strings.HasPrefix(l, "similarity index") || strings.HasPrefix(l, "rename from") ||
			strings.HasPrefix(l, "rename to") || strings.HasPrefix(l, "Binary files"):
			// plumbing, drop
		case strings.HasPrefix(l, "+++ "):
			// prefer the +++ path (handles new files); overrides diff --git guess
			if cur != nil {
				if p := strings.TrimPrefix(l, "+++ b/"); p != l {
					cur.path = p
					cur.lockfile = lockfile.Match(cur.path) != ""
				}
			}
		case strings.HasPrefix(l, "@@ "):
			if cur == nil {
				cur = &fileDiff{path: "?"}
				files = append(files, cur)
			}
			cur.appendLine("@ "+parseHunkStart(l), diffMaxLinesPerFile)
		case strings.HasPrefix(l, "+"):
			if cur != nil {
				cur.adds++
				cur.rememberHunkLine(l)
				cur.appendLine(l, diffMaxLinesPerFile)
			}
		case strings.HasPrefix(l, "-"):
			if cur != nil {
				cur.dels++
				cur.rememberHunkLine(l)
				cur.appendLine(l, diffMaxLinesPerFile)
			}
		case strings.HasPrefix(l, " "):
			// Context lines are dropped from the compact body but preserved
			// for lockfile parsers, which rely on unchanged "name" lines to
			// attribute a bumped "version" line to the right package.
			if cur != nil {
				cur.rememberHunkLine(l)
			}
		default:
			// unknown line class, drop
		}
	}

	if len(files) == 0 {
		return "", false
	}

	// Second pass: replace each lockfile file's body with a semantic summary.
	// Parsers degrade gracefully — ok=false leaves the raw diff body in place.
	for _, f := range files {
		if !f.lockfile {
			continue
		}
		summary, ok := lockfile.Render(f.path, f.hunkLines)
		if !ok {
			continue
		}
		f.body = strings.Split(summary, "\n")
		f.omitted = 0
	}

	var out []string
	total := 0
	for _, f := range files {
		out = append(out, fmt.Sprintf("=== %s +%d -%d", f.path, f.adds, f.dels))
		for _, l := range f.body {
			if total >= diffMaxLinesTotal {
				out = append(out, fmt.Sprintf("... output capped at %d lines", diffMaxLinesTotal))
				return strings.Join(out, "\n"), true
			}
			out = append(out, l)
			total++
		}
		if f.omitted > 0 {
			out = append(out, fmt.Sprintf("+%d more changed lines", f.omitted))
		}
	}
	return strings.Join(out, "\n"), true
}

// fileDiff accumulates the condensed body of one file's diff. hunkLines
// buffers the raw hunk stream (with sigils) for lockfile files, which need
// context to attribute version changes to the right package. For non-lockfile
// files hunkLines stays nil so we do not pay the allocation.
type fileDiff struct {
	path      string
	adds      int
	dels      int
	body      []string
	omitted   int
	lockfile  bool
	hunkLines []string
}

// appendLine adds a body line, counting overflow past the per-file cap.
func (f *fileDiff) appendLine(l string, max int) {
	if len(f.body) < max {
		f.body = append(f.body, l)
	} else {
		f.omitted++
	}
}

// rememberHunkLine buffers a raw hunk line (including its diff sigil) so a
// downstream lockfile parser can walk the +, -, and context stream. No-op for
// non-lockfile files so raw diffs remain O(compact-body).
func (f *fileDiff) rememberHunkLine(l string) {
	if !f.lockfile {
		return
	}
	f.hunkLines = append(f.hunkLines, l)
}

// parseDiffPath extracts the b/ path from a "diff --git a/x b/x" line.
func parseDiffPath(l string) string {
	if i := strings.Index(l, " b/"); i >= 0 {
		return l[i+3:]
	}
	fields := strings.Fields(l)
	if len(fields) > 0 {
		return fields[len(fields)-1]
	}
	return "?"
}

// parseHunkStart extracts the new-file start line from "@@ -a,b +c,d @@ ctx".
func parseHunkStart(l string) string {
	if i := strings.Index(l, "+"); i >= 0 {
		rest := l[i+1:]
		if j := strings.IndexAny(rest, ", @"); j >= 0 {
			return rest[:j]
		}
		return rest
	}
	return "?"
}
