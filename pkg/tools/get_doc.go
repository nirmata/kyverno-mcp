package tools

import (
	"context"
	_ "embed"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"k8s.io/klog/v2"
)

//go:embed docs/installation.md
var installationDoc string

//go:embed docs/troubleshooting.md
var troubleshootingDoc string

func GetDocs(s *server.MCPServer) {
	klog.InfoS("Registering tool: get_docs")
	docTool := mcp.NewTool(
		"get_docs",
		mcp.WithDescription(`Get Kyverno documentation for installation and troubleshooting`),
		mcp.WithString("type", mcp.Description(`Type of documentation to get between installation and troubleshooting Kyverno environment`)),
	)

	s.AddTool(docTool, func(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, ok := request.Params.Arguments.(map[string]any)
		if !ok {
			return mcp.NewToolResultError("Error: invalid arguments format"), nil
		}

		docType, ok := args["type"].(string)
		if !ok {
			return mcp.NewToolResultError("Error: invalid documentation type"), nil
		}

		switch docType {
		case "installation":
			return mcp.NewToolResultText(installationDoc), nil
		case "troubleshooting":
			return mcp.NewToolResultText(troubleshootingDoc), nil
		default:
			return mcp.NewToolResultError("Error: invalid documentation type"), nil
		}
	})
}
