package compact

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"
)

// Google Cloud CLI returns a padded human table by default and structured
// JSON when `--format=json`. The hand-crafted rewrites in gcloud.bsh cover
// the top ~90 verbs with tuned value() projections to tab-separated text.
// Anything else -- unfamiliar services, `--format=json` opt-ins, alpha /
// beta surfaces -- falls through to this generic renderer.
//
// renderGcloudJSON activates on any `gcloud` invocation whose output is
// JSON and shapes it into a compact tab-separated table without per-verb
// knowledge.
//
// GCP JSON shape:
//   - Most `gcloud X list --format=json` verbs return a bare top-level
//     array of records.
//   - Knative-style surfaces (Cloud Run, Cloud Functions v2, Workflows,
//     Vertex AI) bury the useful state under `metadata.name`,
//     `status.state`, `status.url`. The one-level nested-scalar rule
//     surfaces these as columns without exposing the raw sub-object.
//   - Deep buries like `spec.template.spec.containers[0].image` are
//     surrendered to the rewrite path.

const (
	gcloudMaxRows    = 50
	gcloudMaxColumns = 6
	gcloudProbeItems = 10
)

// renderGcloudJSON is the entry point matching `gcloud` in native.go.
// Returns ok=false whenever the output does not look like a GCP JSON
// response, letting the script filter cascade handle it.
func renderGcloudJSON(stdout, stderr string, exitCode int) (string, bool) {
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

	items, key, ok := gcloudFindPrimaryArray(root)
	if !ok {
		return "", false
	}
	if len(items) == 0 {
		return fmt.Sprintf("no %s", gcloudHeaderKey(key)), true
	}

	if scalar, ok := gcloudAllScalars(items); ok {
		return gcloudFormatScalarArray(key, scalar), true
	}

	cols := gcloudPickColumns(items)
	if len(cols) == 0 {
		return "", false
	}
	rows := gcloudProject(items, cols)
	return gcloudFormatTable(key, cols, rows, len(items)), true
}

// gcloudFindPrimaryArray walks the top level of a GCP JSON response and
// returns the array most likely to hold the caller's records.
//
// The rules, in order:
//   - A top-level array is used directly (unnamed) -- the shape of every
//     `gcloud * list --format=json` invocation.
//   - Among the top-level object's array-valued fields, the longest one
//     wins. Ties are stable-broken by sorted key order so output stays
//     deterministic.
//   - No array-valued fields → not something this renderer can handle.
func gcloudFindPrimaryArray(root any) ([]any, string, bool) {
	switch v := root.(type) {
	case []any:
		return v, "", true
	case map[string]any:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)

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

// gcloudAllScalars reports whether every element in items is a scalar.
// `gcloud pubsub topics list --format="value(name)"` returns arrays of
// bare strings; scalar arrays render as a single column.
func gcloudAllScalars(items []any) ([]string, bool) {
	out := make([]string, 0, len(items))
	for _, it := range items {
		switch v := it.(type) {
		case string:
			out = append(out, v)
		case float64:
			out = append(out, gcloudFormatNumber(v))
		case bool:
			out = append(out, fmt.Sprintf("%t", v))
		default:
			return nil, false
		}
	}
	return out, true
}

// gcloudPickColumns ranks the scalar keys of the first N items and returns
// up to gcloudMaxColumns of them, ordered so the most identity-carrying
// fields (name, id, state, timestamps) lead.
func gcloudPickColumns(items []any) []string {
	scores := map[string]int{}
	probe := items
	if len(probe) > gcloudProbeItems {
		probe = probe[:gcloudProbeItems]
	}
	for _, it := range probe {
		obj, ok := it.(map[string]any)
		if !ok {
			continue
		}
		for _, k := range gcloudScalarKeys(obj, "") {
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
		pi, pj := gcloudPreference(keys[i]), gcloudPreference(keys[j])
		if pi != pj {
			return pi < pj
		}
		if scores[keys[i]] != scores[keys[j]] {
			return scores[keys[i]] > scores[keys[j]]
		}
		return keys[i] < keys[j]
	})

	if len(keys) > gcloudMaxColumns {
		keys = keys[:gcloudMaxColumns]
	}
	return keys
}

// gcloudScalarKeys returns dotted-path keys pointing at scalar leaves.
// One level of nesting is allowed so `metadata.name`, `status.state` and
// `status.url` survive; deeper trees (`spec.template.spec.containers[0].
// image`) are skipped so we do not pick a field we cannot summarise.
func gcloudScalarKeys(obj map[string]any, prefix string) []string {
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
			out = append(out, gcloudScalarKeys(child, path)...)
		}
	}
	return out
}

// gcloudProject reads each item's scalar leaves for the chosen columns.
// Missing values render as the empty string so column alignment stays
// deterministic; whole-row overflow is capped in gcloudFormatTable.
func gcloudProject(items []any, cols []string) [][]string {
	rows := make([][]string, 0, len(items))
	for _, it := range items {
		obj, ok := it.(map[string]any)
		if !ok {
			continue
		}
		row := make([]string, len(cols))
		for i, c := range cols {
			row[i] = gcloudBasename(gcloudScalarAt(obj, c))
		}
		rows = append(rows, row)
	}
	return rows
}

// gcloudScalarAt follows a dotted key into obj and stringifies the leaf.
// Non-scalar targets and missing paths render as "" (blank column).
func gcloudScalarAt(obj map[string]any, dotted string) string {
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
		return gcloudScalarAt(child, parts[1])
	}
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return gcloudFormatNumber(t)
	case bool:
		return fmt.Sprintf("%t", t)
	case nil:
		return ""
	default:
		return ""
	}
}

