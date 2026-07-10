package extension

import (
	"strings"
	"testing"

	"go.banish.sh/banish/internal/compact"
)

// loadAWSPack parses the embedded aws.bsh once for the AWS pack tests. All
// tests exercise the shipped source so any drift in the pack surfaces here
// alongside the bench golden diff.
func loadAWSPack(t *testing.T) *Loader {
	t.Helper()
	builtins, err := Builtin()
	if err != nil {
		t.Fatal(err)
	}
	src, ok := builtins["aws.bsh"]
	if !ok {
		t.Fatal("no embedded aws.bsh")
	}
	l := NewLoader()
	if err := l.LoadSource("aws.bsh", src); err != nil {
		t.Fatal(err)
	}
	return l
}

// findRewrite returns the named rewrite from the loader, failing the test
// when it is missing -- the pack advertises a rewrite for each covered
// describe-* / list-* verb, so a missing name means the pack regressed.
func findRewrite(t *testing.T, l *Loader, name string) RewriteDef {
	t.Helper()
	for _, rw := range l.Rewrites() {
		if rw.Name == name {
			return rw
		}
	}
	t.Fatalf("rewrite %q not registered by aws.bsh", name)
	return RewriteDef{}
}

// findFilter returns the named filter or fails the test.
func findFilter(t *testing.T, l *Loader, name string) FilterDef {
	t.Helper()
	for _, f := range l.Filters() {
		if f.Name == name {
			return f
		}
	}
	t.Fatalf("filter %q not registered by aws.bsh", name)
	return FilterDef{}
}

