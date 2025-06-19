// Package tools provides tools for the MCP server.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	kyverno "kyverno-mcp/pkg/kyverno-cli"
	"log"
	"os"
	"strings"

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
var kubernetesBestPracticesPolicy []byte

func defaultPolicies() []byte {
	combinedPolicy := strings.TrimSpace(string(podSecurityPolicy)) + "\n---\n" + strings.TrimSpace(string(rbacBestPracticesPolicy)) + "\n---\n" + strings.TrimSpace(string(kubernetesBestPracticesPolicy))
	return []byte(combinedPolicy)
}

func applyPolicy(policyKey string, namespace string, gitBranch string) (string, error) {
	// Select the appropriate embedded policy content based on the requested key
	var policyData []byte
	switch policyKey {
	case "pod-security":
		policyData = podSecurityPolicy
	case "rbac-best-practices":
		policyData = rbacBestPracticesPolicy
	case "kubernetes-best-practices":
		policyData = kubernetesBestPracticesPolicy
	default:
		policyData = defaultPolicies()
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

	applyCommandConfig := &apply.ApplyCommandConfig{
		PolicyPaths:  []string{tmpFile.Name()},
		Cluster:      true,
		Namespace:    namespace,
		PolicyReport: true,
		OutputFormat: "json",
		GitBranch:    gitBranch,
	}

	result, err := kyverno.ApplyCommandHelper(applyCommandConfig)
	if err != nil {
		return "", fmt.Errorf("failed to apply policy: %w", err)
	}

	results := kyverno.BuildPolicyReportResults(false, result.EngineResponses...)
	jsonResults, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal policy report results: %w", err)
	}

	return string(jsonResults), nil
}

func ApplyPolicies(s *server.MCPServer) {
	log.Println("Registering tool: apply_policies")
	applyPoliciesTool := mcp.NewTool(
		"apply_policies",
		mcp.WithDescription(`Apply Kyverno policies to Kubernetes resources in a cluster. If no namespace is provided, the policies will be applied to the default namespace.`),
		mcp.WithString("policySets", mcp.Description(`Policy set key: pod-security, rbac-best-practices, kubernetes-best-practices, all (default: all).`)),
		mcp.WithString("namespace", mcp.Description(`Namespace to apply policies to (default: default)`)),
	)

	s.AddTool(applyPoliciesTool, func(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, ok := request.Params.Arguments.(map[string]any)
		if !ok {
			return mcp.NewToolResultError("Error: invalid arguments format"), nil
		}

		policySets := "all"
		if args["policySets"] != nil {
			policySets = args["policySets"].(string)
		}

		namespace := "default"
		if args["namespace"] != nil {
			namespace = args["namespace"].(string)
		}

		gitBranch := "main"
		if args["gitBranch"] != nil {
			gitBranch = args["gitBranch"].(string)
		}

		results, err := applyPolicy(policySets, namespace, gitBranch)
		if err != nil {
			// Surface the error back to the MCP client without terminating the server.
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(results), nil
	})
}
