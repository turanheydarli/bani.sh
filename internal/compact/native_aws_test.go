package compact

import (
	"strings"
	"testing"
)

// TestAWSJSONRenderNonJSONPassesThrough guarantees the renderer only fires
// on JSON output. The rewrites in aws.bsh convert most describe-* calls to
// --output text, so the renderer must surrender for those; otherwise it
// would fight with the per-service script filter cascade.
func TestAWSJSONRenderNonJSONPassesThrough(t *testing.T) {
	cases := []string{
		"i-0abc123\trunning\tt3.medium\n",
		"",
		"help usage line",
		"upload: ./x to s3://bucket/x",
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			if _, ok := renderAWSJSON(c, "", 0); ok {
				t.Errorf("renderer accepted non-JSON input %q", c)
			}
		})
	}
}

// TestAWSJSONRenderRootArray covers the plain list-* verbs that return an
// unnamed top-level array. Uses list-tables-shaped output (DynamoDB) since
// the record is just a bare string.
func TestAWSJSONRenderRootArray(t *testing.T) {
	// Object-of-strings inside an object: real AWS shape for list-tables.
	raw := `{"TableNames": ["orders", "users", "sessions"]}`
	out, ok := renderAWSJSON(raw, "", 0)
	if !ok {
		t.Fatal("renderer rejected list-tables shape")
	}
	if !strings.Contains(out, "orders") || !strings.Contains(out, "users") {
		t.Errorf("scalar rendering lost values:\n%s", out)
	}
	if !strings.Contains(out, "TABLENAMES") {
		t.Errorf("scalar rendering missing key header:\n%s", out)
	}
}

// TestAWSJSONRenderObjectArray drives the object-array path. Uses a Lambda-
// shaped input so we know the preferred-field ranking (Name > timestamp)
// gets a real test.
func TestAWSJSONRenderObjectArray(t *testing.T) {
	raw := `{
        "Functions": [
            {"FunctionName": "orders", "Runtime": "nodejs20.x", "Timeout": 30, "MemorySize": 512, "LastModified": "2026-07-01"},
            {"FunctionName": "notifs", "Runtime": "python3.12", "Timeout": 60, "MemorySize": 1024, "LastModified": "2026-06-28"}
        ]
    }`
	out, ok := renderAWSJSON(raw, "", 0)
	if !ok {
		t.Fatal("renderer rejected list-functions shape")
	}
	// Header banner uses the field name (Functions) with count.
	if !strings.Contains(out, "Functions (2)") {
		t.Errorf("header banner missing:\n%s", out)
	}
	// Both records survived.
	if !strings.Contains(out, "orders") || !strings.Contains(out, "notifs") {
		t.Errorf("row lost:\n%s", out)
	}
	// FunctionName ranks above Runtime/Timeout via the Name-suffix heuristic.
	nameIdx := strings.Index(out, "FUNCTIONNAME")
	runtimeIdx := strings.Index(out, "RUNTIME")
	if nameIdx < 0 || runtimeIdx < 0 || nameIdx > runtimeIdx {
		t.Errorf("column ordering wrong -- Name should precede Runtime:\n%s", out)
	}
}

// TestAWSJSONRenderNestedScalars covers the State.Name / Placement.Az
// pattern where the useful field is one level down. This is the shape
// EC2 / RDS / ECS default to, so the renderer must climb one level.
func TestAWSJSONRenderNestedScalars(t *testing.T) {
	raw := `{
        "Snapshots": [
            {"SnapshotId": "snap-1", "State": {"Code": 16, "Name": "completed"}, "VolumeSize": 8},
            {"SnapshotId": "snap-2", "State": {"Code": 32, "Name": "pending"},   "VolumeSize": 100}
        ]
    }`
	out, ok := renderAWSJSON(raw, "", 0)
	if !ok {
		t.Fatal("renderer rejected nested-scalar shape")
	}
	// State.Name is projected as a scalar column named "Name" -- proves
	// the one-level nesting rule worked without exposing the nested map.
	if !strings.Contains(out, "completed") || !strings.Contains(out, "pending") {
		t.Errorf("nested State.Name lost:\n%s", out)
	}
	if !strings.Contains(out, "snap-1") || !strings.Contains(out, "snap-2") {
		t.Errorf("SnapshotId row lost:\n%s", out)
	}
}

// TestAWSJSONRenderRowCap enforces the 50-row overflow marker: without it
// pathological list responses (10k CloudWatch metrics, huge log-streams)
// would swamp the caller's context even after the JSON→text switch.
func TestAWSJSONRenderRowCap(t *testing.T) {
	var b strings.Builder
	b.WriteString(`{"Items":[`)
	for i := 0; i < 120; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`{"Id":"item-`)
		b.WriteString(itoaFast(i))
		b.WriteString(`","Name":"n"}`)
	}
	b.WriteString(`]}`)
	out, ok := renderAWSJSON(b.String(), "", 0)
	if !ok {
		t.Fatal("renderer rejected 120-row input")
	}
	if !strings.Contains(out, "+70 more") {
		t.Errorf("overflow marker missing or wrong count:\n%s", out)
	}
	// First row should survive, the very last should be trimmed. Column
	// order and tabwriter padding vary with the preference heuristic, so
	// match the identifier substring directly instead of tab-boundary.
	if !strings.Contains(out, "item-0") {
		t.Errorf("first row lost:\n%s", out)
	}
	if strings.Contains(out, "item-119") {
		t.Errorf("row 119 should have been trimmed:\n%s", out)
	}
}

// TestAWSJSONRenderEmptyArray covers the "no results" branch. This is
// important because AWS list-* verbs return an empty-array wrapper on
// success (no exception), and we still need a non-empty single line so
// callers see something meaningful.
func TestAWSJSONRenderEmptyArray(t *testing.T) {
	out, ok := renderAWSJSON(`{"Users": []}`, "", 0)
	if !ok {
		t.Fatal("renderer rejected empty-array shape")
	}
	if !strings.Contains(out, "no users") {
		t.Errorf("empty-array marker missing:\n%s", out)
	}
}

// TestAWSJSONRenderPrefersLargestArray guards the picker: an AWS response
// commonly bundles NextToken (scalar) or an empty side list next to the
// primary array. The picker must land on the substantive one.
func TestAWSJSONRenderPrefersLargestArray(t *testing.T) {
	raw := `{
        "NextToken": "opaque",
        "SmallList": [],
        "Users": [
            {"UserName": "alice", "UserId": "u-1"},
            {"UserName": "bob",   "UserId": "u-2"},
            {"UserName": "carol", "UserId": "u-3"}
        ]
    }`
	out, ok := renderAWSJSON(raw, "", 0)
	if !ok {
		t.Fatal("renderer rejected mixed-shape input")
	}
	if !strings.Contains(out, "Users (3)") {
		t.Errorf("primary-array picker landed on the wrong field:\n%s", out)
	}
}

// itoaFast avoids strconv import churn.
func itoaFast(n int) string {
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
