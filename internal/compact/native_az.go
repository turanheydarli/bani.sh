package compact

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"
)

// Azure CLI returns nested JSON by default. The hand-crafted rewrites in
// az.bsh cover the top ~100 verbs with tuned JMESPath (--query) projections,
// but Azure has 30+ service groups and hundreds of subcommands; a rewrite
// per verb does not scale.
//
// renderAzJSON is the generic fallback: it activates on any `az`-invoked
// command whose output is JSON (either because the caller asked for
// --output json/jsonc explicitly and tripped a rewrite guard, or because
// the verb has no tuned rewrite yet). It shapes the JSON into a compact
// tab-separated table without per-verb knowledge.
//
// Azure JSON shape versus AWS:
//   - Most `az * list` verbs return a bare top-level array of records
//     (not an object wrapping an array). renderAzJSON handles both.
//   - Records commonly bury the useful fields under a `properties`
//     sub-object (`.properties.provisioningState`, `.properties.state`,
//     `.properties.hardwareProfile.vmSize`). The one-level nested-scalar
//     rule catches `.properties.X` for scalar X; two-level buries
//     (`.properties.hardwareProfile.vmSize`) are surrendered to the
//     rewrite path.
//
// Any shape we do not recognise (single top-level object, deeply nested
// arrays of records each holding another array, non-JSON output) is
// surrendered via ok=false so the script filter cascade takes over.

const (
	azMaxRows    = 50
	azMaxColumns = 6
	azProbeItems = 10
)

// renderAzJSON is the entry point matching `az` in native.go. Returns
// ok=false whenever the output does not look like an Azure JSON response,
// letting the script filter cascade handle it.
func renderAzJSON(stdout, stderr string, exitCode int) (string, bool) {
	trimmed := strings.TrimSpace(stdout)
	if trimmed == "" {
		return "", false
	}
	if trimmed[0] != '{' && trimmed[0] != '[' {
		return "", false
	}

	var root any
	if err := json.Unmarshal([]byte(trimmed), &root); err != nil {
		return "", false
	}

	items, key, ok := azFindPrimaryArray(root)
	if !ok {
		return "", false
	}
	if len(items) == 0 {
		return fmt.Sprintf("no %s", azHeaderKey(key)), true
	}

	if scalar, ok := azAllScalars(items); ok {
		return azFormatScalarArray(key, scalar), true
	}

	cols := azPickColumns(items)
	if len(cols) == 0 {
		return "", false
	}
	rows := azProject(items, cols)
	return azFormatTable(key, cols, rows, len(items)), true
}

// azFindPrimaryArray walks the top level of an Azure JSON response and
// returns the array most likely to hold the caller's records, along with
// its field name.
//
// The rules, in order:
//   - A top-level array is used directly (unnamed) -- the most common Azure
//     shape for `az * list`.
//   - Among the top-level object's array-valued fields, the longest one
//     wins. Tied lengths are stable-broken by sorted key order so output
//     stays deterministic across runs.
//   - No array-valued fields → not something this renderer can handle.
func azFindPrimaryArray(root any) ([]any, string, bool) {
	switch v := root.(type) {
	case []any:
		return v, "", true
	case map[string]any:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys) // deterministic tie-break

		var bestKey string
		var bestArr []any
		found := false
		for _, k := range keys {
			arr, ok := v[k].([]any)
			if !ok {
				continue
			}
			if !found || len(arr) > len(bestArr) {
				bestKey = k
				bestArr = arr
				found = true
			}
		}
		if !found {
			return nil, "", false
		}
		return bestArr, bestKey, true
	}
	return nil, "", false
}

// azAllScalars reports whether every element in items is a scalar (string,
// number or bool). `az acr repository list` and similar Azure list-only
// verbs return arrays of bare strings; scalar arrays render as a single
// column.
func azAllScalars(items []any) ([]string, bool) {
	out := make([]string, 0, len(items))
	for _, it := range items {
		switch v := it.(type) {
		case string:
			out = append(out, v)
		case float64:
			out = append(out, azFormatNumber(v))
		case bool:
			out = append(out, fmt.Sprintf("%t", v))
		default:
			return nil, false
		}
	}
	return out, true
}

