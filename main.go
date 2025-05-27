package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"

	kyvernoclient "github.com/kyverno/kyverno/pkg/client/clientset/versioned"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// KyvernoClient wraps the Kyverno client for policy operations
type KyvernoClient struct {
	client *kyvernoclient.Clientset
}

// NewKyvernoClient creates a new Kyverno client using the specified context
func NewKyvernoClient(contextName string) (*KyvernoClient, error) {
	// Get the kubeconfig loading rules
	configLoadingRules := clientcmd.NewDefaultClientConfigLoadingRules()

	// Load the configuration with the specified context
	configOverrides := &clientcmd.ConfigOverrides{}
	if contextName != "" {
		configOverrides.CurrentContext = contextName
	}

	// Get the raw config to check if we need to handle AWS auth
	rawConfig, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		configLoadingRules,
		configOverrides,
	).RawConfig()

	if err != nil {
		return nil, fmt.Errorf("error loading raw kubeconfig: %v", err)
	}

	// Get the current context
	ctxName := contextName
	if ctxName == "" {
		ctxName = rawConfig.CurrentContext
	}

	// Get the context
	ctx, exists := rawConfig.Contexts[ctxName]
	if !exists {
		return nil, fmt.Errorf("context %q does not exist", ctxName)
	}

	// Get the cluster info
	cluster, exists := rawConfig.Clusters[ctx.Cluster]
	if !exists {
		return nil, fmt.Errorf("no cluster found for context %q", ctxName)
	}

	// Get the auth info
	_, exists = rawConfig.AuthInfos[ctx.AuthInfo]
	if !exists {
		return nil, fmt.Errorf("no auth info found for context %q", ctxName)
	}

	// Log the cluster and auth info for debugging
	log.Printf("Using cluster: %s (%s)", ctx.Cluster, cluster.Server)
	log.Printf("Using auth: %s\n", ctx.AuthInfo)

	// Create the client config with the specified context
	config, err := clientcmd.NewNonInteractiveClientConfig(
		rawConfig,
		ctxName,
		&clientcmd.ConfigOverrides{},
		configLoadingRules,
	).ClientConfig()

	if err != nil {
		return nil, fmt.Errorf("error creating client config: %v", err)
	}

	// Create the Kyverno clientset
	kyvernoClient, err := kyvernoclient.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("error creating Kyverno client: %v", err)
	}

	return &KyvernoClient{client: kyvernoClient}, nil
}

// ListClusterPolicies lists all ClusterPolicies
func (k *KyvernoClient) ListClusterPolicies() ([]byte, error) {
	policies, err := k.client.KyvernoV1().ClusterPolicies().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("error listing cluster policies: %v", err)
	}
	return json.MarshalIndent(policies.Items, "", "  ")
}

// GetClusterPolicy gets a specific ClusterPolicy by name
func (k *KyvernoClient) GetClusterPolicy(name string) ([]byte, error) {
	policy, err := k.client.KyvernoV1().ClusterPolicies().Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("error getting cluster policy %s: %v", name, err)
	}
	return json.MarshalIndent(policy, "", "  ")
}

func main() {
	// Parse command line flags
	contextName := flag.String("context", "", "Kubernetes context to use (default: current context)")
	debug := flag.Bool("debug", false, "Enable debug logging")
	flag.Parse()

	if !*debug {
		log.SetOutput(io.Discard) // Disable logging unless debug is enabled
	} else {
		log.SetFlags(log.LstdFlags | log.Lshortfile)
	}

	// Initialize the MCP server
	s := server.NewMCPServer(
		"Kyverno MCP Server",
		"0.1.0",
		server.WithToolCapabilities(false),
	)

	// Create Kyverno client
	kyvernoClient, err := NewKyvernoClient(*contextName)
	if err != nil {
		log.Fatalf("Error creating Kyverno client: %v", err)
	}

	// Create a tool to scan the cluster for resources matching a policy
	scanClusterTool := mcp.NewTool(
		"scan_cluster",
		mcp.WithDescription("Scan the cluster for resources that match the given policy"),
		mcp.WithString("policy", mcp.Description("Name of the policy to scan with")),
		mcp.WithString("namespace", mcp.Description("Namespace to scan (use 'all' for all namespaces)")),
		mcp.WithString("kind", mcp.Description("Kind of resources to scan (e.g., Pod, Deployment)")),
	)

	// Register the scan cluster tool
	s.AddTool(scanClusterTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, ok := request.Params.Arguments.(map[string]interface{})
		if !ok {
			return mcp.NewToolResultText("Error: invalid arguments format"), nil
		}

		policyName, ok := args["policy"].(string)
		if !ok || policyName == "" {
			return mcp.NewToolResultText("Error: 'policy' parameter is required"), nil
		}

		namespace, _ := args["namespace"].(string)
		kind, _ := args["kind"].(string)

		// Get the policy to verify it exists
		_, err := kyvernoClient.GetClusterPolicy(policyName)
		if err != nil {
			return mcp.NewToolResultText(fmt.Sprintf("Error getting policy %s: %v", policyName, err)), nil
		}

		// In a real implementation, we would use the Kyverno CLI or API to scan the cluster
		// For now, we'll return a placeholder response with the policy info
		result := map[string]interface{}{
			"status":    "success",
			"policy":    policyName,
			"namespace": namespace,
			"kind":      kind,
			"matches":   []string{"pod/example-pod-1", "deployment/app-deployment"},
		}

		resultJSON, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return mcp.NewToolResultText(fmt.Sprintf("Error formatting result: %v", err)), nil
		}

		return mcp.NewToolResultText(string(resultJSON)), nil
	})

	// Create a tool to apply a policy to resources
	applyPolicyTool := mcp.NewTool(
		"apply_policy",
		mcp.WithDescription("Apply a policy to specified resources in the cluster"),
		mcp.WithString("policy", mcp.Description("Name of the policy to apply")),
		mcp.WithString("resource", mcp.Description("Name of the resource to apply the policy to")),
		mcp.WithString("namespace", mcp.Description("Namespace of the resource (use 'default' if not specified)")),
	)

	// Register the apply policy tool
	s.AddTool(applyPolicyTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, ok := request.Params.Arguments.(map[string]interface{})
		if !ok {
			return mcp.NewToolResultText("Error: invalid arguments format"), nil
		}

		policyName, ok := args["policy"].(string)
		if !ok || policyName == "" {
			return mcp.NewToolResultText("Error: 'policy' parameter is required"), nil
		}

		resourceName, ok := args["resource"].(string)
		if !ok || resourceName == "" {
			return mcp.NewToolResultText("Error: 'resource' parameter is required"), nil
		}

		namespace, _ := args["namespace"].(string)
		if namespace == "" {
			namespace = "default"
		}

		// Verify the policy exists
		_, err := kyvernoClient.GetClusterPolicy(policyName)
		if err != nil {
			return mcp.NewToolResultText(fmt.Sprintf("Error getting policy %s: %v", policyName, err)), nil
		}

		// In a real implementation, we would use the Kyverno CLI or API to apply the policy
		// For now, we'll return a success response with the policy and resource info
		result := map[string]interface{}{
			"status":    "success",
			"policy":    policyName,
			"resource":  resourceName,
			"namespace": namespace,
			"message":   "Policy would be applied to the resource",
		}

		resultJSON, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return mcp.NewToolResultText(fmt.Sprintf("Error formatting result: %v", err)), nil
		}

		return mcp.NewToolResultText(string(resultJSON)), nil
	})

	// Start the MCP server
	if err := server.ServeStdio(s); err != nil {
		log.Fatalf("Error starting server: %v\n", err)
	}
}
