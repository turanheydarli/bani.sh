package compact

import (
	"fmt"
	"strings"
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
				cur.appendLine(l, diffMaxLinesPerFile)
			}
		case strings.HasPrefix(l, "-"):
			if cur != nil {
				cur.dels++
				cur.appendLine(l, diffMaxLinesPerFile)
			}
		default:
			// context line, drop
		}
	}

	if len(files) == 0 {
		return "", false
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

// fileDiff accumulates the condensed body of one file's diff.
type fileDiff struct {
	path    string
	adds    int
	dels    int
	body    []string
	omitted int
}

// appendLine adds a body line, counting overflow past the per-file cap.
func (f *fileDiff) appendLine(l string, max int) {
	if len(f.body) < max {
		f.body = append(f.body, l)
	} else {
		f.omitted++
	}
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
