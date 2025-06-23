// Package tools provides tools for the MCP server.
package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"k8s.io/klog/v2"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"k8s.io/client-go/tools/clientcmd"
)

var kubeconfigPath string

func ListContexts(s *server.MCPServer) {
	// Helper to build loading rules based on optional explicit kubeconfig path
	newLoadingRules := func() *clientcmd.ClientConfigLoadingRules {
		if kubeconfigPath != "" {
			return &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath}
		}
		return clientcmd.NewDefaultClientConfigLoadingRules()
	}

	// Add a tool to list available contexts
	klog.InfoS("Registering tool: list_contexts")
	s.AddTool(mcp.NewTool("list_contexts",
		mcp.WithDescription("List all available Kubernetes contexts"),
	), func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		klog.InfoS("Tool 'list_contexts' invoked.")
		// Load the Kubernetes configuration from the specified kubeconfig or default location
		loadingRules := newLoadingRules()
		configOverrides := &clientcmd.ConfigOverrides{}

		config := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
		rawConfig, err := config.RawConfig()
		if err != nil {
			klog.ErrorS(err, "Error in 'list_contexts': failed to load kubeconfig")
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
			klog.ErrorS(err, "Error in 'list_contexts': failed to format result")
			return mcp.NewToolResultError(fmt.Sprintf("Error formatting result: %v", err)), nil
		}

		return mcp.NewToolResultText(string(resultJSON)), nil
	})
}
