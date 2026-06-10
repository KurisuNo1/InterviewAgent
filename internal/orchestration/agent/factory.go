package agent

import (
	"context"
	"fmt"
	"log"

	"github.com/cloudwego/eino/compose"
	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/components/tool"
)

// AgentFactory creates and manages ReAct agents with shared tool sets.
type AgentFactory struct {
	chatModel  einomodel.ToolCallingChatModel
	allTools   []tool.BaseTool
	toolByName map[string]tool.BaseTool
	thinker    ThinkLogger
}

// NewAgentFactory creates a new agent factory.
// tools should be the full set of available MCP tools.
func NewAgentFactory(chatModel einomodel.ToolCallingChatModel, tools []tool.BaseTool, thinker ThinkLogger) *AgentFactory {
	toolByName := make(map[string]tool.BaseTool)
	for _, t := range tools {
		info, err := t.Info(context.Background())
		if err == nil {
			toolByName[info.Name] = t
		}
	}
	log.Printf("[AgentFactory] Initialized with %d tools available", len(tools))
	return &AgentFactory{
		chatModel:  chatModel,
		allTools:   tools,
		toolByName: toolByName,
		thinker:    thinker,
	}
}

// NewAgent creates a ReAct agent with optional tool filtering.
// If toolFilter is empty or nil, all tools are available.
// If toolFilter contains names, only matching tools are included (prefix match).
func (f *AgentFactory) NewAgent(ctx context.Context, name string, toolFilter []string, maxSteps int) (*react.Agent, error) {
	tools := f.filterTools(toolFilter)
	if maxSteps <= 0 {
		maxSteps = 8
	}

	agent, err := react.NewAgent(ctx, &react.AgentConfig{
		ToolCallingModel: f.chatModel,
		ToolsConfig:      compose.ToolsNodeConfig{Tools: tools},
		MaxStep:          maxSteps,
		GraphName:        name,
	})
	if err != nil {
		return nil, fmt.Errorf("create agent %s: %w", name, err)
	}

	log.Printf("[AgentFactory] Agent %q created: %d tools, maxSteps=%d", name, len(tools), maxSteps)
	return agent, nil
}

// filterTools returns tools matching the given name prefixes.
func (f *AgentFactory) filterTools(names []string) []tool.BaseTool {
	if len(names) == 0 {
		return f.allTools
	}
	var result []tool.BaseTool
	for _, name := range names {
		if t, ok := f.toolByName[name]; ok {
			result = append(result, t)
		}
	}
	if len(result) == 0 {
		// If no tools match, return all — better to have tools than none
		log.Printf("[AgentFactory] WARNING: no tools matched filter %v, using all %d tools", names, len(f.allTools))
		return f.allTools
	}
	return result
}

// Thinker returns the shared thinking logger.
func (f *AgentFactory) Thinker() ThinkLogger { return f.thinker }
