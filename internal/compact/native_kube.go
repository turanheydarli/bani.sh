package compact

import (
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"
)

// kubectl get -o json returns a List object:
//
//   { "kind": "PodList",   "items": [ {"kind":"Pod",   ...}, ... ] }
//   { "kind": "NodeList",  "items": [ {"kind":"Node",  ...}, ... ] }
//   { "kind": "ServiceList", ... }
//   { "kind": "DeploymentList", ... }
//
// A single resource ("kubectl get pod my-pod -o json") returns the object
// directly with no items[] wrapper. The renderer handles both shapes.
//
// The kubectl -o json schema is versioned, stable, and covered by the k8s
// API guarantees -- unlike the wide/human table which reflows across
// kubectl versions. Per-kind we pick the same columns kubectl's own table
// output would surface (NAME/READY/STATUS/RESTARTS/AGE for pods, etc.) so
// the compacted view maps 1:1 to what a human would expect.

const kubeMaxRows = 60

// kubeListJSON is the outer shape of any List (PodList, NodeList, ...).
type kubeListJSON struct {
	Kind  string            `json:"kind"`
	Items []json.RawMessage `json:"items"`
}

// renderKubectlGet parses kubectl get -o json output and produces a compact
// tab-separated table. Falls through (ok=false) for anything that does not
// unmarshal as a k8s List (single-resource GETs, non-JSON output, empty).
func renderKubectlGet(stdout, stderr string, exitCode int) (string, bool) {
	trimmed := strings.TrimSpace(stdout)
	if !strings.HasPrefix(trimmed, "{") {
		return "", false
	}

	var list kubeListJSON
	if err := json.Unmarshal([]byte(trimmed), &list); err != nil {
		return "", false
	}
	if !strings.HasSuffix(list.Kind, "List") {
		return "", false
	}

	itemKind := strings.TrimSuffix(list.Kind, "List")
	renderer, ok := kubeItemRenderers[itemKind]
	if !ok {
		// Unknown kind: fall back to a name-only table so we never break
		// on kinds we haven't taught the renderer about yet.
		renderer = renderKubeName
	}

	rows := make([][]string, 0, len(list.Items))
	for _, raw := range list.Items {
		row, ok := renderer(raw)
		if !ok {
			continue
		}
		rows = append(rows, row)
	}

	// If every row lives in the same namespace, strip the ns/ prefix from
	// the name column -- kubectl's own table does the same when you did not
	// pass --all-namespaces. This is the biggest per-row win for single-ns
	// queries.
	collapseSingleNamespace(itemKind, rows)

	return formatKubeTable(itemKind, len(list.Items), rows), true
}

// collapseSingleNamespace strips the ns/ prefix from the NAME column when
// every row shares the same namespace. Mutates rows in place. Cluster-scoped
// kinds (Node) never have a namespace, so this is a no-op there.
func collapseSingleNamespace(itemKind string, rows [][]string) {
	if len(rows) == 0 {
		return
	}
	if _, needsNS := kubeHeaders[itemKind]; !needsNS {
		return
	}
	firstSlash := strings.IndexByte(rows[0][0], '/')
	if firstSlash < 0 {
		return
	}
	ns := rows[0][0][:firstSlash]
	for _, r := range rows {
		i := strings.IndexByte(r[0], '/')
		if i < 0 || r[0][:i] != ns {
			return
		}
	}
	for _, r := range rows {
		r[0] = r[0][len(ns)+1:]
	}
}

// kubeItemRenderers maps a k8s object kind ("Pod", "Node", ...) to a row
// extractor. New kinds land as one function each; unknown kinds fall back
// to the name-only renderer via the fallback in renderKubectlGet.
var kubeItemRenderers = map[string]func(json.RawMessage) ([]string, bool){
	"Pod":        renderKubePod,
	"Node":       renderKubeNode,
	"Service":    renderKubeService,
	"Deployment": renderKubeDeployment,
}

