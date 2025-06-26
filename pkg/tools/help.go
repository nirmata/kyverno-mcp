package tools

import (
	"context"
	_ "embed"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"k8s.io/klog/v2"
)

//go:embed docs/installation.md
var installationHelp string

//go:embed docs/troubleshooting.md
var troubleshootingHelp string

func Help(s *server.MCPServer) {
	klog.InfoS("Registering tool: help")
	docTool := mcp.NewTool(
		"help",
		mcp.WithDescription(`Get Kyverno documentation for installation and troubleshooting`),
		mcp.WithString("topic", mcp.Description(`Topic of documentation to get between installation and troubleshooting Kyverno environment`), mcp.Required()),
	)

	s.AddTool(docTool, func(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, ok := request.Params.Arguments.(map[string]any)
		if !ok {
			return mcp.NewToolResultError("Error: invalid arguments format"), nil
		}

		topic, ok := args["topic"].(string)
		if !ok {
			return mcp.NewToolResultError("Error: invalid documentation type"), nil
		}

		switch topic {
		case "installation":
			return mcp.NewToolResultText(installationHelp), nil
		case "troubleshooting":
			return mcp.NewToolResultText(troubleshootingHelp), nil
		default:
			return mcp.NewToolResultError("Error: invalid documentation type"), nil
		}
	})
}
