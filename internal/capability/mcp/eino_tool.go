package mcp

import (
	"context"
	"encoding/json"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/eino-contrib/jsonschema"
)

// EinoTool wraps an MCP tool as an Eino InvokableTool so all MCP calls flow
// through Eino's framework and benefit from unified callbacks and observability.
type EinoTool struct {
	serverName  string
	toolName    string
	desc        string
	inputSchema map[string]any
	manager     *Manager

	// callbackHandler is invoked at OnStart/OnEnd/OnError lifecycle points.
	// Built via observability.NewToolCallbackHandler() using callbacks.NewHandlerBuilder().
	callbackHandler callbacks.Handler
}

// NewEinoTool creates an Eino InvokableTool from an MCP ToolDef.
func NewEinoTool(serverName string, def ToolDef, manager *Manager, handler callbacks.Handler) tool.InvokableTool {
	return &EinoTool{
		serverName:      serverName,
		toolName:        def.Name,
		desc:            def.Description,
		inputSchema:     def.InputSchema,
		manager:         manager,
		callbackHandler: handler,
	}
}

// Info returns tool metadata for the LLM to decide when to call this tool.
func (t *EinoTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	info := &schema.ToolInfo{
		Name:  t.serverName + "_" + t.toolName,
		Desc:  t.desc,
		Extra: map[string]any{"mcp_server": t.serverName, "mcp_tool": t.toolName},
	}

	if t.inputSchema != nil {
		js, err := convertToJSONSchema(t.inputSchema)
		if err == nil && js != nil {
			info.ParamsOneOf = schema.NewParamsOneOfByJSONSchema(js)
		}
	}

	return info, nil
}

// InvokableRun executes the MCP tool and returns the result as a JSON string.
// It fires the Eino callback handler at OnStart/OnEnd/OnError for unified observability.
func (t *EinoTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	// Parse arguments
	var args map[string]any
	if argumentsInJSON != "" {
		if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
			args = map[string]any{"query": argumentsInJSON}
		}
	}

	// Build RunInfo for callback context
	runInfo := &callbacks.RunInfo{
		Name:      t.serverName + "_" + t.toolName,
		Type:      "MCP",
		Component: "Tool",
	}

	// --- OnStart callback ---
	ctx = t.callbackHandler.OnStart(ctx, runInfo, &tool.CallbackInput{
		ArgumentsInJSON: argumentsInJSON,
	})

	// Delegate to MCP manager
	result, err := t.manager.CallTool(ctx, t.serverName, t.toolName, args)

	if err != nil {
		// --- OnError callback ---
		t.callbackHandler.OnError(ctx, runInfo, err)
		return "", err
	}
	if result.Error != nil {
		t.callbackHandler.OnError(ctx, runInfo, result.Error)
		return "", result.Error
	}

	// --- OnEnd callback ---
	ctx = t.callbackHandler.OnEnd(ctx, runInfo, &tool.CallbackOutput{
		Response: result.Content,
	})

	return result.Content, nil
}

// convertToJSONSchema converts an MCP InputSchema (map[string]any in JSON Schema format)
// into an Eino jsonschema.Schema by marshalling through JSON.
func convertToJSONSchema(mcpSchema map[string]any) (*jsonschema.Schema, error) {
	if len(mcpSchema) == 0 {
		return nil, nil
	}
	data, err := json.Marshal(mcpSchema)
	if err != nil {
		return nil, err
	}
	var s jsonschema.Schema
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}