func renderKubePod(raw json.RawMessage) ([]string, bool) {
	var p struct {
		Metadata struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
		} `json:"metadata"`
		Status struct {
			Phase             string `json:"phase"`
			ContainerStatuses []struct {
				Ready        bool `json:"ready"`
				RestartCount int  `json:"restartCount"`
			} `json:"containerStatuses"`
		} `json:"status"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, false
	}
	ready, total, restarts := 0, len(p.Status.ContainerStatuses), 0
	for _, c := range p.Status.ContainerStatuses {
		if c.Ready {
			ready++
		}
		restarts += c.RestartCount
	}
	return []string{
		nsQualified(p.Metadata.Namespace, p.Metadata.Name),
		fmt.Sprintf("%d/%d", ready, total),
		nonEmpty(p.Status.Phase, "Unknown"),
		fmt.Sprintf("%d", restarts),
	}, true
}

func renderKubeNode(raw json.RawMessage) ([]string, bool) {
	var n struct {
		Metadata struct {
			Name   string            `json:"name"`
			Labels map[string]string `json:"labels"`
		} `json:"metadata"`
		Status struct {
			Conditions []struct {
				Type   string `json:"type"`
				Status string `json:"status"`
			} `json:"conditions"`
			NodeInfo struct {
				KubeletVersion string `json:"kubeletVersion"`
			} `json:"nodeInfo"`
		} `json:"status"`
	}
	if err := json.Unmarshal(raw, &n); err != nil {
		return nil, false
	}
	status := "NotReady"
	for _, c := range n.Status.Conditions {
		if c.Type == "Ready" && c.Status == "True" {
			status = "Ready"
			break
		}
	}
	role := "<none>"
	for k := range n.Metadata.Labels {
		const rolePrefix = "node-role.kubernetes.io/"
		if strings.HasPrefix(k, rolePrefix) {
			role = strings.TrimPrefix(k, rolePrefix)
			break
		}
	}
	return []string{
		n.Metadata.Name,
		status,
		role,
		nonEmpty(n.Status.NodeInfo.KubeletVersion, "-"),
	}, true
}

func renderKubeService(raw json.RawMessage) ([]string, bool) {
	var s struct {
		Metadata struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
		} `json:"metadata"`
		Spec struct {
			Type      string   `json:"type"`
			ClusterIP string   `json:"clusterIP"`
			Ports     []struct {
				Port     int    `json:"port"`
				Protocol string `json:"protocol"`
			} `json:"ports"`
		} `json:"spec"`
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, false
	}
	ports := make([]string, 0, len(s.Spec.Ports))
	for _, p := range s.Spec.Ports {
		proto := p.Protocol
		if proto == "" {
			proto = "TCP"
		}
		ports = append(ports, fmt.Sprintf("%d/%s", p.Port, proto))
	}
	return []string{
		nsQualified(s.Metadata.Namespace, s.Metadata.Name),
		nonEmpty(s.Spec.Type, "ClusterIP"),
		nonEmpty(s.Spec.ClusterIP, "-"),
		nonEmpty(strings.Join(ports, ","), "-"),
	}, true
}

func renderKubeDeployment(raw json.RawMessage) ([]string, bool) {
	var d struct {
		Metadata struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
		} `json:"metadata"`
		Spec struct {
			Replicas int `json:"replicas"`
		} `json:"spec"`
		Status struct {
			ReadyReplicas     int `json:"readyReplicas"`
			UpdatedReplicas   int `json:"updatedReplicas"`
			AvailableReplicas int `json:"availableReplicas"`
		} `json:"status"`
	}
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, false
	}
	return []string{
		nsQualified(d.Metadata.Namespace, d.Metadata.Name),
		fmt.Sprintf("%d/%d", d.Status.ReadyReplicas, d.Spec.Replicas),
		fmt.Sprintf("%d", d.Status.UpdatedReplicas),
		fmt.Sprintf("%d", d.Status.AvailableReplicas),
	}, true
}

// renderKubeName is the fallback when the item kind has no dedicated
// renderer -- new kinds still surface as a name-only table instead of
// falling through to the raw JSON dump.
func renderKubeName(raw json.RawMessage) ([]string, bool) {
	var g struct {
		Metadata struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(raw, &g); err != nil {
		return nil, false
	}
	return []string{nsQualified(g.Metadata.Namespace, g.Metadata.Name)}, true
}

// kubeHeaders picks the column titles for each item kind. Missing kinds
// (fallback path) render header-less name-only tables.
var kubeHeaders = map[string][]string{
	"Pod":        {"NAME", "READY", "STATUS", "RESTARTS"},
	"Node":       {"NAME", "STATUS", "ROLE", "VERSION"},
	"Service":    {"NAME", "TYPE", "CLUSTER-IP", "PORTS"},
	"Deployment": {"NAME", "READY", "UP-TO-DATE", "AVAILABLE"},
}

func formatKubeTable(itemKind string, totalItems int, rows [][]string) string {
	if len(rows) == 0 {
		return fmt.Sprintf("no %ss found", strings.ToLower(itemKind))
	}

	var b strings.Builder
	tw := tabwriter.NewWriter(&b, 2, 4, 2, ' ', 0)

	if headers, ok := kubeHeaders[itemKind]; ok {
		fmt.Fprintln(tw, strings.Join(headers, "\t"))
	}

	shown := rows
	overflow := 0
	if len(shown) > kubeMaxRows {
		overflow = len(shown) - kubeMaxRows
		shown = shown[:kubeMaxRows]
	}
	for _, r := range shown {
		fmt.Fprintln(tw, strings.Join(r, "\t"))
	}
	tw.Flush()

	if overflow > 0 {
		fmt.Fprintf(&b, "+%d more %ss\n", overflow, strings.ToLower(itemKind))
	}
	return strings.TrimRight(b.String(), "\n")
}

// nsQualified returns "namespace/name" or just "name" for cluster-scoped
// resources. Matches how kubectl -A / --all-namespaces disambiguates rows.
func nsQualified(ns, name string) string {
	if ns == "" {
		return name
	}
	return ns + "/" + name
}

func nonEmpty(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
