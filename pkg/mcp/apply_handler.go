package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	mcp "github.com/mark3labs/mcp-go/mcp"
	mcpServer "github.com/mark3labs/mcp-go/server"

	"github.com/kyverno/go-kyverno-mcp/pkg/kyverno"
	"github.com/kyverno/go-kyverno-mcp/pkg/mcp/types"
)

const (
	// ToolName is the name of the Kyverno policy application tool
	ToolName = "kyverno-apply-policies"
	// ToolDescription is the description of the Kyverno policy application tool
	ToolDescription = "Applies Kyverno policies to Kubernetes resources and returns the validation results"
)

// ApplyService implements the MCP service for applying Kyverno policies to resources
type ApplyService struct {
	kyvernoEngine kyverno.Engine
}

// NewApplyService creates a new instance of ApplyService
//
// Parameters:
//   - engine: The Kyverno engine to use for applying policies
//
// Returns:
//   - A new instance of ApplyService
func NewApplyService(engine kyverno.Engine) *ApplyService {
	return &ApplyService{
		kyvernoEngine: engine,
	}
}

// ProcessApplyRequest processes an ApplyRequest by delegating to the Kyverno engine
//
// Parameters:
//   - ctx: The context for the request
//   - req: The ApplyRequest to process
//
// Returns:
//   - The ApplyResponse from the Kyverno engine
//   - An error if the request could not be processed
func (s *ApplyService) ProcessApplyRequest(ctx context.Context, req *types.ApplyRequest) (*types.ApplyResponse, error) {
	return s.kyvernoEngine.ApplyPolicies(ctx, req)
}

// GetTool returns the MCP tool definition for the ApplyService
func (s *ApplyService) GetTool() mcp.Tool {
	// Create a simple tool with a JSON schema string
	schema := `{
		"type": "object",
		"properties": {
			"policyPaths": {
				"type": "array",
				"items": {
					"type": "string"
				},
				"description": "Paths to Kyverno policy files or directories containing policies"
			},
			"resourceQueries": {
				"type": "array",
				"items": {
					"type": "object",
					"properties": {
						"apiVersion": {
							"type": "string",
							"description": "API version of the resource (e.g., 'v1', 'apps/v1')"
						},
						"kind": {
							"type": "string",
							"description": "Kind of the resource (e.g., 'Pod', 'Deployment')"
						},
						"namespace": {
							"type": "string",
							"description": "Namespace of the resource (empty for cluster-scoped resources)"
						},
						"name": {
							"type": "string",
							"description": "Name of a specific resource to fetch (if empty, all matching resources are returned)"
						},
						"labelSelector": {
							"type": "string",
							"description": "Label selector to filter resources"
						}
					},
					"required": ["apiVersion", "kind"]
				},
				"description": "Queries for resources to apply policies to"
			},
			"kubeconfigPath": {
				"type": "string",
				"description": "Path to kubeconfig file (uses in-cluster config if not provided)"
			}
		},
		"required": ["policyPaths", "resourceQueries"]
	}`

	// Create a tool with the raw JSON schema
	return mcp.NewToolWithRawSchema(
		ToolName,
		ToolDescription,
		json.RawMessage(schema),
	)
}

// ToolHandler handles MCP tool calls for the ApplyService
func (s *ApplyService) ToolHandler() mcpServer.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Parse the request arguments into ApplyRequest
		var req types.ApplyRequest
		
		// Use BindArguments to unmarshal the request arguments into the ApplyRequest
		if err := request.BindArguments(&req); err != nil {
			// If we can't bind the arguments, return an error result
			errMsg := fmt.Sprintf("failed to parse request arguments: %v", err)
			return mcp.NewToolResultError(errMsg), nil
		}

		// Validate required fields
		if len(req.PolicyPaths) == 0 {
			return mcp.NewToolResultError("at least one policy path is required"), nil
		}
		if len(req.ResourceQueries) == 0 {
			return mcp.NewToolResultError("at least one resource query is required"), nil
		}

		// Process the request
		response, err := s.ProcessApplyRequest(ctx, &req)
		if err != nil {
			errMsg := fmt.Sprintf("failed to process request: %v", err)
			return mcp.NewToolResultError(errMsg), nil
		}

		// Convert the response to JSON
		responseJSON, err := json.Marshal(response)
		if err != nil {
			errMsg := fmt.Sprintf("failed to marshal response: %v", err)
			return mcp.NewToolResultError(errMsg), nil
		}

		// Return the result as a CallToolResult with the response as text content
		return mcp.NewToolResultText(string(responseJSON)), nil
	}
}

// RegisterTool registers the ApplyService tool with the MCP server
func (s *ApplyService) RegisterTool(server *mcpServer.MCPServer) {
	tool := s.GetTool()
	handler := s.ToolHandler()
	server.AddTool(tool, handler)
}
