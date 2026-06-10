package agent

import "time"

// AgentResult wraps the output of an agent's reasoning cycle.
type AgentResult struct {
	Content   string   `json:"content"`
	ToolCalls int      `json:"tool_calls"`
	Thinking  []string `json:"thinking,omitempty"`
}

// ThinkLog records a single thinking step in an agent's reasoning cycle.
type ThinkLog struct {
	AgentName string    `json:"agent_name"`
	Phase     string    `json:"phase"` // "plan", "execute", "observe", "reflect", "output"
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// ThinkLogger is the interface for recording agent thinking traces.
type ThinkLogger interface {
	Log(agentName, phase, content string)
	GetTraces(agentName string) []ThinkLog
	AllTraces() []ThinkLog
}
