package extension

import (
	"strings"
	"testing"

	"go.banish.sh/banish/internal/compact"
)

// loadGcloudPack parses the embedded gcloud.bsh once for the gcloud pack
// tests. All tests exercise the shipped source so any drift in the pack
// surfaces here alongside the bench golden diff.
func loadGcloudPack(t *testing.T) *Loader {
	t.Helper()
	builtins, err := Builtin()
	if err != nil {
		t.Fatal(err)
	}
	src, ok := builtins["gcloud.bsh"]
	if !ok {
		t.Fatal("no embedded gcloud.bsh")
	}
	l := NewLoader()
	if err := l.LoadSource("gcloud.bsh", src); err != nil {
		t.Fatal(err)
	}
	return l
}

func findGcloudRewrite(t *testing.T, l *Loader, name string) RewriteDef {
	t.Helper()
	for _, rw := range l.Rewrites() {
		if rw.Name == name {
			return rw
		}
	}
	t.Fatalf("rewrite %q not registered by gcloud.bsh", name)
	return RewriteDef{}
}

func findGcloudFilter(t *testing.T, l *Loader, name string) FilterDef {
	t.Helper()
	for _, f := range l.Filters() {
		if f.Name == name {
			return f
		}
	}
	t.Fatalf("filter %q not registered by gcloud.bsh", name)
	return FilterDef{}
}

// TestGcloudRewritesProjectToValue is the headline contract: every gcloud
// rewrite switches the caller's default (a padded human table) to a bare
// tab-separated projection via --format="value(...)". If any of these
// regress, the compact table for that verb collapses back.
func TestGcloudRewritesProjectToValue(t *testing.T) {
	l := loadGcloudPack(t)

	// One rewrite per service family so a missed rewrite in any family
	// fails a distinct subtest.
	names := []string{
		// Projects / auth / config / services.
		"gcloud-projects-list",
		"gcloud-organizations-list",
		"gcloud-auth-list",
		"gcloud-config-list",
		"gcloud-services-list",
		// Compute.
		"gcloud-compute-instances-list",
		"gcloud-compute-disks-list",
		"gcloud-compute-images-list",
		"gcloud-compute-networks-list",
		"gcloud-compute-firewall-rules-list",
		"gcloud-compute-forwarding-rules-list",
		"gcloud-compute-backend-services-list",
		"gcloud-compute-instance-templates-list",
		"gcloud-compute-instance-groups-list",
		"gcloud-compute-target-https-proxies-list",
		"gcloud-compute-addresses-list",
		// GKE.
		"gcloud-container-clusters-list",
		"gcloud-container-node-pools-list",
		"gcloud-container-fleet-memberships-list",
		// Storage / Run / Functions / App Engine.
		"gcloud-storage-buckets-list",
		"gcloud-run-services-list",
		"gcloud-run-revisions-list",
		"gcloud-functions-list",
		"gcloud-app-services-list",
		// SQL / Firestore / Spanner / Bigtable.
		"gcloud-sql-instances-list",
		"gcloud-sql-databases-list",
		"gcloud-firestore-databases-list",
		"gcloud-spanner-instances-list",
		"gcloud-bigtable-instances-list",
		// Pub/Sub / Tasks / Scheduler / Workflows / Eventarc.
		"gcloud-pubsub-topics-list",
		"gcloud-pubsub-subscriptions-list",
		"gcloud-tasks-queues-list",
		"gcloud-scheduler-jobs-list",
		"gcloud-workflows-list",
		"gcloud-eventarc-triggers-list",
		// DNS / IAM / Secrets / KMS.
		"gcloud-dns-managed-zones-list",
		"gcloud-iam-service-accounts-list",
		"gcloud-iam-roles-list",
		"gcloud-secrets-list",
		"gcloud-kms-keys-list",
		// Logging / Monitoring.
		"gcloud-logging-sinks-list",
		"gcloud-monitoring-channels-list",
		"gcloud-monitoring-policies-list",
		// Build / Source / Artifact Registry.
		"gcloud-builds-list",
		"gcloud-source-repos-list",
		"gcloud-artifacts-repositories-list",
		// AI / data.
		"gcloud-ai-models-list",
		"gcloud-ai-endpoints-list",
		"gcloud-dataflow-jobs-list",
		"gcloud-dataproc-clusters-list",
		"gcloud-composer-environments-list",
		// Deployment Manager / Certificate Manager / Batch.
		"gcloud-deployment-manager-deployments-list",
		"gcloud-certificate-manager-certificates-list",
		"gcloud-batch-jobs-list",
		// Networking extras.
		"gcloud-access-context-manager-policies-list",
		"gcloud-compute-vpc-access-connectors-list",
		"gcloud-endpoints-services-list",
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			rw := findGcloudRewrite(t, l, name)
			if !strings.Contains(rw.To, "--format='value(") &&
				!strings.Contains(rw.To, "--format=\"value(") {
				t.Errorf("%s: To=%q does not project to value(...) format", name, rw.To)
			}
			// Guard on --format so json/yaml/csv opt-ins are respected.
			found := false
			for _, u := range rw.Unless {
				if u == "--format" {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("%s: !unless missing --format; guard set = %v", name, rw.Unless)
			}
			if !rw.Announce {
				t.Errorf("%s: rewrite fundamentally reshapes output but !announce is off", name)
			}
		})
	}
}