// TestAWSRewritesProjectToText is the headline contract: every AWS rewrite
// switches the caller's verbose default (nested JSON) to a tab-separated
// text projection via --query and --output text. If any of these regress
// to raw JSON, the "aws" row in the README savings table collapses.
func TestAWSRewritesProjectToText(t *testing.T) {
	l := loadAWSPack(t)

	// A representative selection across the covered surface -- one per
	// service family so a missed rewrite in any family fails a distinct
	// subtest. Expanded here to catch drift across the extended service
	// set (EKS, KMS, apigateway, step-functions, kinesis, events, cw,
	// autoscaling, elb, cognito, code*, athena, glue, sagemaker, backup,
	// organizations, redshift, elasticache, opensearch, kafka, cloudtrail,
	// bedrock).
	names := []string{
		// Original set.
		"aws-ec2-describe-instances",
		"aws-ec2-describe-security-groups",
		"aws-lambda-list-functions",
		"aws-logs-describe-log-groups",
		"aws-logs-filter-log-events",
		"aws-iam-list-users",
		"aws-cf-list-stacks",
		"aws-cf-describe-stack-events",
		"aws-ecs-describe-services",
		"aws-rds-describe-db-instances",
		"aws-dynamodb-describe-table",
		"aws-s3api-list-buckets",
		"aws-route53-list-hosted-zones",
		"aws-ssm-describe-parameters",
		"aws-secretsmanager-list-secrets",
		// Extended set -- one per newly added service family.
		"aws-eks-describe-cluster",
		"aws-kms-list-keys",
		"aws-apigateway-get-rest-apis",
		"aws-apigatewayv2-get-apis",
		"aws-stepfunctions-list-state-machines",
		"aws-kinesis-describe-stream",
		"aws-firehose-describe-delivery-stream",
		"aws-events-list-rules",
		"aws-cloudwatch-describe-alarms",
		"aws-autoscaling-describe-auto-scaling-groups",
		"aws-elbv2-describe-load-balancers",
		"aws-elb-describe-load-balancers",
		"aws-cloudfront-list-distributions",
		"aws-acm-list-certificates",
		"aws-cognito-idp-list-user-pools",
		"aws-appsync-list-graphql-apis",
		"aws-sesv2-list-email-identities",
		"aws-codebuild-list-projects",
		"aws-codepipeline-list-pipelines",
		"aws-codecommit-list-repositories",
		"aws-deploy-list-applications",
		"aws-athena-list-work-groups",
		"aws-glue-get-tables",
		"aws-emr-list-clusters",
		"aws-sagemaker-list-endpoints",
		"aws-batch-describe-jobs",
		"aws-apprunner-list-services",
		"aws-amplify-list-apps",
		"aws-wafv2-list-web-acls",
		"aws-guardduty-get-findings",
		"aws-securityhub-get-findings",
		"aws-configservice-describe-config-rules",
		"aws-inspector2-list-findings",
		"aws-backup-list-backup-plans",
		"aws-organizations-list-accounts",
		"aws-ram-list-resource-shares",
		"aws-license-manager-list-license-configurations",
		"aws-redshift-describe-clusters",
		"aws-elasticache-describe-cache-clusters",
		"aws-docdb-describe-db-clusters",
		"aws-neptune-describe-db-clusters",
		"aws-opensearch-describe-domain",
		"aws-kafka-list-clusters",
		"aws-mq-list-brokers",
		"aws-timestream-write-list-tables",
		"aws-cloudtrail-lookup-events",
		"aws-iot-list-things",
		"aws-appconfig-list-applications",
		"aws-transfer-list-servers",
		"aws-ds-describe-directories",
		"aws-workspaces-describe-workspaces",
		"aws-connect-list-instances",
		"aws-bedrock-list-foundation-models",
		"aws-xray-get-trace-summaries",
		"aws-ce-list-cost-category-definitions",
		"aws-budgets-describe-budgets",
		"aws-mediaconvert-list-jobs",
		"aws-medialive-list-channels",
		"aws-mediapackage-list-channels",
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			rw := findRewrite(t, l, name)
			if !strings.Contains(rw.To, "--output text") {
				t.Errorf("%s: To=%q does not project to --output text", name, rw.To)
			}
			if !strings.Contains(rw.To, "--query") {
				t.Errorf("%s: To=%q does not add a --query projection", name, rw.To)
			}
			// The guard set must include the flag families a caller uses
			// to demand a specific shape; otherwise explicit intent gets
			// silently overridden.
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

// TestAWSRewriteRespectsExplicitOutput proves the !unless guard actually
// stops the rewrite when the caller asked for JSON. This is the guarantee
// that lets us reshape default calls without ever surprising a script that
// pipes the raw JSON to jq downstream.
func TestAWSRewriteRespectsExplicitOutput(t *testing.T) {
	l := loadAWSPack(t)
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
			name:   "plain describe-instances gets rewritten",
			input:  "aws ec2 describe-instances",
			expect: "aws-ec2-describe-instances",
		},
		{
			name:   "explicit --output json is respected",
			input:  "aws ec2 describe-instances --output json",
			expect: "",
		},
		{
			name:   "output=table (equals form) is respected",
			input:  "aws ec2 describe-instances --output=table",
			expect: "",
		},
		{
			name:   "explicit --query is respected",
			input:  "aws ec2 describe-instances --query Reservations[0]",
			expect: "",
		},
		{
			name:   "short -o is respected",
			input:  "aws ec2 describe-instances -o yaml",
			expect: "",
		},
		{
			name:   "trailing profile flag still lets rewrite fire",
			input:  "aws lambda list-functions --profile prod",
			expect: "aws-lambda-list-functions",
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

// TestAWSFilterCaps checks that the safety-net filters clip runaway output
// even when the rewrite got bypassed (caller asked for JSON). This guards
// the token budget for the pathological case: the agent explicitly asked
// for full JSON, the filter still keeps the response bounded.
func TestAWSFilterCaps(t *testing.T) {
	l := loadAWSPack(t)
	f := findFilter(t, l, "aws-ec2")

	if f.Ops.MaxLines == 0 {
		t.Error("aws-ec2 filter has no max-lines cap")
	}
	if f.Ops.MaxLineLen == 0 {
		t.Error("aws-ec2 filter has no max-line-len cap")
	}

	// Simulate a 500-line raw response with a mix of short and long lines.
	var lines []string
	for i := 0; i < 500; i++ {
		lines = append(lines, "very-long-line-"+strings.Repeat("x", 500))
	}
	raw := strings.Join(lines, "\n")
	got := f.Ops.Apply(raw)

	outLines := strings.Split(got, "\n")
	if len(outLines) > f.Ops.MaxLines+1 { // +1 for the overflow marker line
		t.Errorf("filter kept %d lines, want <= %d + overflow", len(outLines), f.Ops.MaxLines)
	}
	for _, ln := range outLines {
		if strings.HasPrefix(ln, "+") { // overflow marker; skip length check
			continue
		}
		if len(ln) > f.Ops.MaxLineLen {
			t.Errorf("line exceeds cap %d chars: len=%d", f.Ops.MaxLineLen, len(ln))
			break
		}
	}
}

// TestAWSFilterMatchLongestWins guarantees that a highly specific filter
// (aws-cloudformation-describe-stack-events) beats the generic per-service
// filter (aws-cloudformation) for the same command, since the registry
// resolves by longest-prefix-wins. If this regresses, stack-event floods
// silently escape the tight 40-line cap.
func TestAWSFilterMatchLongestWins(t *testing.T) {
	l := loadAWSPack(t)
	var defs []compact.ScriptFilterDef
	for _, f := range l.Filters() {
		defs = append(defs, compact.ScriptFilterDef{
			Name: f.Name, Match: f.Match, Compact: f.Compact, Ops: f.Ops,
		})
	}
	reg := compact.NewRegistry()
	reg.RegisterScriptFilters(defs)

	_, handler := reg.Compact(
		"aws cloudformation describe-stack-events --stack-name prod",
		"one\ntwo\nthree\n", "", 0,
	)
	if handler != "aws-cloudformation-describe-stack-events" {
		t.Errorf("handler = %q, want aws-cloudformation-describe-stack-events (longest match)", handler)
	}
}
