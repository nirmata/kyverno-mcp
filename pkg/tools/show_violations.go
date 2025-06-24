// Package tools provides tools for the MCP server.
package tools

import (
	"context"
	"fmt"
	"strings"

	"sigs.k8s.io/yaml"

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

			yamls, err := gatherReportYAML(ctx, ns)
			if err != nil {
				// If Kyverno (PolicyReport CRDs) is not installed, provide Helm installation instructions instead
				if strings.Contains(err.Error(), "no PolicyReport CRD found") {
					return mcp.NewToolResultText(kyvernoHelmInstructions()), nil
				}
				return mcp.NewToolResultError(err.Error()), nil
			}

			if len(yamls) == 0 {
				return mcp.NewToolResultText("[]"), nil
			}
			return mcp.NewToolResultText(string(yamls)), nil
		})
}

func gatherReportYAML(ctx context.Context, ns string) ([]byte, error) {
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

	var b strings.Builder

	// ---------------------------------------------------------------------
	// 1. Namespaced PolicyReports
	// ---------------------------------------------------------------------
	if polrGVR.Resource != "" {
		prList, err := dyn.Resource(polrGVR).Namespace(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			klog.ErrorS(err, "cannot list namespaced PolicyReports")
		} else {
			if err := appendAsYAML(&b, prList.Items); err != nil {
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
			if err := appendAsYAML(&b, cprList.Items); err != nil {
				return nil, err
			}
		}
	}

	return []byte(b.String()), nil
}

func appendAsYAML(b *strings.Builder, items []unstructured.Unstructured) error {
	for i, item := range items {
		j, err := yaml.Marshal(item.Object)
		if err != nil {
			return err
		}
		y, err := yaml.JSONToYAML(j)
		if err != nil {
			return err
		}
		b.Write(y)
		if i < len(items)-1 {
			b.WriteString("\n---\n")
		}
	}
	if len(items) > 0 && b.Len() > 0 {
		b.WriteString("\n")
	}
	return nil
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
	return schema.GroupVersionResource{}, schema.GroupVersionResource{}, fmt.Errorf("no PolicyReport CRD found")
}

// kyvernoHelmInstructions returns user-friendly instructions to install Kyverno via Helm.
func kyvernoHelmInstructions() string {
	return `Kyverno does not appear to be installed in this cluster.

Install Kyverno using Helm:

1. Add the Kyverno Helm repository:
   helm repo add kyverno https://kyverno.github.io/kyverno/

2. Update the local Helm chart repository cache:
   helm repo update

3. Install Kyverno in the kyverno namespace (creates it if it doesn't exist):
   helm install kyverno kyverno/kyverno --namespace kyverno --create-namespace

4. (Optional) Install the Kyverno policies:
   helm install kyverno-policies kyverno/kyverno-policies --namespace kyverno

After installation, wait until all Kyverno pods are running before re-running this tool.`
}
