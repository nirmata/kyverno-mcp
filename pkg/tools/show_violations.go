// Package tools provides tools for the MCP server.
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
)

// errNoPolicyReportCRD is returned when the PolicyReport and ClusterPolicyReport CRDs are not present in the cluster.
var errNoPolicyReportCRD = errors.New("no PolicyReport CRD found")

// ShowViolations registers the show_violations tool with the MCP server.
func ShowViolations(s *server.MCPServer) {
	s.AddTool(
		mcp.NewTool(
			"show_violations",
			mcp.WithDescription(`This tool is used when Kyverno is installed in the cluster. It returns all non-passing Kyverno PolicyReport results for a workload.`),
			mcp.WithString("namespace", mcp.Description(`Namespace (default: default)`), mcp.DefaultString("default")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			ns, _ := req.RequireString("namespace")
			if ns == "" {
				ns = "default"
			}

			violationsJSON, err := gatherViolationsJSON(ctx, ns)
			if err != nil {
				// If Kyverno (PolicyReport CRDs) is not installed, provide Helm installation instructions instead
				if errors.Is(err, errNoPolicyReportCRD) {
					return mcp.NewToolResultText(kyvernoHelmInstructions()), nil
				}
				return mcp.NewToolResultError(err.Error()), nil
			}

			return mcp.NewToolResultText(string(violationsJSON)), nil
		})
}

// gatherViolationsJSON fetches PolicyReport and ClusterPolicyReport resources and returns a JSON
// array containing only failing reports with relevant violation details.
// The returned JSON format mirrors the structure produced by the following `kubectl` command:
//
//	kubectl get policyreports -n <ns> -o json | jq '.items[] | select(.summary.fail > 0) | {name: .metadata.name, resource: .scope, summary: .summary, violations: [.results[] | select(.result == "fail") | {policy: .policy, rule: .rule, message: .message, severity: .severity}]}'
//
// If the PolicyReport CRDs are not present it returns errNoPolicyReportCRD so the caller can
// gracefully instruct the user to install Kyverno.
func gatherViolationsJSON(ctx context.Context, ns string) ([]byte, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		cfg, err = clientcmd.BuildConfigFromFlags("", clientcmd.RecommendedHomeFile)
	}
	if err != nil {
		return nil, fmt.Errorf("build kube-config: %w", err)
	}

	disc, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return nil, err
	}

	// Discover the GVRs for PolicyReport / ClusterPolicyReport
	polrGVR, cpolrGVR, err := policyReportGVRs(disc)
	if err != nil {
		return nil, err
	}

	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	if ns == "" {
		ns = "default"
	}

	// We will accumulate the filtered reports in this slice and marshal it to JSON at the end.
	var failingReports []map[string]interface{}

	// ---------------------------------------------------------------------
	// 1. Namespaced PolicyReports
	// ---------------------------------------------------------------------
	if polrGVR.Resource != "" {
		prList, err := dyn.Resource(polrGVR).Namespace(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			klog.ErrorS(err, "cannot list namespaced PolicyReports")
		} else {
			if err := filterFailingReports(prList.Items, &failingReports); err != nil {
				return nil, err
			}
		}
	}

	// ---------------------------------------------------------------------
	// 2. Cluster-scoped ClusterPolicyReports
	// ---------------------------------------------------------------------
	if cpolrGVR.Resource != "" {
		cprList, err := dyn.Resource(cpolrGVR).List(ctx, metav1.ListOptions{})
		if err != nil {
			klog.ErrorS(err, "cannot list ClusterPolicyReports")
		} else {
			if err := filterFailingReports(cprList.Items, &failingReports); err != nil {
				return nil, err
			}
		}
	}

	if len(failingReports) == 0 {
		return []byte("[]"), nil
	}
	return json.MarshalIndent(failingReports, "", "  ")
}

// filterFailingReports inspects each PolicyReport / ClusterPolicyReport item and, if it
// contains failing summaries, appends a simplified representation to out. The output is optimized for LLM consumption.
func filterFailingReports(items []unstructured.Unstructured, out *[]map[string]interface{}) error {
	for _, item := range items {
		obj := item.Object

		// The Kyverno PolicyReport spec keeps summary at the top-level.
		summary, ok := obj["summary"].(map[string]interface{})
		if !ok {
			continue
		}

		// Extract the fail count regardless of numeric type (int, int64, float64, json.Number).
		if getInt(summary["fail"]) == 0 {
			continue // skip reports with no failures
		}

		rep := map[string]interface{}{
			"name":     item.GetName(),
			"resource": obj["scope"],
			"summary":  summary,
		}

		// Collect failing result details
		resultsRaw, _ := obj["results"].([]interface{})
		var violations []map[string]interface{}
		for _, r := range resultsRaw {
			rMap, ok := r.(map[string]interface{})
			if !ok {
				continue
			}
			if res, _ := rMap["result"].(string); res != "fail" {
				continue
			}
			violations = append(violations, map[string]interface{}{
				"policy":   rMap["policy"],
				"rule":     rMap["rule"],
				"message":  rMap["message"],
				"severity": rMap["severity"],
			})
		}
		rep["violations"] = violations

		*out = append(*out, rep)
	}
	return nil
}

// getInt attempts to coerce various numeric representations into int.
func getInt(v interface{}) int {
	switch t := v.(type) {
	case int:
		return t
	case int32:
		return int(t)
	case int64:
		return int(t)
	case float64:
		return int(t)
	case json.Number:
		i, _ := t.Int64()
		return int(i)
	default:
		return 0
	}
}

// policyReportGVRs discovers policyreports / clusterpolicyreports
func policyReportGVRs(disc discovery.DiscoveryInterface) (schema.GroupVersionResource, schema.GroupVersionResource, error) {
	const group = "wgpolicyk8s.io"
	grps, err := disc.ServerGroups()
	if err != nil {
		return schema.GroupVersionResource{}, schema.GroupVersionResource{}, err
	}

	for _, g := range grps.Groups {
		if g.Name != group {
			continue
		}
		for _, v := range g.Versions {
			resList, err := disc.ServerResourcesForGroupVersion(v.GroupVersion)
			if err != nil {
				continue
			}
			for _, r := range resList.APIResources {
				if r.Name == "policyreports" {
					polr := schema.GroupVersionResource{Group: group, Version: v.Version, Resource: "policyreports"}
					cpolr := schema.GroupVersionResource{Group: group, Version: v.Version, Resource: "clusterpolicyreports"}
					return polr, cpolr, nil
				}
			}
		}
	}
	return schema.GroupVersionResource{}, schema.GroupVersionResource{}, errNoPolicyReportCRD
}

// kyvernoHelmInstructions returns user-friendly instructions to install Kyverno via Helm.
func kyvernoHelmInstructions() string {
	return `Kyverno is not installed in the cluster.  

Install Kyverno using Helm:

1. Add the Kyverno Helm repository:
   helm repo add kyverno https://kyverno.github.io/kyverno/

2. Update the local Helm chart repository cache:
   helm repo update

3. Install Kyverno in the kyverno namespace (creates it if it doesn't exist):
   helm install kyverno kyverno/kyverno --namespace kyverno --create-namespace

4. (Optional) Install the Kyverno policies for pod security standards:
   helm install kyverno-policies kyverno/kyverno-policies --namespace kyverno

After installation, wait until all Kyverno pods are running before re-running this tool.`
}
