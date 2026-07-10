package extension

import (
	"strings"
	"testing"

	"go.banish.sh/banish/internal/compact"
)

// loadAzPack parses the embedded az.bsh once for the Azure pack tests.
// All tests exercise the shipped source so any drift in the pack surfaces
// here alongside the bench golden diff.
func loadAzPack(t *testing.T) *Loader {
	t.Helper()
	builtins, err := Builtin()
	if err != nil {
		t.Fatal(err)
	}
	src, ok := builtins["az.bsh"]
	if !ok {
		t.Fatal("no embedded az.bsh")
	}
	l := NewLoader()
	if err := l.LoadSource("az.bsh", src); err != nil {
		t.Fatal(err)
	}
	return l
}

func findAzRewrite(t *testing.T, l *Loader, name string) RewriteDef {
	t.Helper()
	for _, rw := range l.Rewrites() {
		if rw.Name == name {
			return rw
		}
	}
	t.Fatalf("rewrite %q not registered by az.bsh", name)
	return RewriteDef{}
}

func findAzFilter(t *testing.T, l *Loader, name string) FilterDef {
	t.Helper()
	for _, f := range l.Filters() {
		if f.Name == name {
			return f
		}
	}
	t.Fatalf("filter %q not registered by az.bsh", name)
	return FilterDef{}
}

// TestAzRewritesProjectToTSV is the headline contract: every Azure rewrite
// switches the caller's verbose default (JSON) to a tab-separated projection
// via --query and --output tsv. If any of these regress, the compact table
// for that verb collapses back to raw JSON.
func TestAzRewritesProjectToTSV(t *testing.T) {
	l := loadAzPack(t)

	// One rewrite per service family so a missed rewrite in any family
	// fails a distinct subtest.
	names := []string{
		// Resource management.
		"az-group-list",
		"az-resource-list",
		"az-deployment-group-list",
		// Account / provider / extension.
		"az-account-list",
		"az-provider-list",
		"az-extension-list",
		// Compute.
		"az-vm-list",
		"az-vmss-list",
		"az-disk-list",
		// Networking.
		"az-network-vnet-list",
		"az-network-nsg-list",
		"az-network-public-ip-list",
		"az-network-lb-list",
		"az-network-dns-zone-list",
		"az-network-application-gateway-list",
		// Storage.
		"az-storage-account-list",
		"az-storage-container-list",
		"az-storage-blob-list",
		// App Services.
		"az-webapp-list",
		"az-functionapp-list",
		"az-appservice-plan-list",
		// AKS.
		"az-aks-list",
		"az-aks-nodepool-list",
		// Databases.
		"az-sql-server-list",
		"az-sql-db-list",
		"az-cosmosdb-list",
		"az-postgres-flexible-server-list",
		"az-mysql-flexible-server-list",
		"az-redis-list",
		// Key Vault.
		"az-keyvault-list",
		"az-keyvault-secret-list",
		"az-keyvault-key-list",
		// Container.
		"az-acr-list",
		"az-containerapp-list",
		"az-container-list",
		// Identity.
		"az-ad-user-list",
		"az-ad-sp-list",
		"az-role-assignment-list",
		"az-role-definition-list",
		// Monitor.
		"az-monitor-log-analytics-workspace-list",
		"az-monitor-activity-log-list",
		"az-monitor-metrics-alert-list",
		// Messaging.
		"az-servicebus-namespace-list",
		"az-eventhubs-namespace-list",
		"az-eventgrid-topic-list",
		// AI / data.
		"az-cognitiveservices-account-list",
		"az-ml-workspace-list",
		"az-search-service-list",
		"az-synapse-workspace-list",
		"az-datafactory-list",
		"az-databricks-workspace-list",
		// CDN / API Management.
		"az-cdn-profile-list",
		"az-apim-list",
		// Policy / advisor.
		"az-policy-assignment-list",
		"az-policy-definition-list",
		"az-advisor-recommendation-list",
		// Backup / IoT.
		"az-backup-vault-list",
		"az-iot-hub-list",
		"az-signalr-list",
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			rw := findAzRewrite(t, l, name)
			if !strings.Contains(rw.To, "--output tsv") {
				t.Errorf("%s: To=%q does not project to --output tsv", name, rw.To)
			}
			// scalar-only rewrites can use `[].name` without brackets; still
			// require --query somewhere in the replacement.
			if !strings.Contains(rw.To, "--query") && !strings.HasSuffix(rw.To, "--output tsv") {
				t.Errorf("%s: To=%q does not add a --query projection", name, rw.To)
			}
			for _, want := range []string{"--output", "--query", "-o"} {
				found := false
				for _, u := range rw.Unless {
					if u == want {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("%s: !unless missing %q; guard set = %v", name, want, rw.Unless)
				}
			}
			if !rw.Announce {
				t.Errorf("%s: rewrite fundamentally reshapes output but !announce is off", name)
			}
		})
	}
}

