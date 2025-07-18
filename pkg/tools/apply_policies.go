// Package tools provides tools for the MCP server.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/nirmata/kyverno-mcp/pkg/common"
	kyverno "github.com/nirmata/kyverno-mcp/pkg/kyverno-cli"

	// Add import for Kyverno engine API to filter responses
	engineapi "github.com/kyverno/kyverno/pkg/engine/api"

	"github.com/kyverno/kyverno/cmd/cli/kubectl-kyverno/commands/apply"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"k8s.io/klog/v2"

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

func applyPolicy(policyKey string, namespace string, gitBranch string, namespaceExclude string) (string, error) {
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
		if cerr := tmpFile.Close(); cerr != nil {
			klog.ErrorS(cerr, "failed to close temp file after write error")
		}
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

	// Build a set of namespaces to exclude from the policy report results.
	excludedNS := common.ParseNamespaceExcludes(namespaceExclude)

	// Filter out engine responses that belong to excluded namespaces.
	var filteredEngineResponses []engineapi.EngineResponse
	for _, er := range result.EngineResponses {
		if _, found := excludedNS[er.Resource.GetNamespace()]; found {
			continue
		}
		filteredEngineResponses = append(filteredEngineResponses, er)
	}

	results := kyverno.BuildPolicyReportResults(false, filteredEngineResponses...)
	jsonResults, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal policy report results: %w", err)
	}

	return string(jsonResults), nil
}

func ApplyPolicies(s *server.MCPServer) {
	klog.InfoS("Registering tool: apply_policies")
	applyPoliciesTool := mcp.NewTool(
		"apply_policies",
		mcp.WithDescription(`Scan the cluster resources for policy violations with provided policies or default policy sets. Use "all" to scan all namespaces. If no namespace is provided i.e. "", the policies will be applied to the default namespace.`),
		mcp.WithString("policySets", mcp.Description(`Policy set key: pod-security, rbac-best-practices, kubernetes-best-practices, all (default: all).`)),
		mcp.WithString("namespace", mcp.Description(`Namespace to apply policies to (default: default)`)),
		mcp.WithString("gitBranch", mcp.Description(`Git branch to apply policies from (default: main)`)),
		mcp.WithString("namespace_exclude", mcp.Description(`Namespace to exclude from applying policies to (default: kube-system, kyverno)`)),
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

		namespace := ""
		if args["namespace"] != nil {
			namespace = args["namespace"].(string)
		}

		gitBranch := "main"
		if args["gitBranch"] != nil {
			gitBranch = args["gitBranch"].(string)
		}

		namespaceExclude := "kube-system,kyverno"
		if args["namespace_exclude"] != nil {
			namespaceExclude = args["namespace_exclude"].(string)
		}

		results, err := applyPolicy(policySets, namespace, gitBranch, namespaceExclude)
		if err != nil {
			// Surface the error back to the MCP client without terminating the server.
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(results), nil
	})
}
