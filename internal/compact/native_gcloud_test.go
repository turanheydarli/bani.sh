package compact

import (
	"strings"
	"testing"
)

// TestGcloudJSONRenderNonJSONPassesThrough guarantees the renderer only
// fires on JSON output. The rewrites in gcloud.bsh convert most `list`
// calls to `--format="value(...)"` (bare tab-separated text), so the
// renderer must surrender for those; otherwise it would fight with the
// per-service script filter cascade.
func TestGcloudJSONRenderNonJSONPassesThrough(t *testing.T) {
	cases := []string{
		"prod-web\tus-central1-a\tn1-standard-1\tRUNNING\n",
		"",
		"Copying gs://bucket/file...",
		"Updated property [core/project].",
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			if _, ok := renderGcloudJSON(c, "", 0); ok {
				t.Errorf("renderer accepted non-JSON input %q", c)
			}
		})
	}
}

// TestGcloudJSONRenderRootArray covers the shape used by every `gcloud X
// list --format=json`: a bare top-level array. Banner reads "N results"
// since there is no field name to surface.
func TestGcloudJSONRenderRootArray(t *testing.T) {
	raw := `[
        {"projectId": "prod-analytics-1234", "name": "prod-analytics", "projectNumber": "111111"},
        {"projectId": "prod-web-5678",       "name": "prod-web",       "projectNumber": "222222"},
        {"projectId": "staging-9012",        "name": "staging",        "projectNumber": "333333"}
    ]`
	out, ok := renderGcloudJSON(raw, "", 0)
	if !ok {
		t.Fatal("renderer rejected top-level array")
	}
	if !strings.Contains(out, "3 results") {
		t.Errorf("bare-array header missing:\n%s", out)
	}
	if !strings.Contains(out, "prod-analytics") || !strings.Contains(out, "staging") {
		t.Errorf("row lost:\n%s", out)
	}
}

// TestGcloudJSONRenderMetadataNesting drives the `metadata.name` /
// `status.state` heuristic central to Knative-style GCP surfaces (Cloud
// Run, Cloud Functions v2, Workflows, Vertex AI). The renderer must
// reach through the sub-object without exposing the raw map.
func TestGcloudJSONRenderMetadataNesting(t *testing.T) {
	raw := `[
        {"metadata": {"name": "orders-api",   "namespace": "prod"}, "status": {"url": "https://orders-api-abc.a.run.app",   "conditions": []}},
        {"metadata": {"name": "notifs-fanout","namespace": "prod"}, "status": {"url": "https://notifs-fanout-def.a.run.app","conditions": []}},
        {"metadata": {"name": "docs-site",    "namespace": "prod"}, "status": {"url": "https://docs-site-ghi.a.run.app",    "conditions": []}}
    ]`
	out, ok := renderGcloudJSON(raw, "", 0)
	if !ok {
		t.Fatal("renderer rejected metadata-nested shape")
	}
	// Short header on the nested field.
	if !strings.Contains(out, "NAME") || !strings.Contains(out, "URL") {
		t.Errorf("nested column header not shortened:\n%s", out)
	}
	// Values from the nested fields survive.
	for _, want := range []string{"orders-api", "notifs-fanout", "orders-api-abc.a.run.app"} {
		if !strings.Contains(out, want) {
			t.Errorf("nested value %q lost:\n%s", want, out)
		}
	}
}

// TestGcloudJSONRenderBasenameStripsURIs proves the .basename() equivalent
// runs on projected values. GCP resource URIs are the biggest per-cell
// noise source in Compute output; without this reduction the JSON→text
// win collapses on the first machineType or network column.
func TestGcloudJSONRenderBasenameStripsURIs(t *testing.T) {
	raw := `[
        {
            "name": "prod-web-1",
            "zone": "https://www.googleapis.com/compute/v1/projects/p/zones/us-central1-a",
            "machineType": "https://www.googleapis.com/compute/v1/projects/p/zones/us-central1-a/machineTypes/n1-standard-4",
            "status": "RUNNING"
        }
    ]`
	out, ok := renderGcloudJSON(raw, "", 0)
	if !ok {
		t.Fatal("renderer rejected input")
	}
	// The full URI must NOT appear -- basename must have stripped it.
	if strings.Contains(out, "googleapis.com") {
		t.Errorf("resource URI leaked into table (basename did not fire):\n%s", out)
	}
	// The last segment DID land in the row.
	for _, want := range []string{"us-central1-a", "n1-standard-4"} {
		if !strings.Contains(out, want) {
			t.Errorf("basename-projected value %q missing:\n%s", want, out)
		}
	}
}

