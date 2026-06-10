package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
)

// ServerConfig holds configuration for an MCP server.
type ServerConfig struct {
	Name    string
	Command string
	Args    []string
	Env     map[string]string
}

// serverInstance tracks state for a single MCP server connection.
type serverInstance struct {
	cfg ServerConfig
	cli *client.Client
}

// Manager manages multiple MCP server connections.
type Manager struct {
	configs map[string]ServerConfig
	mu      sync.RWMutex
	clients map[string]*serverInstance
}

// NewManager creates a new MCP Manager.
func NewManager(servers []ServerConfig) *Manager {
	m := &Manager{
		configs: make(map[string]ServerConfig),
		clients: make(map[string]*serverInstance),
	}
	for _, s := range servers {
		m.configs[s.Name] = s
	}
	return m
}

// Start connects to all configured MCP servers.
func (m *Manager) Start(ctx context.Context) error {
	for name, cfg := range m.configs {
		if err := m.connect(ctx, name, cfg); err != nil {
			return fmt.Errorf("failed to connect to MCP server %s: %w", name, err)
		}
	}
	return nil
}

// connect establishes a connection to a single MCP server.
func (m *Manager) connect(ctx context.Context, name string, cfg ServerConfig) error {
	// Resolve environment variables in args and env
	args := make([]string, len(cfg.Args))
	for i, arg := range cfg.Args {
		args[i] = os.ExpandEnv(arg)
	}
	envVars := make([]string, 0, len(cfg.Env))
	for k, v := range cfg.Env {
		envVars = append(envVars, fmt.Sprintf("%s=%s", k, os.ExpandEnv(v)))
	}

	// Create stdio transport
	stdioTransport := transport.NewStdio(cfg.Command, envVars, args...)

	// Create MCP client
	cli := client.NewClient(stdioTransport)

	// Start the stdio subprocess first
	if err := cli.Start(ctx); err != nil {
		cli.Close()
		return fmt.Errorf("failed to start MCP server %s: %w", name, err)
	}

	// Initialize the session
	connectCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{
		Name:    "InterviewAgent",
		Version: "1.0.0",
	}

	_, err := cli.Initialize(connectCtx, initReq)
	if err != nil {
		cli.Close()
		return fmt.Errorf("failed to initialize MCP server %s: %w", name, err)
	}

	m.mu.Lock()
	m.clients[name] = &serverInstance{cfg: cfg, cli: cli}
	m.mu.Unlock()

	return nil
}

// CallTool calls a tool on the specified MCP server.
func (m *Manager) CallTool(ctx context.Context, serverName, toolName string, args map[string]any) (*ToolResult, error) {
	m.mu.RLock()
	inst, ok := m.clients[serverName]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("MCP server %s not connected", serverName)
	}

	callReq := mcp.CallToolRequest{}
	callReq.Params.Name = toolName
	callReq.Params.Arguments = args // map[string]any → JSON object (NOT []byte → base64)

	result, err := inst.cli.CallTool(ctx, callReq)
	if err != nil {
		return &ToolResult{Error: fmt.Errorf("tool call failed: %w", err)}, err
	}

	// Extract text content from result
	var content string
	for _, c := range result.Content {
		if textContent, ok := c.(mcp.TextContent); ok {
			content += textContent.Text
		}
	}

	return &ToolResult{Content: content}, nil
}

// ListTools lists available tools on the specified MCP server.
func (m *Manager) ListTools(ctx context.Context, serverName string) ([]ToolDef, error) {
	m.mu.RLock()
	inst, ok := m.clients[serverName]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("MCP server %s not connected", serverName)
	}

	listReq := mcp.ListToolsRequest{}
	result, err := inst.cli.ListTools(ctx, listReq)
	if err != nil {
		return nil, fmt.Errorf("list tools failed: %w", err)
	}

	tools := make([]ToolDef, 0, len(result.Tools))
	for _, t := range result.Tools {
		// Convert ToolInputSchema to map[string]any
		schemaMap := make(map[string]any)
		schemaBytes, _ := json.Marshal(t.InputSchema)
		json.Unmarshal(schemaBytes, &schemaMap)

		tools = append(tools, ToolDef{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: schemaMap,
		})
	}
	return tools, nil
}

// ServerNames returns the names of all configured MCP servers.
func (m *Manager) ServerNames() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	names := make([]string, 0, len(m.configs))
	for name := range m.configs {
		names = append(names, name)
	}
	return names
}

// Close disconnects from all MCP servers.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for name, inst := range m.clients {
		inst.cli.Close()
		delete(m.clients, name)
	}
}
