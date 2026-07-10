package compact

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"
)

// AWS CLI returns deeply nested JSON by default (Reservations/Instances/etc.).
// The hand-crafted rewrites in aws.bsh cover the top ~100 verbs with tuned
// JMESPath projections, but AWS has hundreds of services and thousands of
// subcommands -- a rewrite per verb does not scale.
//
// renderAWSJSON is the generic fallback: it activates on any `aws`-invoked
// command whose output is JSON (either because the caller demanded
// --output json and tripped a rewrite guard, or because the verb has no
// tuned rewrite yet). It shapes the JSON into a compact tab-separated
// table without needing per-verb knowledge.
//
// The heuristic:
//
//  1. Locate the primary array. Almost every AWS list-*/describe-* response
//     wraps its records in exactly one field (Reservations, Instances,
//     Functions, StackSummaries, Users, Roles, Buckets, ...). Rule: pick
//     the array-valued field with the most elements, ignoring pagination
//     scalars like NextToken.
//
//  2. Pick columns. First scan the first ~10 items for scalar leaf keys
//     (dotted access into one nested level like State.Name is allowed so
//     EC2/RDS common patterns survive). Rank keys by a preferred-field
//     list (Id / Name / Arn / Status / State / *Time / *Date / Type / ...)
//     to keep the same 5-6 columns the human console shows.
//
//  3. Format tab-separated with a `[$service $verb] N results:` header
//     and a `+N more` marker when the list overflows.
//
// Any shape we do not recognize (single top-level object, deeply nested
// arrays of objects each holding another array, non-JSON output) is
// surrendered via ok=false so the script filter cascade takes over.

const (
	awsMaxRows    = 50
	awsMaxColumns = 6
	awsProbeItems = 10
)

// renderAWSJSON is the entry point matching `aws` in native.go. Returns
// ok=false whenever the output does not look like an AWS JSON response,
// letting the script filter cascade handle it.
func renderAWSJSON(stdout, stderr string, exitCode int) (string, bool) {
	trimmed := strings.TrimSpace(stdout)
	if trimmed == "" {
		return "", false
	}
	// AWS JSON responses always start with { or [. Anything else (aws help
	// text, --output text tab rows, aws s3 sync progress, an error blob) is
	// left to the script filter cascade.
	if trimmed[0] != '{' && trimmed[0] != '[' {
		return "", false
	}

	var root any
	if err := json.Unmarshal([]byte(trimmed), &root); err != nil {
		return "", false
	}

	items, key, ok := awsFindPrimaryArray(root)
	if !ok {
		return "", false
	}
	if len(items) == 0 {
		return fmt.Sprintf("no %s", awsHeaderKey(key)), true
	}

	// Scalar arrays: single-column table (aws ec2 describe-availability-zones
	// with --query, aws dynamodb list-tables, aws sqs list-queues, ...).
	if scalar, ok := awsAllScalars(items); ok {
		return awsFormatScalarArray(key, scalar), true
	}

	// Object arrays: pick columns from the first N items, project, format.
	cols := awsPickColumns(items)
	if len(cols) == 0 {
		return "", false
	}
	rows := awsProject(items, cols)
	return awsFormatTable(key, cols, rows, len(items)), true
}

