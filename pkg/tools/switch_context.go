// Package tools provides tools for the MCP server.
package tools

import (
	"context"
	"fmt"

	"k8s.io/klog/v2"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"k8s.io/client-go/tools/clientcmd"
)

func SwitchContext(s *server.MCPServer) {
	// Switch context tool
	klog.InfoS("Registering tool: switch_context")
	s.AddTool(mcp.NewTool("switch_context",
		mcp.WithDescription("Switch to a different Kubernetes context. If no context is provided, the default context will be used."),
		mcp.WithString("context",
			mcp.Description("Name of the context to switch to"),
			mcp.Required(),
		),
	), func(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Get the context parameter
		contextName, err := request.RequireString("context")
		if err != nil {
			klog.ErrorS(err, "Error in 'switch_context': Invalid context parameter")
			return mcp.NewToolResultError(fmt.Sprintf("Invalid context parameter: %v", err)), nil
		}

		pathOpts := clientcmd.NewDefaultPathOptions()

		cfg, err := pathOpts.GetStartingConfig()
		if err != nil {
			klog.ErrorS(err, "Error in 'switch_context': Error loading kubeconfig")
			return mcp.NewToolResultError(fmt.Sprintf("Error loading kubeconfig: %v", err)), nil
		}

		if _, ok := cfg.Contexts[contextName]; !ok {
			availableContexts := make([]string, 0, len(cfg.Contexts))
			for name := range cfg.Contexts {
				availableContexts = append(availableContexts, name)
			}
			klog.ErrorS(err, "Error in 'switch_context': Context '%s' not found. Available: %v", contextName, availableContexts)
			return mcp.NewToolResultError(fmt.Sprintf("Context '%s' not found. Available contexts: %v", contextName, availableContexts)), nil
		}

		cfg.CurrentContext = contextName

		if err := clientcmd.ModifyConfig(pathOpts, *cfg, false); err != nil {
			klog.ErrorS(err, "Error in 'switch_context': Error writing kubeconfig")
			return mcp.NewToolResultError(fmt.Sprintf("Error writing kubeconfig: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Switched to context: %s (saved to kubeconfig)", contextName)), nil
	},
	)
}
