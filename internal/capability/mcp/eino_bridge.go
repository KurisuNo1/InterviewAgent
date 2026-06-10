package mcp

import (
	"context"
	"fmt"
	"log"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components/tool"
)

// EinoBridge connects MCP servers to Eino's tool framework.
// It discovers all tools from configured MCP servers and provides them
// as Eino InvokableTool instances, so all MCP calls flow through Eino.
type EinoBridge struct {
	manager         *Manager
	tools           map[string]tool.InvokableTool // fullName (server_tool) -> tool
	toolCallback    callbacks.Handler             // injected into every EinoTool
}

// NewEinoBridge creates a bridge and discovers all tools from the MCP manager.
// toolHandler is a callbacks.Handler (built via callbacks.NewHandlerBuilder)
// that will be invoked at OnStart/OnEnd/OnError for every MCP tool invocation.
func NewEinoBridge(ctx context.Context, manager *Manager, toolHandler callbacks.Handler) *EinoBridge {
	bridge := &EinoBridge{
		manager:      manager,
		tools:        make(map[string]tool.InvokableTool),
		toolCallback: toolHandler,
	}
	bridge.discoverTools(ctx)
	return bridge
}

// discoverTools queries all MCP servers for their tools and wraps them.
func (b *EinoBridge) discoverTools(ctx context.Context) {
	for _, serverName := range b.manager.ServerNames() {
		tools, err := b.manager.ListTools(ctx, serverName)
		if err != nil {
			log.Printf("[mcp-eino] WARNING: failed to list tools for MCP server %q: %v", serverName, err)
			continue
		}
		for _, def := range tools {
			einoTool := NewEinoTool(serverName, def, b.manager, b.toolCallback)
			fullName := serverName + "_" + def.Name
			b.tools[fullName] = einoTool
		}
		log.Printf("[mcp-eino] Server %q: %d tools discovered", serverName, len(tools))
	}
	log.Printf("[mcp-eino] Bridge ready: %d total tools across %d servers", len(b.tools), len(b.manager.ServerNames()))
}

// GetTool returns an Eino tool by its full name (e.g. "github_search_repositories").
func (b *EinoBridge) GetTool(fullName string) (tool.InvokableTool, error) {
	t, ok := b.tools[fullName]
	if !ok {
		return nil, fmt.Errorf("MCP tool %q not found", fullName)
	}
	return t, nil
}

// CallTool is a convenience method that looks up the Eino tool by name and
// executes it with the given JSON arguments string.
func (b *EinoBridge) CallTool(ctx context.Context, fullName string, argumentsInJSON string) (string, error) {
	t, err := b.GetTool(fullName)
	if err != nil {
		return "", err
	}
	return t.InvokableRun(ctx, argumentsInJSON)
}

// FindToolByServer finds the first tool from a given MCP server.
// Useful when the exact tool name is unknown — tries common names first, then any match.
func (b *EinoBridge) FindToolByServer(serverName string, preferredNames ...string) (tool.InvokableTool, string, error) {
	// Try preferred names first
	for _, name := range preferredNames {
		fullName := serverName + "_" + name
		if t, ok := b.tools[fullName]; ok {
			return t, fullName, nil
		}
	}
	// Fall back to any tool from this server
	for fullName, t := range b.tools {
		if len(fullName) > len(serverName) && fullName[:len(serverName)+1] == serverName+"_" {
			return t, fullName, nil
		}
	}
	return nil, "", fmt.Errorf("no tool found for MCP server %q", serverName)
}

// GetAllTools returns all discovered tools as Eino InvokableTool instances.
// Useful for passing to graph nodes that support WithTools.
func (b *EinoBridge) GetAllTools() []tool.InvokableTool {
	result := make([]tool.InvokableTool, 0, len(b.tools))
	for _, t := range b.tools {
		result = append(result, t)
	}
	return result
}

// ToolSummary is a lightweight description of an available tool for the frontend.
type ToolSummary struct {
	Name        string `json:"name"`
	Server      string `json:"server"`
	Description string `json:"description"`
}

// ListToolSummaries returns a list of all available tools for API exposure.
func (b *EinoBridge) ListToolSummaries(ctx context.Context) []ToolSummary {
	var result []ToolSummary
	for name, t := range b.tools {
		info, err := t.Info(ctx)
		if err != nil {
			continue
		}
		server := ""
		if s, ok := info.Extra["mcp_server"]; ok {
			server = fmt.Sprintf("%v", s)
		}
		result = append(result, ToolSummary{
			Name:        name,
			Server:      server,
			Description: info.Desc,
		})
	}
	return result
}

// GetToolInfos returns all tool metadata for use in LLM prompts.
func (b *EinoBridge) GetToolInfos(ctx context.Context) []string {
	var infos []string
	for name, t := range b.tools {
		info, err := t.Info(ctx)
		if err != nil {
			continue
		}
		infos = append(infos, fmt.Sprintf("- %s: %s", name, info.Desc))
	}
	return infos
}
