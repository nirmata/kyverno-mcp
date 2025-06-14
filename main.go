// Package main implements a Model Context Protocol (MCP) server for Kyverno.
// It provides tools for managing and interacting with Kyverno policies and resources.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	kyvernocli "kyverno-mcp/pkg/kyverno-cli"
	"log"
	"os"
	"time"

	_ "embed"

	"github.com/kyverno/kyverno/cmd/cli/kubectl-kyverno/commands/apply"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"k8s.io/client-go/tools/clientcmd"
)

//go:embed policies/pod-security.yaml
var podSecurityPolicy []byte

//go:embed policies/rbac-best-practices.yaml
var rbacBestPracticesPolicy []byte

//go:embed policies/kubernetes-best-practices.yaml
var bestPracticesK8sPolicy []byte

//go:embed policies/all.yaml
var allPolicy []byte

// Define a package-level map that stores the temporary file path for each embedded policy
var policyTempFiles map[string]string

// Define a package-level map that stores the temporary file path for each embedded resource
var resourceTempFiles map[string]string

// kubeconfigPath holds the path to the kubeconfig file supplied via the --kubeconfig flag.
// If empty, the default resolution logic from client-go is used.
var kubeconfigPath string

// awsconfigPath holds the path to the AWS config file supplied via the --awsconfig flag.
// If empty, the default resolution logic (environment variable AWS_CONFIG_FILE or ~/.aws/config) is used.
var awsconfigPath string

// awsProfile holds the AWS profile name supplied via the --awsprofile flag.
var awsProfile string

//go:embed test.yaml
var resource []byte

func applyPolicy() string {
	resourceTempFiles := os.TempDir() + "/test.yaml"

	err := os.WriteFile(resourceTempFiles, resource, 0644)
	if err != nil {
		log.Printf("init: failed to create temp file for embedded resource: %v", err)
		os.Exit(1)
	}

	policyTempFiles := os.TempDir() + "/policies.yaml"

	err = os.WriteFile(policyTempFiles, allPolicy, 0644)

	if err != nil {
		log.Printf("init: failed to create temp file for embedded policy: %v", err)
		os.Exit(1)
	}

	applyCommandConfig := &apply.ApplyCommandConfig{
		PolicyPaths: []string{policyTempFiles},
		//ResourcePaths: []string{resourceTempFiles},
		Cluster:      true,
		Namespace:    "default",
		PolicyReport: true,
		OutputFormat: "json",
	}

	result, err := kyvernocli.ApplyCommandHelper(applyCommandConfig)
	if err != nil {
		log.Printf("applyPolicy: failed to apply policy: %v", err)
		os.Exit(1)
	}
	results := kyvernocli.BuildPolicyReportResults(false, result.EngineResponses...)
	jsonResults, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		log.Printf("applyPolicy: failed to marshal policy report results: %v", err)
		os.Exit(1)
	}
	//log.Printf("Results: %v", string(jsonResults))
	return string(jsonResults)

}

func listContexts(s *server.MCPServer) {

	// Helper to build loading rules based on optional explicit kubeconfig path
	newLoadingRules := func() *clientcmd.ClientConfigLoadingRules {
		if kubeconfigPath != "" {
			return &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath}
		}
		return clientcmd.NewDefaultClientConfigLoadingRules()
	}

	// Add a tool to list available contexts
	log.Println("Registering tool: list_contexts")
	s.AddTool(mcp.NewTool("list_contexts",
		mcp.WithDescription("List all available Kubernetes contexts"),
	), func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		log.Println("Tool 'list_contexts' invoked.")
		// Load the Kubernetes configuration from the specified kubeconfig or default location
		loadingRules := newLoadingRules()
		configOverrides := &clientcmd.ConfigOverrides{}

		config := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
		rawConfig, err := config.RawConfig()
		if err != nil {
			log.Printf("Error in 'list_contexts': failed to load kubeconfig: %v", err)
			return mcp.NewToolResultError(fmt.Sprintf("Error loading kubeconfig: %v", err)), nil
		}

		// Extract context names
		var contexts []string
		for name := range rawConfig.Contexts {
			contexts = append(contexts, name)
		}

		// Return the list of contexts as a JSON array
		result := map[string]interface{}{
			"available_contexts": contexts,
		}

		resultJSON, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			log.Printf("Error in 'list_contexts': failed to format result: %v", err)
			return mcp.NewToolResultError(fmt.Sprintf("Error formatting result: %v", err)), nil
		}

		return mcp.NewToolResultText(string(resultJSON)), nil
	})
}

