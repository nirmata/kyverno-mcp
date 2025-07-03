// Package tools provides tools for the MCP server.
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"kyverno-mcp/pkg/common"

	policyreportv1alpha2 "github.com/kyverno/kyverno/api/policyreport/v1alpha2"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
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
			mcp.WithString("namespace", mcp.Description(`Namespace to query (default: default, use "all" for all namespaces)`), mcp.DefaultString("default")),
			mcp.WithString("namespace_exclude", mcp.Description(`Comma-separated namespaces to exclude when namespace="all" (default: kube-system,kyverno)`), mcp.DefaultString("kube-system,kyverno")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			ns, _ := req.RequireString("namespace")
			if ns == "" {
				ns = "default"
			}

			nsExclude, _ := req.RequireString("namespace_exclude")
			if nsExclude == "" {
				nsExclude = "kube-system,kyverno"
			}

			violationsJSON, err := gatherViolationsJSON(ctx, ns, nsExclude)
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
// array containing only failing and error reports with relevant violation details.
// It uses Kyverno's BuildPolicyReportResults helper to convert PolicyReports into a consistent format.
func gatherViolationsJSON(ctx context.Context, ns, nsExclude string) ([]byte, error) {
	cfg, err := common.KubeConfig()
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

	// Determine if we should apply namespace exclude filtering
	// Only apply exclude filtering when querying "all" namespaces
	queryAllNamespaces := ns == "all"
	var excludeSet map[string]struct{}
	if queryAllNamespaces {
		excludeSet = common.ParseNamespaceExcludes(nsExclude)
	}

	var allResults []policyreportv1alpha2.PolicyReportResult

	// Helper function to process PolicyReport items
	addPolicyReportResults := func(items []unstructured.Unstructured) error {
		for _, u := range items {
			// Skip if namespace is excluded (only when querying all namespaces)
			if queryAllNamespaces {
				if _, skip := excludeSet[u.GetNamespace()]; skip {
					continue
				}
			}

			// Convert unstructured to typed PolicyReport
			var pr policyreportv1alpha2.PolicyReport
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, &pr); err != nil {
				klog.ErrorS(err, "failed to convert to PolicyReport", "name", u.GetName(), "namespace", u.GetNamespace())
				continue
			}

			// Skip reports with no failures, errors, or warnings
			if pr.Summary.Fail == 0 && pr.Summary.Error == 0 && pr.Summary.Warn == 0 {
				continue
			}

			// Extract relevant results from PolicyReport
			for _, result := range pr.Results {
				// Only include fail, error, and warn results
				if result.Result != policyreportv1alpha2.StatusFail &&
					result.Result != policyreportv1alpha2.StatusError &&
					result.Result != policyreportv1alpha2.StatusWarn {
					continue
				}

				allResults = append(allResults, result)
			}
		}
		return nil
	}

	// Helper function to process ClusterPolicyReport items
	addClusterPolicyReportResults := func(items []unstructured.Unstructured) error {
		for _, u := range items {
			// Convert unstructured to typed ClusterPolicyReport
			var cpr policyreportv1alpha2.ClusterPolicyReport
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, &cpr); err != nil {
				klog.ErrorS(err, "failed to convert to ClusterPolicyReport", "name", u.GetName())
				continue
			}

			// Skip reports with no failures, errors, or warnings
			if cpr.Summary.Fail == 0 && cpr.Summary.Error == 0 && cpr.Summary.Warn == 0 {
				continue
			}

			// Extract relevant results from ClusterPolicyReport
			for _, result := range cpr.Results {
				// Only include fail, error, and warn results
				if result.Result != policyreportv1alpha2.StatusFail &&
					result.Result != policyreportv1alpha2.StatusError &&
					result.Result != policyreportv1alpha2.StatusWarn {
					continue
				}


				allResults = append(allResults, result)
			}
		}
		return nil
	}

	// ---------------------------------------------------------------------
	// 1. Namespaced PolicyReports
	// ---------------------------------------------------------------------
	if polrGVR.Resource != "" {
		var prList *unstructured.UnstructuredList
		var err error

		if queryAllNamespaces {
			// Query all namespaces
			prList, err = dyn.Resource(polrGVR).List(ctx, metav1.ListOptions{})
		} else {
			// Query specific namespace
			prList, err = dyn.Resource(polrGVR).Namespace(ns).List(ctx, metav1.ListOptions{})
		}

		if err != nil {
			klog.ErrorS(err, "cannot list namespaced PolicyReports")
		} else {
			if err := addPolicyReportResults(prList.Items); err != nil {
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
			if err := addClusterPolicyReportResults(cprList.Items); err != nil {
				return nil, err
			}
		}
	}

	if len(allResults) == 0 {
		return []byte("[]"), nil
	}
	return json.MarshalIndent(allResults, "", "  ")
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
