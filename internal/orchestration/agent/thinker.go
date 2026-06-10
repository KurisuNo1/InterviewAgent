package agent

import (
	"log"
	"sync"
	"time"
)

// ConsoleThinker logs agent thinking to console and keeps an in-memory trace.
type ConsoleThinker struct {
	mu     sync.RWMutex
	traces []ThinkLog
}

// NewConsoleThinker creates a new console-based thinking logger.
func NewConsoleThinker() *ConsoleThinker {
	return &ConsoleThinker{traces: make([]ThinkLog, 0)}
}

// Log records a thinking step to console and the internal trace buffer.
func (t *ConsoleThinker) Log(agentName, phase, content string) {
	entry := ThinkLog{
		AgentName: agentName,
		Phase:     phase,
		Content:   content,
		Timestamp: time.Now(),
	}
	t.mu.Lock()
	t.traces = append(t.traces, entry)
	t.mu.Unlock()

	// Keep console output compact — truncate long content
	display := content
	if len(display) > 120 {
		display = display[:120] + "..."
	}
	log.Printf("[Agent:%s][%s] %s", agentName, phase, display)
}

// GetTraces returns thinking traces for a specific agent.
func (t *ConsoleThinker) GetTraces(agentName string) []ThinkLog {
	t.mu.RLock()
	defer t.mu.RUnlock()
	var result []ThinkLog
	for _, entry := range t.traces {
		if entry.AgentName == agentName {
			result = append(result, entry)
		}
	}
	return result
}

// AllTraces returns all recorded thinking traces.
func (t *ConsoleThinker) AllTraces() []ThinkLog {
	t.mu.RLock()
	defer t.mu.RUnlock()
	result := make([]ThinkLog, len(t.traces))
	copy(result, t.traces)
	return result
}