// TestGcloudRewriteRespectsExplicitFormat proves the !unless guard
// actually stops the rewrite when the caller asked for a specific
// format. Explicit intent (json for jq pipes, yaml for review, a
// bespoke table projection) is always honoured.
func TestGcloudRewriteRespectsExplicitFormat(t *testing.T) {
	l := loadGcloudPack(t)
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
			name:   "plain compute instances list gets rewritten",
			input:  "gcloud compute instances list",
			expect: "gcloud-compute-instances-list",
		},
		{
			name:   "explicit --format=json is respected",
			input:  "gcloud compute instances list --format=json",
			expect: "",
		},
		{
			name:   "space-form --format json is respected",
			input:  "gcloud compute instances list --format json",
			expect: "",
		},
		{
			name:   "custom value projection is respected",
			input:  "gcloud compute instances list --format=value(name)",
			expect: "",
		},
		{
			name:   "yaml is respected",
			input:  "gcloud compute instances list --format=yaml",
			expect: "",
		},
		{
			name:   "trailing --project still lets rewrite fire",
			input:  "gcloud projects list --project my-p",
			expect: "gcloud-projects-list",
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

// TestGcloudFilterCaps checks that the safety-net filters clip runaway
// output when the caller demanded JSON. This guards the token budget for
// the pathological case: the agent explicitly asked for full JSON, the
// filter still keeps the response bounded.
func TestGcloudFilterCaps(t *testing.T) {
	l := loadGcloudPack(t)
	f := findGcloudFilter(t, l, "gcloud-compute")

	if f.Ops.MaxLines == 0 {
		t.Error("gcloud-compute filter has no max-lines cap")
	}
	if f.Ops.MaxLineLen == 0 {
		t.Error("gcloud-compute filter has no max-line-len cap")
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

// TestGcloudFilterMatchLongestWins guarantees that a highly specific
// filter (gcloud-logging-read) beats the generic per-service filter
// (gcloud-logging) for the same command. Otherwise log floods would
// silently escape the tighter 100-line cap.
func TestGcloudFilterMatchLongestWins(t *testing.T) {
	l := loadGcloudPack(t)
	var defs []compact.ScriptFilterDef
	for _, f := range l.Filters() {
		defs = append(defs, compact.ScriptFilterDef{
			Name: f.Name, Match: f.Match, Compact: f.Compact, Ops: f.Ops,
		})
	}
	reg := compact.NewRegistry()
	reg.RegisterScriptFilters(defs)

	_, handler := reg.Compact(
		"gcloud logging read --limit=1000 severity>=ERROR",
		"one\ntwo\nthree\n", "", 0,
	)
	if handler != "gcloud-logging-read" {
		t.Errorf("handler = %q, want gcloud-logging-read (longest match)", handler)
	}
}
