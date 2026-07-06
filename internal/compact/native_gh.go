package compact

import (
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"
	"time"
)

// gh --json returns a top-level JSON array whose element shape depends on
// which fields the caller (or our rewrite rule) selected:
//
//   gh pr list  --json number,title,state,author,createdAt,headRefName
//   gh run list --json databaseId,name,status,conclusion,displayTitle,createdAt
//
// The rewrite rules in defaults.bsh request a fixed field set per subcommand
// so we can rely on a stable input shape. If the caller invoked gh with a
// different --json field set the parser degrades to the fields it finds and
// omits missing columns.

const ghMaxRows = 30

// renderGHPRList parses `gh pr list --json ...` output into a compact table.
// Falls through when stdout is not a JSON array of PR-shaped objects.
func renderGHPRList(stdout, stderr string, exitCode int) (string, bool) {
	items, ok := decodeGHArray(stdout)
	if !ok {
		return "", false
	}

	// Zero PRs is a real answer, not a fallthrough: render an empty state so
	// the caller knows banish saw a valid --json response.
	if len(items) == 0 {
		return "no pull requests", true
	}

	// Sanity-check the first object carries PR-shaped keys before committing
	// to this renderer. Distinguishes a PR-list JSON from a run-list JSON if
	// they ever collide on the same match prefix.
	if _, ok := items[0]["number"]; !ok {
		return "", false
	}

	var b strings.Builder
	tw := tabwriter.NewWriter(&b, 2, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "NUM\tSTATE\tAUTHOR\tBRANCH\tAGE\tTITLE")

	shown := items
	overflow := 0
	if len(shown) > ghMaxRows {
		overflow = len(shown) - ghMaxRows
		shown = shown[:ghMaxRows]
	}
	for _, pr := range shown {
		fmt.Fprintf(tw, "#%s\t%s\t%s\t%s\t%s\t%s\n",
			ghStr(pr, "number"),
			ghStr(pr, "state"),
			ghAuthor(pr),
			ghStr(pr, "headRefName"),
			ghAge(ghStr(pr, "createdAt")),
			ghStr(pr, "title"),
		)
	}
	tw.Flush()
	if overflow > 0 {
		fmt.Fprintf(&b, "+%d more pull requests\n", overflow)
	}
	return strings.TrimRight(b.String(), "\n"), true
}

// renderGHRunList parses `gh run list --json ...` output into a table.
// gh's status/conclusion split is preserved: "queued", "in_progress",
// "completed" statuses; "success", "failure", "cancelled" conclusions.
func renderGHRunList(stdout, stderr string, exitCode int) (string, bool) {
	items, ok := decodeGHArray(stdout)
	if !ok {
		return "", false
	}
	if len(items) == 0 {
		return "no workflow runs", true
	}

	// Run-list objects carry databaseId and status; PR-list carries number.
	if _, hasDBID := items[0]["databaseId"]; !hasDBID {
		if _, hasName := items[0]["name"]; !hasName {
			return "", false
		}
	}

	var b strings.Builder
	tw := tabwriter.NewWriter(&b, 2, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tSTATUS\tCONCLUSION\tWORKFLOW\tAGE\tTITLE")

	shown := items
	overflow := 0
	if len(shown) > ghMaxRows {
		overflow = len(shown) - ghMaxRows
		shown = shown[:ghMaxRows]
	}
	for _, run := range shown {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			ghStr(run, "databaseId"),
			ghStr(run, "status"),
			nonEmpty(ghStr(run, "conclusion"), "-"),
			ghStr(run, "name"),
			ghAge(ghStr(run, "createdAt")),
			ghStr(run, "displayTitle"),
		)
	}
	tw.Flush()
	if overflow > 0 {
		fmt.Fprintf(&b, "+%d more workflow runs\n", overflow)
	}
	return strings.TrimRight(b.String(), "\n"), true
}

// decodeGHArray parses gh's top-level JSON array. Returns ok=false for any
// non-array shape (single-object gh view/status responses, HTML errors,
// empty stdout) so the caller falls through cleanly.
func decodeGHArray(stdout string) ([]map[string]any, bool) {
	trimmed := strings.TrimSpace(stdout)
	if !strings.HasPrefix(trimmed, "[") {
		return nil, false
	}
	var arr []map[string]any
	if err := json.Unmarshal([]byte(trimmed), &arr); err != nil {
		return nil, false
	}
	return arr, true
}

// ghStr coerces the value at key to a string. gh JSON mixes types (number,
// string, null); this normalizes so downstream table formatting doesn't
// have to branch. Nested objects render as their own JSON blob so we never
// silently drop signal -- callers can grep for it later.
func ghStr(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case float64:
		// JSON numbers decode as float64; format integers without decimals.
		if x == float64(int64(x)) {
			return fmt.Sprintf("%d", int64(x))
		}
		return fmt.Sprintf("%g", x)
	case bool:
		return fmt.Sprintf("%t", x)
	default:
		b, _ := json.Marshal(x)
		return string(b)
	}
}

// ghAuthor picks the login field out of the nested author object gh returns
// for pr list. If the caller requested author as a scalar (custom --json),
// we accept a bare string too.
func ghAuthor(pr map[string]any) string {
	v, ok := pr["author"]
	if !ok {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	if obj, ok := v.(map[string]any); ok {
		if login, ok := obj["login"].(string); ok {
			return login
		}
	}
	return ""
}

// Now is the time source used by renderers that surface relative ages
// ("3d" / "2h" / "45m"). Overridable so bench pinning and tests can freeze
// the clock; production callers leave it as time.Now.
var Now = time.Now

// nowFn kept as a package-internal alias so existing test helpers that
// stub it keep working. Prefer overriding Now in new callers.
var nowFn = func() time.Time { return Now() }

// ghAge renders an ISO-8601 timestamp as a compact relative age. Anything
// unparseable falls back to the raw string so we preserve provenance even
// under timezone / format drift.
func ghAge(ts string) string {
	if ts == "" {
		return "-"
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ts
	}
	d := nowFn().Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	default:
		return fmt.Sprintf("%dmo", int(d.Hours()/(24*30)))
	}
}
