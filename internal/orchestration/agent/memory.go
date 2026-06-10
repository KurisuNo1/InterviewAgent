package agent

import "sync"

// MemoryEvent records a write to shared memory for audit purposes.
type MemoryEvent struct {
	Agent string
	Key   string
}

// SharedMemory provides inter-agent collaboration through a key-value store.
// Agents write their findings and downstream agents can read them.
type SharedMemory struct {
	mu     sync.RWMutex
	data   map[string]any
	events []MemoryEvent
}

// NewSharedMemory creates a new shared memory instance.
func NewSharedMemory() *SharedMemory {
	return &SharedMemory{data: make(map[string]any)}
}

// Write stores a value under a keyed by agent+key.
func (m *SharedMemory) Write(agent, key string, value any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[agent+":"+key] = value
	m.events = append(m.events, MemoryEvent{Agent: agent, Key: key})
}

// Read retrieves a value written by a specific agent.
func (m *SharedMemory) Read(agent, key string) (any, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.data[agent+":"+key]
	return v, ok
}

// ReadAny tries to read a key from any agent (returns first match).
func (m *SharedMemory) ReadAny(key string) (any, string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for k, v := range m.data {
		if len(k) > len(key)+1 && k[len(k)-len(key):] == key {
			agent := k[:len(k)-len(key)-1]
			return v, agent, true
		}
	}
	return nil, "", false
}

// Snapshot returns all data for debugging.
func (m *SharedMemory) Snapshot() map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make(map[string]any, len(m.data))
	for k, v := range m.data {
		result[k] = v
	}
	return result
}