// TestGcloudJSONRenderScalarArray covers the `gcloud pubsub topics list
// --format="value(name)"` shape: an array of bare strings collapses to a
// single-column table.
func TestGcloudJSONRenderScalarArray(t *testing.T) {
	raw := `[
        "projects/p/topics/orders-events",
        "projects/p/topics/notifications",
        "projects/p/topics/audit-log"
    ]`
	out, ok := renderGcloudJSON(raw, "", 0)
	if !ok {
		t.Fatal("renderer rejected scalar-array shape")
	}
	// basename() reduction runs on scalar values too so the last path
	// segment lands in the table.
	for _, want := range []string{"orders-events", "notifications", "audit-log"} {
		if !strings.Contains(out, want) {
			t.Errorf("basename-projected scalar %q missing:\n%s", want, out)
		}
	}
	if strings.Contains(out, "projects/p/topics/") {
		t.Errorf("full topic URI leaked -- basename did not fire on scalars:\n%s", out)
	}
}

// TestGcloudJSONRenderNamePreferenceWins guards column ordering: Name
// (rank 0) must beat Id (rank 1), State (rank 4), and anything else.
// Otherwise the first column is an opaque number and the table stops
// being scannable.
func TestGcloudJSONRenderNamePreferenceWins(t *testing.T) {
	raw := `[
        {"id": "job-1234abcd", "name": "nightly-etl", "state": "SUCCEEDED", "type": "DATAFLOW"}
    ]`
	out, ok := renderGcloudJSON(raw, "", 0)
	if !ok {
		t.Fatal("renderer rejected input")
	}
	nameIdx := strings.Index(out, "NAME")
	idIdx := strings.Index(out, " ID ")
	if nameIdx < 0 || idIdx < 0 || nameIdx > idIdx {
		t.Errorf("NAME should precede ID in the header row:\n%s", out)
	}
}

// TestGcloudJSONRenderRowCap enforces the 50-row overflow marker.
// Without it, pathological list responses (thousands of logs, all
// Compute instances across every zone) would swamp the caller's context.
func TestGcloudJSONRenderRowCap(t *testing.T) {
	var b strings.Builder
	b.WriteString(`[`)
	for i := 0; i < 120; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`{"name":"item-`)
		b.WriteString(gcloudItoaFast(i))
		b.WriteString(`","zone":"us-central1-a"}`)
	}
	b.WriteString(`]`)
	out, ok := renderGcloudJSON(b.String(), "", 0)
	if !ok {
		t.Fatal("renderer rejected 120-row input")
	}
	if !strings.Contains(out, "+70 more") {
		t.Errorf("overflow marker missing or wrong count:\n%s", out)
	}
	if !strings.Contains(out, "item-0") {
		t.Errorf("first row lost:\n%s", out)
	}
	if strings.Contains(out, "item-119") {
		t.Errorf("row 119 should have been trimmed:\n%s", out)
	}
}

// TestGcloudJSONRenderEmptyArray covers the "no results" branch.
func TestGcloudJSONRenderEmptyArray(t *testing.T) {
	out, ok := renderGcloudJSON(`[]`, "", 0)
	if !ok {
		t.Fatal("renderer rejected empty top-level array")
	}
	if !strings.Contains(out, "no results") {
		t.Errorf("empty-array marker missing:\n%s", out)
	}
}

// TestGcloudJSONRenderPrefersLargestArray guards the picker when a paged
// response bundles nextPageToken or empty side-arrays next to the
// primary data.
func TestGcloudJSONRenderPrefersLargestArray(t *testing.T) {
	raw := `{
        "nextPageToken": "opaque",
        "warnings": [],
        "items": [
            {"name": "vm-1", "status": "RUNNING"},
            {"name": "vm-2", "status": "RUNNING"},
            {"name": "vm-3", "status": "STOPPED"}
        ]
    }`
	out, ok := renderGcloudJSON(raw, "", 0)
	if !ok {
		t.Fatal("renderer rejected paged shape")
	}
	if !strings.Contains(out, "items (3)") {
		t.Errorf("picker landed on the wrong field:\n%s", out)
	}
}

// TestGcloudBasenameSpecialCases covers the basename helper directly so
// its contract stays documented at unit level, not just via integration
// with the renderer.
func TestGcloudBasenameSpecialCases(t *testing.T) {
	cases := []struct {
		in, out string
	}{
		{"https://www.googleapis.com/compute/v1/projects/p/zones/us-central1-a", "us-central1-a"},
		{"projects/p/topics/orders", "orders"},
		{"plain-value", "plain-value"},
		{"", ""},
		{"projects/p/", "projects/p/"}, // trailing slash: no reduction
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := gcloudBasename(tc.in); got != tc.out {
				t.Errorf("gcloudBasename(%q) = %q, want %q", tc.in, got, tc.out)
			}
		})
	}
}

// gcloudItoaFast avoids strconv import churn in a table-driven test.
func gcloudItoaFast(n int) string {
	if n == 0 {
		return "0"
	}
	buf := make([]byte, 0, 4)
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	return string(buf)
}