// gcloudBasename replicates gcloud's own value(.basename()) transform.
// GCP surfaces resource URIs like
//
//	https://www.googleapis.com/compute/v1/projects/.../machineTypes/n1-standard-1
//
// where the last path segment is the only useful signal. Compact tables
// need this reduction to be readable; without it, one full URI wipes out
// most savings from the JSON→text switch.
func gcloudBasename(s string) string {
	if !strings.HasPrefix(s, "https://") && !strings.HasPrefix(s, "projects/") {
		return s
	}
	if i := strings.LastIndex(s, "/"); i >= 0 && i < len(s)-1 {
		return s[i+1:]
	}
	return s
}

// gcloudFormatNumber prints an integer as "123" and a float as "1.5".
// GCP JSON has no int/float distinction and Node counts / disk sizes
// otherwise render as "3.000000".
func gcloudFormatNumber(f float64) string {
	if f == float64(int64(f)) {
		return fmt.Sprintf("%d", int64(f))
	}
	return fmt.Sprintf("%g", f)
}

// gcloudHeaderKey returns a lowercased label for the empty-array "no
// <thing>" line ("no results" when the array is a bare top-level).
func gcloudHeaderKey(key string) string {
	if key == "" {
		return "results"
	}
	return strings.ToLower(key)
}

// gcloudPreference ranks candidate column keys. Lower is better. Fields
// that mimic what the GCP Console shows come first: identity, state,
// location, kind/type, then timestamps, then everything else. Nested
// `metadata.name`, `status.state`, `status.url` rank alongside their
// top-level twins so Knative-style records still lead with Name.
func gcloudPreference(key string) int {
	last := key
	if i := strings.LastIndex(key, "."); i >= 0 {
		last = key[i+1:]
	}
	lower := strings.ToLower(last)

	// Human-readable names beat opaque IDs.
	if lower == "name" || strings.HasSuffix(lower, "name") {
		return 0
	}
	// Email is the identity for IAM service accounts / users.
	if lower == "email" {
		return 0
	}
	// Bare id / ProjectId, ClusterName-like fields.
	if lower == "id" {
		return 1
	}
	if strings.HasSuffix(lower, "id") {
		return 2
	}
	// URL / endpoint -- Cloud Run status.url, Cloud Functions httpsTrigger.url.
	if lower == "url" {
		return 3
	}
	// State / status.
	if lower == "state" || lower == "status" || lower == "servingstatus" || lower == "lifecyclestate" {
		return 4
	}
	if strings.HasSuffix(lower, "state") || strings.HasSuffix(lower, "status") {
		return 5
	}
	// Zone / region / location.
	if lower == "zone" || lower == "region" || lower == "location" || lower == "locationid" {
		return 6
	}
	// Type / kind / SKU / tier.
	if lower == "kind" || lower == "type" || strings.HasSuffix(lower, "type") {
		return 7
	}
	if lower == "tier" || lower == "sku" {
		return 7
	}
	// Version.
	if lower == "version" || strings.HasSuffix(lower, "version") {
		return 8
	}
	// Timestamps -- creationTimestamp, updateTime, startTime.
	if strings.Contains(lower, "time") || strings.Contains(lower, "date") {
		return 9
	}
	// Everything else.
	return 20
}

// gcloudFormatScalarArray renders a bare-string array as a single-column
// table with a header derived from the field name (or "VALUE" for a
// top-level array).
func gcloudFormatScalarArray(key string, values []string) string {
	label := "value"
	if key != "" {
		label = key
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", strings.ToUpper(label))

	shown := values
	overflow := 0
	if len(shown) > gcloudMaxRows {
		overflow = len(shown) - gcloudMaxRows
		shown = shown[:gcloudMaxRows]
	}
	for _, v := range shown {
		fmt.Fprintln(&b, gcloudBasename(v))
	}
	if overflow > 0 {
		fmt.Fprintf(&b, "+%d more\n", overflow)
	}
	return strings.TrimRight(b.String(), "\n")
}

// gcloudFormatTable emits a `<key> (N)` banner (omitted for a bare
// top-level array), a tab-separated header, then rows, then `+N more`
// overflow when the array exceeds gcloudMaxRows.
func gcloudFormatTable(key string, cols []string, rows [][]string, total int) string {
	var b strings.Builder
	if key != "" {
		fmt.Fprintf(&b, "%s (%d)\n", key, total)
	} else {
		fmt.Fprintf(&b, "%d results\n", total)
	}

	tw := tabwriter.NewWriter(&b, 2, 4, 2, ' ', 0)
	headers := make([]string, len(cols))
	for i, c := range cols {
		headers[i] = strings.ToUpper(gcloudShortHeader(c))
	}
	fmt.Fprintln(tw, strings.Join(headers, "\t"))

	shown := rows
	overflow := 0
	if len(shown) > gcloudMaxRows {
		overflow = len(shown) - gcloudMaxRows
		shown = shown[:gcloudMaxRows]
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

// gcloudShortHeader trims a dotted path down to the last component for
// the column header ("metadata.name" → "name"), keeping the table narrow
// while the projection code still walks the full dotted path to fetch
// values.
func gcloudShortHeader(dotted string) string {
	if i := strings.LastIndex(dotted, "."); i >= 0 {
		return dotted[i+1:]
	}
	return dotted
}
