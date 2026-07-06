package compact

import (
	"strings"
	"testing"
)

const kubePodListJSON = `{
  "kind": "PodList",
  "apiVersion": "v1",
  "metadata": { "resourceVersion": "12345" },
  "items": [
    {
      "kind": "Pod",
      "metadata": { "name": "api-7f8b", "namespace": "default" },
      "status": {
        "phase": "Running",
        "containerStatuses": [
          { "ready": true,  "restartCount": 0 },
          { "ready": true,  "restartCount": 2 }
        ]
      }
    },
    {
      "kind": "Pod",
      "metadata": { "name": "worker-abc", "namespace": "default" },
      "status": {
        "phase": "CrashLoopBackOff",
        "containerStatuses": [
          { "ready": false, "restartCount": 17 }
        ]
      }
    }
  ]
}`

func TestRenderKubectlGetPods(t *testing.T) {
	out, ok := renderKubectlGet(kubePodListJSON, "", 0)
	if !ok {
		t.Fatal("renderKubectlGet ok=false")
	}
	// Both pods live in the "default" namespace, so ns/ prefixes collapse
	// and the plain name survives -- the "single namespace" path.
	for _, want := range []string{
		"NAME", "READY", "STATUS", "RESTARTS",
		"api-7f8b", "2/2", "Running",
		"worker-abc", "0/1", "CrashLoopBackOff", "17",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	// The raw JSON keys must not survive: entire point of the renderer.
	if strings.Contains(out, "containerStatuses") || strings.Contains(out, "resourceVersion") {
		t.Errorf("raw JSON leaked:\n%s", out)
	}
}

// TestRenderKubectlGetMultiNamespaceKeepsPrefix asserts that when pods span
// multiple namespaces (kubectl get pods -A), the ns/ prefix stays -- the
// agent needs to distinguish pods with the same name across namespaces.
func TestRenderKubectlGetMultiNamespaceKeepsPrefix(t *testing.T) {
	multi := `{"kind":"PodList","items":[
	  {"kind":"Pod","metadata":{"name":"api","namespace":"prod"},"status":{"phase":"Running","containerStatuses":[{"ready":true,"restartCount":0}]}},
	  {"kind":"Pod","metadata":{"name":"api","namespace":"staging"},"status":{"phase":"Running","containerStatuses":[{"ready":true,"restartCount":0}]}}
	]}`
	out, ok := renderKubectlGet(multi, "", 0)
	if !ok {
		t.Fatal("ok=false")
	}
	if !strings.Contains(out, "prod/api") || !strings.Contains(out, "staging/api") {
		t.Errorf("ns prefixes lost across multi-ns list:\n%s", out)
	}
}

func TestRenderKubectlGetEmptyList(t *testing.T) {
	out, ok := renderKubectlGet(`{"kind":"PodList","items":[]}`, "", 0)
	if !ok {
		t.Fatal("ok=false")
	}
	if !strings.Contains(out, "no pods found") {
		t.Errorf("empty-list rendering missing:\n%s", out)
	}
}

func TestRenderKubectlGetUnknownKind(t *testing.T) {
	// A kind we have not taught a renderer must still surface as a
	// name-only table -- never fall through to raw JSON.
	json := `{"kind":"ConfigMapList","items":[
	  {"kind":"ConfigMap","metadata":{"name":"cm-a","namespace":"kube-system"}},
	  {"kind":"ConfigMap","metadata":{"name":"cm-b","namespace":"kube-system"}}
	]}`
	out, ok := renderKubectlGet(json, "", 0)
	if !ok {
		t.Fatal("ok=false on unknown kind")
	}
	if !strings.Contains(out, "kube-system/cm-a") || !strings.Contains(out, "kube-system/cm-b") {
		t.Errorf("name-only fallback lost entries:\n%s", out)
	}
}

func TestRenderKubectlGetFallsThroughOnNonJSON(t *testing.T) {
	// The human table (what happens if rewrite was disabled) must not be
	// parsed as JSON; the renderer must decline so the script filter runs.
	humanTable := "NAME       READY   STATUS    RESTARTS   AGE\napi-7f8b   2/2     Running   0          5d"
	if _, ok := renderKubectlGet(humanTable, "", 0); ok {
		t.Error("human table should fall through, not parse as JSON")
	}
}

func TestRenderKubectlGetFallsThroughOnSingleResource(t *testing.T) {
	// `kubectl get pod my-pod -o json` returns a single Pod, not a PodList.
	// Falling through is acceptable here -- a single resource is small
	// enough that the raw JSON is not the token disaster kind list is.
	single := `{"kind":"Pod","metadata":{"name":"solo"}}`
	if _, ok := renderKubectlGet(single, "", 0); ok {
		t.Error("single-resource GET should fall through")
	}
}

func TestRenderKubectlGetNodes(t *testing.T) {
	json := `{"kind":"NodeList","items":[
	  {"kind":"Node","metadata":{"name":"n1","labels":{"node-role.kubernetes.io/master":""}},"status":{"nodeInfo":{"kubeletVersion":"v1.28.3"},"conditions":[{"type":"Ready","status":"True"}]}},
	  {"kind":"Node","metadata":{"name":"n2","labels":{}},"status":{"nodeInfo":{"kubeletVersion":"v1.28.3"},"conditions":[{"type":"Ready","status":"False"}]}}
	]}`
	out, ok := renderKubectlGet(json, "", 0)
	if !ok {
		t.Fatal("ok=false")
	}
	if !strings.Contains(out, "n1") || !strings.Contains(out, "Ready") || !strings.Contains(out, "master") {
		t.Errorf("node row missing fields:\n%s", out)
	}
	if !strings.Contains(out, "n2") || !strings.Contains(out, "NotReady") {
		t.Errorf("not-ready node incorrect:\n%s", out)
	}
}

func TestRenderKubectlGetOverflow(t *testing.T) {
	// Build 80 pods to exceed the row cap and assert the overflow marker.
	var b strings.Builder
	b.WriteString(`{"kind":"PodList","items":[`)
	for i := 0; i < 80; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`{"kind":"Pod","metadata":{"name":"p`)
		b.WriteString(itoa(i))
		b.WriteString(`","namespace":"ns"},"status":{"phase":"Running","containerStatuses":[{"ready":true,"restartCount":0}]}}`)
	}
	b.WriteString(`]}`)
	out, ok := renderKubectlGet(b.String(), "", 0)
	if !ok {
		t.Fatal("ok=false")
	}
	if !strings.Contains(out, "+20 more pods") {
		t.Errorf("overflow marker missing:\n%s", out)
	}
}

// itoa is a tiny int→string to avoid strconv import in test data.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits [10]byte
	i := len(digits)
	for n > 0 {
		i--
		digits[i] = byte('0' + n%10)
		n /= 10
	}
	return string(digits[i:])
}