// awsFindPrimaryArray walks the top level of an AWS JSON response and
// returns the array most likely to hold the records the caller wanted,
// along with its field name.
//
// The rules, in order:
//   - A top-level array is used directly (unnamed).
//   - Among the top-level object's array-valued fields, the longest one wins.
//     AWS conventionally returns records in a single field; ties are rare
//     but stable-broken by the sorted key list so output stays deterministic.
//   - No array-valued fields → not something this renderer can handle.
func awsFindPrimaryArray(root any) ([]any, string, bool) {
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

// awsAllScalars reports whether every element in items is a scalar (string
// or number or bool). AWS list-only APIs (list-tables, list-queues, ...)
// return arrays of bare strings; scalar arrays render as a single column.
func awsAllScalars(items []any) ([]string, bool) {
	out := make([]string, 0, len(items))
	for _, it := range items {
		switch v := it.(type) {
		case string:
			out = append(out, v)
		case float64:
			out = append(out, awsFormatNumber(v))
		case bool:
			out = append(out, fmt.Sprintf("%t", v))
		default:
			return nil, false
		}
	}
	return out, true
}

// awsPickColumns ranks the scalar keys of the first N items and returns
// up to awsMaxColumns of them, ordered so the most identity-carrying
// fields (Id, Name, Arn) lead.
func awsPickColumns(items []any) []string {
	// Count how many of the probed items expose each candidate key with
	// a scalar (or single-nested scalar) value.
	scores := map[string]int{}
	probe := items
	if len(probe) > awsProbeItems {
		probe = probe[:awsProbeItems]
	}
	for _, it := range probe {
		obj, ok := it.(map[string]any)
		if !ok {
			continue
		}
		for _, k := range awsScalarKeys(obj, "") {
			scores[k]++
		}
	}
	if len(scores) == 0 {
		return nil
	}

	// Deterministic order: preferred-field score first, then coverage
	// (how many probed items had the field), then key name.
	keys := make([]string, 0, len(scores))
	for k := range scores {
		keys = append(keys, k)
	}
	sort.SliceStable(keys, func(i, j int) bool {
		pi, pj := awsPreference(keys[i]), awsPreference(keys[j])
		if pi != pj {
			return pi < pj // lower rank number wins
		}
		if scores[keys[i]] != scores[keys[j]] {
			return scores[keys[i]] > scores[keys[j]]
		}
		return keys[i] < keys[j]
	})

	if len(keys) > awsMaxColumns {
		keys = keys[:awsMaxColumns]
	}
	return keys
}

// awsScalarKeys returns dotted-path keys pointing at scalar leaves in an
// AWS record. One level of nesting is allowed (State.Name, Placement.
// AvailabilityZone) because those are the most common shape in EC2 / RDS
// / Batch responses; deeper trees are skipped so we do not accidentally
// pick an id from a container we do not summarise.
func awsScalarKeys(obj map[string]any, prefix string) []string {
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
				continue // only one level of nesting
			}
			out = append(out, awsScalarKeys(child, path)...)
		}
	}
	return out
}

// awsProject reads each item's scalar leaves for the chosen columns.
// Missing values render as the empty string so column alignment stays
// deterministic; whole-row overflow is capped in awsFormatTable.
func awsProject(items []any, cols []string) [][]string {
	rows := make([][]string, 0, len(items))
	for _, it := range items {
		obj, ok := it.(map[string]any)
		if !ok {
			continue
		}
		row := make([]string, len(cols))
		for i, c := range cols {
			row[i] = awsScalarAt(obj, c)
		}
		rows = append(rows, row)
	}
	return rows
}

// awsScalarAt follows a dotted key into obj and stringifies the leaf.
// Non-scalar targets and missing paths render as "" (blank column).
func awsScalarAt(obj map[string]any, dotted string) string {
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
		return awsScalarAt(child, parts[1])
	}
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return awsFormatNumber(t)
	case bool:
		return fmt.Sprintf("%t", t)
	case nil:
		return ""
	default:
		return ""
	}
}

// awsFormatNumber prints an integer as "123" and a float as "1.5"; the
// aws JSON has no distinction so we heuristically pick the shorter form.
// Prevents InstanceCount showing up as "3.000000".
func awsFormatNumber(f float64) string {
	if f == float64(int64(f)) {
		return fmt.Sprintf("%d", int64(f))
	}
	return fmt.Sprintf("%g", f)
}

// awsHeaderKey lowercases and singularises the field name for the
// "no <thing>" empty-result line ("no functions", "no instances").
func awsHeaderKey(key string) string {
	if key == "" {
		return "results"
	}
	lower := strings.ToLower(key)
	return lower
}

