package compact

import (
	"strings"
	"testing"
)

// TestAzJSONRenderNonJSONPassesThrough guarantees the renderer only fires
// on JSON output. The rewrites in az.bsh convert most list-* calls to
// --output tsv, so the renderer must surrender for those; otherwise it
// would fight with the per-service script filter cascade.
func TestAzJSONRenderNonJSONPassesThrough(t *testing.T) {
	cases := []string{
		"vm-1\trunning\teastus\n",
		"",
		"help usage line",
		"myrepo\ntag1",
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			if _, ok := renderAzJSON(c, "", 0); ok {
				t.Errorf("renderer accepted non-JSON input %q", c)
			}
		})
	}
}

// TestAzJSONRenderRootArray covers the most common Azure shape: `az X
// list` returns a bare top-level array. The banner reads "N results"
// since there is no field name to surface.
func TestAzJSONRenderRootArray(t *testing.T) {
	raw := `[
        {"name": "rg-prod",    "location": "eastus"},
        {"name": "rg-staging", "location": "westus2"},
        {"name": "rg-dev",     "location": "northeurope"}
    ]`
	out, ok := renderAzJSON(raw, "", 0)
	if !ok {
		t.Fatal("renderer rejected top-level array")
	}
	if !strings.Contains(out, "3 results") {
		t.Errorf("bare-array header missing:\n%s", out)
	}
	if !strings.Contains(out, "rg-prod") || !strings.Contains(out, "eastus") {
		t.Errorf("row lost:\n%s", out)
	}
}

// TestAzJSONRenderPropertiesNesting drives the `.properties.X` heuristic
// that is central to Azure -- most useful state hides one level deep and
// the renderer must reach through it without exposing the raw map.
func TestAzJSONRenderPropertiesNesting(t *testing.T) {
	raw := `[
        {"name": "vm-1", "location": "eastus",  "properties": {"provisioningState": "Succeeded", "vmSize": "Standard_DS2_v2"}},
        {"name": "vm-2", "location": "eastus",  "properties": {"provisioningState": "Failed",    "vmSize": "Standard_DS1_v2"}},
        {"name": "vm-3", "location": "westus2", "properties": {"provisioningState": "Updating",  "vmSize": "Standard_DS3_v2"}}
    ]`
	out, ok := renderAzJSON(raw, "", 0)
	if !ok {
		t.Fatal("renderer rejected properties-nested shape")
	}
	// The state field lives at properties.provisioningState but the short
	// header should drop the prefix.
	if !strings.Contains(out, "PROVISIONINGSTATE") {
		t.Errorf("nested column header not shortened:\n%s", out)
	}
	// Values from the nested field survive.
	for _, want := range []string{"Succeeded", "Failed", "Updating"} {
		if !strings.Contains(out, want) {
			t.Errorf("nested value %q lost:\n%s", want, out)
		}
	}
}

// TestAzJSONRenderScalarArray covers the `az acr repository list` shape:
// a top-level array of bare strings collapses to a single-column table.
func TestAzJSONRenderScalarArray(t *testing.T) {
	raw := `["nginx", "web-api", "worker", "migrations"]`
	out, ok := renderAzJSON(raw, "", 0)
	if !ok {
		t.Fatal("renderer rejected scalar-array shape")
	}
	for _, want := range []string{"nginx", "web-api", "worker", "migrations"} {
		if !strings.Contains(out, want) {
			t.Errorf("value %q lost:\n%s", want, out)
		}
	}
}

// TestAzJSONRenderNamePreferenceWins guards column ordering: Name (rank 0)
// must beat Id (rank 1/2), Location (rank 4), and any secondary field.
// This is the difference between a useful compact row and a noisy one.
func TestAzJSONRenderNamePreferenceWins(t *testing.T) {
	raw := `[
        {"id": "/subs/xxx/rgs/prod/providers/Microsoft.Web/sites/api", "name": "api", "location": "eastus", "kind": "app,linux"}
    ]`
	out, ok := renderAzJSON(raw, "", 0)
	if !ok {
		t.Fatal("renderer rejected input")
	}
	// tabwriter expands tabs to spaces on flush, so match against the
	// space-padded header row. NAME is the primary identifier and must
	// appear before ID.
	nameIdx := strings.Index(out, "NAME")
	idIdx := strings.Index(out, " ID ")
	if nameIdx < 0 || idIdx < 0 || nameIdx > idIdx {
		t.Errorf("NAME should precede ID in the header row:\n%s", out)
	}
}

// TestAzJSONRenderRowCap enforces the 50-row overflow marker: without it,
// pathological list responses (thousands of resources returned by a broad
// `az resource list`) would swamp the caller's context even after the
// JSON→text switch.
func TestAzJSONRenderRowCap(t *testing.T) {
	var b strings.Builder
	b.WriteString(`[`)
	for i := 0; i < 120; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`{"name":"item-`)
		b.WriteString(azItoaFast(i))
		b.WriteString(`","location":"eastus"}`)
	}
	b.WriteString(`]`)
	out, ok := renderAzJSON(b.String(), "", 0)
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

// TestAzJSONRenderEmptyArray covers the "no results" branch.
func TestAzJSONRenderEmptyArray(t *testing.T) {
	out, ok := renderAzJSON(`[]`, "", 0)
	if !ok {
		t.Fatal("renderer rejected empty top-level array")
	}
	if !strings.Contains(out, "no results") {
		t.Errorf("empty-array marker missing:\n%s", out)
	}
}

// TestAzJSONRenderPrefersLargestArray guards the picker: an Azure paged
// response commonly bundles a nextLink or empty side list next to the
// primary array. The picker must land on the substantive one.
func TestAzJSONRenderPrefersLargestArray(t *testing.T) {
	raw := `{
        "nextLink": "https://management.azure.com/...&$skiptoken=...",
        "empty": [],
        "value": [
            {"name": "kv-1", "location": "eastus"},
            {"name": "kv-2", "location": "eastus"},
            {"name": "kv-3", "location": "westus2"}
        ]
    }`
	out, ok := renderAzJSON(raw, "", 0)
	if !ok {
		t.Fatal("renderer rejected paged shape")
	}
	if !strings.Contains(out, "value (3)") {
		t.Errorf("picker landed on the wrong field:\n%s", out)
	}
}

// azItoaFast avoids strconv import churn in a table-driven test.
func azItoaFast(n int) string {
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
