package tools

import (
	"context"
	"encoding/json"
	"fmt"
	kyvernocli "kyverno-mcp/pkg/kyverno-cli"
	"log"
	"os"

	"github.com/kyverno/kyverno/cmd/cli/kubectl-kyverno/commands/apply"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	_ "embed"
)

//go:embed policies/pod-security.yaml
var podSecurityPolicy []byte

//go:embed policies/rbac-best-practices.yaml
var rbacBestPracticesPolicy []byte

//go:embed policies/kubernetes-best-practices.yaml
var bestPracticesK8sPolicy []byte

//go:embed policies/all.yaml
var allPolicy []byte

func applyPolicy(policyKey string, namespace string) (string, error) {
	// Select the appropriate embedded policy content based on the requested key
	var policyData []byte
	switch policyKey {
	case "pod-security":
		policyData = podSecurityPolicy
	case "rbac-best-practices":
		policyData = rbacBestPracticesPolicy
	case "best-practices-k8s":
		policyData = bestPracticesK8sPolicy
	default:
		policyData = allPolicy
	}

	// Create a uniquely named temporary file to avoid collisions between concurrent requests.
	tmpFile, err := os.CreateTemp("", "kyverno-policy-*.yaml")
	if err != nil {
		return "", fmt.Errorf("failed to create temp policy file: %w", err)
	}

	// Ensure the file is cleaned up after we have finished processing.
	// The cleanup is deferred *after* the temp file is successfully created so that
	// the file is always removed regardless of subsequent failures.
	defer func(name string) {
		_ = os.Remove(name)
	}(tmpFile.Name())

	// Write the selected policy content to the temporary file
	if _, err := tmpFile.Write(policyData); err != nil {
		tmpFile.Close()
		return "", fmt.Errorf("failed to write policy data to temp file: %w", err)
	}

	// Flush the file to disk before it's used by downstream helpers
	if err := tmpFile.Close(); err != nil {
		return "", fmt.Errorf("failed to close temp policy file: %w", err)
	}

	// Use an empty string to indicate that Kyverno should scan all namespaces
	if namespace == "all" {
		namespace = ""
	}

	applyCommandConfig := &apply.ApplyCommandConfig{
		PolicyPaths:  []string{tmpFile.Name()},
		Cluster:      true,
		Namespace:    namespace,
		PolicyReport: true,
		OutputFormat: "json",
	}

	result, err := kyvernocli.ApplyCommandHelper(applyCommandConfig)
	if err != nil {
		return "", fmt.Errorf("failed to apply policy: %w", err)
	}

	results := kyvernocli.BuildPolicyReportResults(false, result.EngineResponses...)
	jsonResults, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal policy report results: %w", err)
	}

	return string(jsonResults), nil
}

func ScanCluster(s *server.MCPServer) {
	log.Println("Registering tool: scan_cluster")
	scanClusterTool := mcp.NewTool(
		"scan_cluster",
		mcp.WithDescription("Apply Kyverno policies to Kubernetes resources in a cluster"),
		mcp.WithString("policySets", mcp.Description("Policy set key: pod-security, rbac-best-practices, best-practices-k8s, all (default: all).")),
		mcp.WithString("namespace", mcp.Description("Namespace to scan (default: all)")),
	)

	s.AddTool(scanClusterTool, func(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, ok := request.Params.Arguments.(map[string]interface{})
		if !ok {
			return mcp.NewToolResultError("Error: invalid arguments format"), nil
		}

		namespace, _ := args["namespace"].(string)
		if namespace == "" {
			namespace = "all"
		}

		policyKey, _ := args["policySets"].(string)
		if policyKey == "" {
			policyKey = "all"
		}

		results, err := applyPolicy(policyKey, namespace)
		if err != nil {
			// Surface the error back to the MCP client without terminating the server.
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(results), nil
	})
}