// azPickColumns ranks the scalar keys of the first N items and returns
// up to azMaxColumns of them, ordered so the most identity-carrying
// fields (name, Id, location, state, timestamps) lead.
func azPickColumns(items []any) []string {
	scores := map[string]int{}
	probe := items
	if len(probe) > azProbeItems {
		probe = probe[:azProbeItems]
	}
	for _, it := range probe {
		obj, ok := it.(map[string]any)
		if !ok {
			continue
		}
		for _, k := range azScalarKeys(obj, "") {
			scores[k]++
		}
	}
	if len(scores) == 0 {
		return nil
	}

	keys := make([]string, 0, len(scores))
	for k := range scores {
		keys = append(keys, k)
	}
	sort.SliceStable(keys, func(i, j int) bool {
		pi, pj := azPreference(keys[i]), azPreference(keys[j])
		if pi != pj {
			return pi < pj
		}
		if scores[keys[i]] != scores[keys[j]] {
			return scores[keys[i]] > scores[keys[j]]
		}
		return keys[i] < keys[j]
	})

	if len(keys) > azMaxColumns {
		keys = keys[:azMaxColumns]
	}
	return keys
}

// azScalarKeys returns dotted-path keys pointing at scalar leaves. One
// level of nesting is allowed so `properties.provisioningState` and
// `sku.name` survive; deeper trees (`properties.hardwareProfile.vmSize`)
// are skipped so we do not pick a field we cannot summarise.
func azScalarKeys(obj map[string]any, prefix string) []string {
	var out []string
	for k, v := range obj {
		path := k
		if prefix != "" {
			path = prefix + "." + k
		}
		switch child := v.(type) {
		case string, float64, bool, nil:
			out = append(out, path)
		case map[string]any:
			if prefix != "" {
				continue
			}
			out = append(out, azScalarKeys(child, path)...)
		}
	}
	return out
}

// azProject reads each item's scalar leaves for the chosen columns.
// Missing values render as the empty string so column alignment stays
// deterministic; whole-row overflow is capped in azFormatTable.
func azProject(items []any, cols []string) [][]string {
	rows := make([][]string, 0, len(items))
	for _, it := range items {
		obj, ok := it.(map[string]any)
		if !ok {
			continue
		}
		row := make([]string, len(cols))
		for i, c := range cols {
			row[i] = azScalarAt(obj, c)
		}
		rows = append(rows, row)
	}
	return rows
}

// azScalarAt follows a dotted key into obj and stringifies the leaf.
// Non-scalar targets and missing paths render as "" (blank column).
func azScalarAt(obj map[string]any, dotted string) string {
	parts := strings.SplitN(dotted, ".", 2)
	v, ok := obj[parts[0]]
	if !ok {
		return ""
	}
	if len(parts) == 2 {
		child, ok := v.(map[string]any)
		if !ok {
			return ""
		}
		return azScalarAt(child, parts[1])
	}
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return azFormatNumber(t)
	case bool:
		return fmt.Sprintf("%t", t)
	case nil:
		return ""
	default:
		return ""
	}
}

// azFormatNumber prints an integer as "123" and a float as "1.5"; the
// az JSON has no int/float distinction so we pick the shorter form to
// avoid "3.000000" showing up in Count columns.
func azFormatNumber(f float64) string {
	if f == float64(int64(f)) {
		return fmt.Sprintf("%d", int64(f))
	}
	return fmt.Sprintf("%g", f)
}

// azHeaderKey returns a lowercased fallback label for the empty-array
// "no <thing>" line ("no results" when the array is a bare top-level).
func azHeaderKey(key string) string {
	if key == "" {
		return "results"
	}
	return strings.ToLower(key)
}