// TestAzRewriteRespectsExplicitOutput proves the !unless guard actually
// stops the rewrite when the caller asked for JSON / yaml / a bespoke
// projection.
func TestAzRewriteRespectsExplicitOutput(t *testing.T) {
	l := loadAzPack(t)
	var rules []compact.RewriteRule
	for _, rw := range l.Rewrites() {
		rules = append(rules, compact.RewriteRule{
			Name: rw.Name, Match: rw.Match, Unless: rw.Unless, To: rw.To,
			Announce: rw.Announce,
		})
	}
	rw := compact.NewRewriter(rules)

	cases := []struct {
		name   string
		input  string
		expect string // "" means expect no rewrite
	}{
		{
			name:   "plain vm list gets rewritten",
			input:  "az vm list",
			expect: "az-vm-list",
		},
		{
			name:   "explicit --output json is respected",
			input:  "az vm list --output json",
			expect: "",
		},
		{
			name:   "output=jsonc (equals form) is respected",
			input:  "az vm list --output=jsonc",
			expect: "",
		},
		{
			name:   "explicit --query is respected",
			input:  "az vm list --query [0].name",
			expect: "",
		},
		{
			name:   "short -o is respected",
			input:  "az vm list -o yaml",
			expect: "",
		},
		{
			name:   "trailing --subscription still lets rewrite fire",
			input:  "az group list --subscription my-sub",
			expect: "az-group-list",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, got := rw.Rewrite(tc.input)
			if got != tc.expect {
				t.Errorf("Rewrite(%q) rule = %q, want %q", tc.input, got, tc.expect)
			}
		})
	}
}

// TestAzFilterCaps checks that the safety-net filters clip runaway output
// when the caller demanded JSON. This guards the token budget for the
// pathological case: the agent explicitly asked for full JSON, the filter
// still keeps the response bounded.
func TestAzFilterCaps(t *testing.T) {
	l := loadAzPack(t)
	f := findAzFilter(t, l, "az-vm")

	if f.Ops.MaxLines == 0 {
		t.Error("az-vm filter has no max-lines cap")
	}
	if f.Ops.MaxLineLen == 0 {
		t.Error("az-vm filter has no max-line-len cap")
	}

	var lines []string
	for i := 0; i < 500; i++ {
		lines = append(lines, "very-long-line-"+strings.Repeat("x", 500))
	}
	raw := strings.Join(lines, "\n")
	got := f.Ops.Apply(raw)

	outLines := strings.Split(got, "\n")
	if len(outLines) > f.Ops.MaxLines+1 {
		t.Errorf("filter kept %d lines, want <= %d + overflow", len(outLines), f.Ops.MaxLines)
	}
	for _, ln := range outLines {
		if strings.HasPrefix(ln, "+") {
			continue
		}
		if len(ln) > f.Ops.MaxLineLen {
			t.Errorf("line exceeds cap %d chars: len=%d", f.Ops.MaxLineLen, len(ln))
			break
		}
	}
}

// TestAzFilterMatchLongestWins guarantees that a highly specific filter
// (az-monitor-activity-log) beats the generic per-service filter
// (az-monitor) for the same command. Otherwise activity-log floods would
// silently escape the tighter 60-line cap.
func TestAzFilterMatchLongestWins(t *testing.T) {
	l := loadAzPack(t)
	var defs []compact.ScriptFilterDef
	for _, f := range l.Filters() {
		defs = append(defs, compact.ScriptFilterDef{
			Name: f.Name, Match: f.Match, Compact: f.Compact, Ops: f.Ops,
		})
	}
	reg := compact.NewRegistry()
	reg.RegisterScriptFilters(defs)

	_, handler := reg.Compact(
		"az monitor activity-log list --start-time 2026-07-01",
		"one\ntwo\nthree\n", "", 0,
	)
	if handler != "az-monitor-activity-log" {
		t.Errorf("handler = %q, want az-monitor-activity-log (longest match)", handler)
	}
}
