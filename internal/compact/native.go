package compact

import (
	"sort"
	"strings"
)

// RenderFunc parses structured command output and re-renders it compactly.
// ok=false means the output did not look like what the renderer expects;
// the caller falls through to script filters or raw output. Renderers must
// degrade gracefully -- never lose data, never guess.
type RenderFunc func(stdout, stderr string, exitCode int) (string, bool)

// nativeRenderer binds a tokenized prefix pattern to a renderer.
type nativeRenderer struct {
	name   string
	match  string
	render RenderFunc
}

// nativeRenderers is the built-in table, sorted longest pattern first at
// init so more specific patterns win. Native renderers exist only for
// output that needs structural re-rendering; everything expressible as
// line-wise ops lives in the embedded defaults.bsh instead.
var nativeRenderers = []nativeRenderer{
	{"git-diff", "git diff", renderGitDiff},
	{"git-show", "git show", renderGitDiff},
	{"kubectl-get-json", "kubectl get", renderKubectlGet},
	{"gh-pr-list-json", "gh pr list", renderGHPRList},
	{"gh-run-list-json", "gh run list", renderGHRunList},
	// az is the shortest match by design: the longest-prefix sort in init
	// puts it last, so any service-specific native renderer we add later
	// still wins. This catches every Azure CLI command whose output is
	// JSON -- either because no hand-crafted rewrite in az.bsh covered it,
	// or the caller opted into JSON with --output json / --output=jsonc.
	{"az-json", "az", renderAzJSON},
	// aws is the shortest match by design: the longest-prefix sort in init
	// puts it last, so any service-specific native renderer we add later
	// still wins. This catches every AWS command whose output is JSON --
	// either because no hand-crafted rewrite in aws.bsh covered it, or the
	// caller opted into JSON with --output json / --output=json.
	{"aws-json", "aws", renderAWSJSON},
	// gcloud is the shortest match by design: the longest-prefix sort in
	// init puts it last, so any service-specific native renderer we add
	// later still wins. This catches every gcloud command whose output is
	// JSON -- either because no hand-crafted rewrite in gcloud.bsh covered
	// it, or the caller opted into JSON with --format=json.
	{"gcloud-json", "gcloud", renderGcloudJSON},
}

func init() {
	sort.SliceStable(nativeRenderers, func(i, j int) bool {
		return len(strings.Fields(nativeRenderers[i].match)) > len(strings.Fields(nativeRenderers[j].match))
	})
}

// lookupNative returns the renderer whose pattern prefix-matches the words,
// or nil.
func lookupNative(words []Word) *nativeRenderer {
	for i := range nativeRenderers {
		if _, ok := MatchPrefix(words, nativeRenderers[i].match); ok {
			return &nativeRenderers[i]
		}
	}
	return nil
}
