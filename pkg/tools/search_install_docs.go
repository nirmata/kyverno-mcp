package tools

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func SearchInstallDocs(s *server.MCPServer) {
	s.AddTool(mcp.NewTool("search_install_docs",
		mcp.WithDescription("Search the Kyverno install docs for a given query"),
		mcp.WithString("query",
			mcp.Description("The query to search the install docs for"),
			mcp.Required(),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query := mcp.ParseString(req, "query", "")
		limit := mcp.ParseInt(req, "limit", 1)

		library := NewLibrary()

		results := library.Search(query, limit)
		if len(results) == 0 {
			return mcp.NewToolResultError("No documents found"), nil
		}

		content, err := library.Fetch(results)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return mcp.NewToolResultText(content), nil
	})
}