// awsPreference ranks candidate column keys. Lower is better. Fields that
// mimic what the AWS console shows come first: identity, name, state,
// version, then timestamps, then everything else. This keeps the compact
// table useful even for services this file has never heard of.
func awsPreference(key string) int {
	last := key
	if i := strings.LastIndex(key, "."); i >= 0 {
		last = key[i+1:]
	}
	lower := strings.ToLower(last)

	// Identity comes first. Human-readable names beat opaque IDs -- in most
	// AWS list-* responses the *Name field is the discriminator a caller
	// would recognise (FunctionName, ClusterName, TableName), while *Id
	// values (VpcId, SubnetId, RoleId) are secondary references.
	if lower == "name" || strings.HasSuffix(lower, "name") {
		return 0
	}
	if lower == "id" {
		return 1
	}
	if strings.HasSuffix(lower, "id") {
		return 2
	}
	// Arn on its own carries the full identity; nested *Arn (RoleArn,
	// PolicyArn) is a reference to something else and ranks lower so the
	// primary Arn column beats it when both are present.
	if lower == "arn" {
		return 3
	}
	if strings.HasSuffix(lower, "arn") {
		return 9 // deprioritise reference ARNs; below timestamps
	}

	// State / status / lifecycle.
	if lower == "state" || lower == "status" {
		return 4
	}
	if strings.HasSuffix(lower, "state") || strings.HasSuffix(lower, "status") {
		return 5
	}

	// Type / kind / version.
	if lower == "type" || strings.HasSuffix(lower, "type") {
		return 6
	}
	if lower == "kind" {
		return 6
	}
	if lower == "version" || strings.HasSuffix(lower, "version") {
		return 7
	}

	// Timestamps.
	if strings.Contains(lower, "time") || strings.Contains(lower, "date") || strings.Contains(lower, "at") {
		return 8
	}

	// Everything else.
	return 20
}

// awsFormatScalarArray renders a bare-string array (list-tables, list-
// queues, ...) as a single-column table with a header derived from the
// field name.
func awsFormatScalarArray(key string, values []string) string {
	label := "value"
	if key != "" {
		label = key
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", strings.ToUpper(label))

	shown := values
	overflow := 0
	if len(shown) > awsMaxRows {
		overflow = len(shown) - awsMaxRows
		shown = shown[:awsMaxRows]
	}
	for _, v := range shown {
		fmt.Fprintln(&b, v)
	}
	if overflow > 0 {
		fmt.Fprintf(&b, "+%d more\n", overflow)
	}
	return strings.TrimRight(b.String(), "\n")
}

// awsFormatTable emits a `[$key N results]` banner + tab-separated header
// + rows, then `+N more` overflow when the array exceeds awsMaxRows.
func awsFormatTable(key string, cols []string, rows [][]string, total int) string {
	var b strings.Builder
	if key != "" {
		fmt.Fprintf(&b, "%s (%d)\n", key, total)
	}

	tw := tabwriter.NewWriter(&b, 2, 4, 2, ' ', 0)
	headers := make([]string, len(cols))
	for i, c := range cols {
		headers[i] = strings.ToUpper(awsShortHeader(c))
	}
	fmt.Fprintln(tw, strings.Join(headers, "\t"))

	shown := rows
	overflow := 0
	if len(shown) > awsMaxRows {
		overflow = len(shown) - awsMaxRows
		shown = shown[:awsMaxRows]
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

// awsShortHeader trims a dotted path down to the last component for the
// column header ("State.Name" → "Name"), keeping the table narrow while
// leaving the projection code operating on the full dotted path.
func awsShortHeader(dotted string) string {
	if i := strings.LastIndex(dotted, "."); i >= 0 {
		return dotted[i+1:]
	}
	return dotted
}