// azPreference ranks candidate column keys. Lower is better. Fields that
// mimic what the Azure Portal shows come first: identity, name, location,
// state, sku, then timestamps, then everything else. Nested `properties.X`
// columns rank the same as their top-level twins so `properties.
// provisioningState` still lands near the primary Status column.
func azPreference(key string) int {
	last := key
	if i := strings.LastIndex(key, "."); i >= 0 {
		last = key[i+1:]
	}
	lower := strings.ToLower(last)

	// Human-readable names beat opaque IDs (name, displayName, hostName).
	if lower == "name" || strings.HasSuffix(lower, "name") {
		return 0
	}
	// Bare ID / any ...Id (appId, subscriptionId, resourceGroupId).
	if lower == "id" {
		return 1
	}
	if strings.HasSuffix(lower, "id") {
		return 2
	}
	// Azure records live inside a resource group and a region; both are
	// as identifying as the name for the caller.
	if lower == "resourcegroup" {
		return 3
	}
	if lower == "location" {
		return 4
	}
	// State / status / lifecycle.
	if lower == "state" || lower == "status" || lower == "provisioningstate" {
		return 5
	}
	if strings.HasSuffix(lower, "state") || strings.HasSuffix(lower, "status") {
		return 6
	}
	// Kind / type / SKU / tier / plan.
	if lower == "kind" || lower == "type" || strings.HasSuffix(lower, "type") {
		return 7
	}
	if lower == "sku" || strings.HasSuffix(lower, "sku") || lower == "tier" {
		return 7
	}
	// Version.
	if lower == "version" || strings.HasSuffix(lower, "version") {
		return 8
	}
	// Timestamps.
	if strings.Contains(lower, "time") || strings.Contains(lower, "date") || strings.Contains(lower, "modified") {
		return 9
	}
	// Everything else.
	return 20
}

// azFormatScalarArray renders a bare-string array (list-tables-shaped
// output like `az acr repository list`) as a single-column table with a
// header derived from the field name (or "VALUE" for a top-level array).
func azFormatScalarArray(key string, values []string) string {
	label := "value"
	if key != "" {
		label = key
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", strings.ToUpper(label))

	shown := values
	overflow := 0
	if len(shown) > azMaxRows {
		overflow = len(shown) - azMaxRows
		shown = shown[:azMaxRows]
	}
	for _, v := range shown {
		fmt.Fprintln(&b, v)
	}
	if overflow > 0 {
		fmt.Fprintf(&b, "+%d more\n", overflow)
	}
	return strings.TrimRight(b.String(), "\n")
}

// azFormatTable emits a `<key> (N)` banner (omitted for a bare top-level
// array), a tab-separated header, then rows, then `+N more` overflow when
// the array exceeds azMaxRows.
func azFormatTable(key string, cols []string, rows [][]string, total int) string {
	var b strings.Builder
	if key != "" {
		fmt.Fprintf(&b, "%s (%d)\n", key, total)
	} else {
		fmt.Fprintf(&b, "%d results\n", total)
	}

	tw := tabwriter.NewWriter(&b, 2, 4, 2, ' ', 0)
	headers := make([]string, len(cols))
	for i, c := range cols {
		headers[i] = strings.ToUpper(azShortHeader(c))
	}
	fmt.Fprintln(tw, strings.Join(headers, "\t"))

	shown := rows
	overflow := 0
	if len(shown) > azMaxRows {
		overflow = len(shown) - azMaxRows
		shown = shown[:azMaxRows]
	}
	for _, r := range shown {
		fmt.Fprintln(tw, strings.Join(r, "\t"))
	}
	tw.Flush()

	if overflow > 0 {
		fmt.Fprintf(&b, "+%d more\n", overflow)
	}
	return strings.TrimRight(b.String(), "\n")
}

// azShortHeader trims a dotted path down to the last component for the
// column header ("properties.provisioningState" → "provisioningState"),
// keeping the table narrow while the projection code still walks the
// full dotted path to fetch values.
func azShortHeader(dotted string) string {
	if i := strings.LastIndex(dotted, "."); i >= 0 {
		return dotted[i+1:]
	}
	return dotted
}
