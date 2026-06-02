package mcp

import "context"

// ToolDef defines the metadata of an MCP tool.
type ToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

// ToolCall represents a request to call an MCP tool.
type ToolCall struct {
	Name      string
	Arguments map[string]any
}

// ToolResult represents the result of an MCP tool call.
type ToolResult struct {
	Content string
	Error   error
}

// MCPClient is the interface for MCP tool calls.
type MCPClient interface {
	CallTool(ctx context.Context, serverName, toolName string, args map[string]any) (*ToolResult, error)
	ListTools(ctx context.Context, serverName string) ([]ToolDef, error)
}