func switchContext(s *server.MCPServer) {
	// Switch context tool
	log.Println("Registering tool: switch_context")
	s.AddTool(mcp.NewTool("switch_context",
		mcp.WithDescription("Switch to a different Kubernetes context"),
		mcp.WithString("context",
			mcp.Description("Name of the context to switch to"),
			mcp.Required(),
		),
	), func(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Get the context parameter
		contextName, err := request.RequireString("context")
		if err != nil {
			log.Printf("Error in 'switch_context': Invalid context parameter: %v", err)
			return mcp.NewToolResultError(fmt.Sprintf("Invalid context parameter: %v", err)), nil
		}

		// Load the Kubernetes configuration from the specified kubeconfig or default location
		loadingRules := func() *clientcmd.ClientConfigLoadingRules {
			if kubeconfigPath != "" {
				return &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath}
			}
			return clientcmd.NewDefaultClientConfigLoadingRules()
		}()
		configOverrides := &clientcmd.ConfigOverrides{
			CurrentContext: contextName,
		}

		// Create a new config with the overridden context
		config := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

		// Get the raw config to verify the context exists
		rawConfig, err := config.RawConfig()
		if err != nil {
			log.Printf("Error in 'switch_context': Error loading kubeconfig: %v", err)
			return mcp.NewToolResultError(fmt.Sprintf("Error loading kubeconfig: %v", err)), nil
		}

		// Verify the requested context exists
		if _, exists := rawConfig.Contexts[contextName]; !exists {
			availableContexts := make([]string, 0, len(rawConfig.Contexts))
			for name := range rawConfig.Contexts {
				availableContexts = append(availableContexts, name)
			}
			log.Printf("Error in 'switch_context': Context '%s' not found. Available: %v", contextName, availableContexts)
			return mcp.NewToolResultError(fmt.Sprintf("Context '%s' not found. Available contexts: %v", contextName, availableContexts)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Switched to context: %s", contextName)), nil
	},
	)
}

func scanCluster(s *server.MCPServer) {
	// Create a tool to scan the cluster for resources matching a policy
	log.Println("Registering tool: scan_cluster")
	scanClusterTool := mcp.NewTool(
		"scan_cluster",
		mcp.WithDescription("Scan cluster resources using an embedded Kyverno policy"),
		mcp.WithString("policySets", mcp.Description("Policy set key: pod-security, rbac-best-practices, best-practices-k8s, all (default: all).")),
		mcp.WithString("namespace", mcp.Description("Namespace to scan (default: all)")),
	)

	// Register the scan cluster tool
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

		results := applyPolicy()

		// Format the collected responses as pretty-printed JSON so callers can parse them.
		respJSON, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to serialise responses: %v", err)), nil
		}

		return mcp.NewToolResultText(string(respJSON)), nil
	})
}

func main() {

	// Define CLI flags
	flag.StringVar(&kubeconfigPath, "kubeconfig", "", "Path to the kubeconfig file to use. If not provided, defaults are used.")
	flag.StringVar(&awsconfigPath, "awsconfig", "", "Path to the AWS config file to use. If not provided, defaults to environment variable AWS_CONFIG_FILE or ~/.aws/config.")
	flag.StringVar(&awsProfile, "awsprofile", "", "AWS profile to use (defaults to current profile).")

	// Parse CLI flags early so subsequent init can rely on them
	flag.Parse()

	if kubeconfigPath != "" {
		// Ensure downstream libraries relying on KUBECONFIG honour the supplied path (e.g., Kyverno CLI helpers)
		_ = os.Setenv("KUBECONFIG", kubeconfigPath)
		log.Printf("Using kubeconfig file: %s", kubeconfigPath)
	}

	if awsconfigPath != "" {
		_ = os.Setenv("AWS_CONFIG_FILE", awsconfigPath)
		log.Printf("Using AWS config file: %s", awsconfigPath)
	}

	if awsProfile != "" {
		_ = os.Setenv("AWS_PROFILE", awsProfile)
		log.Printf("Using AWS profile: %s", awsProfile)
	}

	// Setup logging to standard output
	log.SetOutput(os.Stderr)
	log.Println("Logging initialized to Stdout.")
	log.Println("------------------------------------------------------------------------")
	log.Printf("Kyverno MCP Server starting at %s", time.Now().Format(time.RFC3339))

	log.SetPrefix("kyverno-mcp: ")
	log.Println("Starting Kyverno MCP server...")

	// Create a new MCP server
	log.Println("Creating new MCP server instance...")
	s := server.NewMCPServer(
		"Kyverno MCP Server",
		"1.0.0",
		server.WithToolCapabilities(false),
		server.WithRecovery(),
	)
	log.Println("MCP server instance created.")

	// Register tools
	listContexts(s)
	switchContext(s)
	scanCluster(s)

	// Start the MCP server
	log.Println("Starting MCP server on stdio...")
	var err error
	if err = server.ServeStdio(s); err != nil {
		log.Fatalf("Error starting server: %v\n", err)
	}
}
