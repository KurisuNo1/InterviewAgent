package skill

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"

	"github.com/cloudwego/eino/compose"
)

const checkpointKeyPrefix = "skill_ckpt:"

// Registry manages all registered skills and dispatches requests with checkpoint persistence.
type Registry struct {
	mu             sync.RWMutex
	skills         map[string]Skill
	checkpointStore compose.CheckPointStore
}

// NewRegistry creates a new skill registry.
func NewRegistry(checkpointStore compose.CheckPointStore) *Registry {
	return &Registry{
		skills:          make(map[string]Skill),
		checkpointStore: checkpointStore,
	}
}

// Register adds a skill to the registry.
func (r *Registry) Register(skill Skill) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.skills[skill.Name()] = skill
}

// Dispatch routes a request to the correct skill with checkpoint-backed state.
func (r *Registry) Dispatch(ctx context.Context, sessionID string, subIntent string, input string, ragDocuments string) (*SkillResponse, error) {
	r.mu.RLock()
	skill, err := r.findSkill(subIntent)
	r.mu.RUnlock()
	if err != nil {
		return nil, err
	}

	// Try to load state from checkpoint
	state, err := r.loadCheckpoint(ctx, sessionID)
	if err != nil || state == nil {
		// Create new session
		state, err = skill.NewSession(ctx, subIntent)
		if err != nil {
			return nil, fmt.Errorf("failed to create skill session: %w", err)
		}
	}

	// Store RAG documents in state for skill to use
	if ragDocuments != "" {
		if state.Data == nil {
			state.Data = make(map[string]any)
		}
		state.Data["rag_documents"] = ragDocuments
	}

	// Handle the input
	resp, err := skill.Handle(ctx, state, input)
	if err != nil {
		return nil, err
	}

	// Persist state via checkpoint (survives server restarts)
	if resp.IsComplete {
		if err := r.deleteCheckpoint(ctx, sessionID); err != nil {
			log.Printf("[SkillRegistry] Warning: failed to delete checkpoint for session %s: %v", sessionID, err)
		}
	} else {
		if err := r.saveCheckpoint(ctx, sessionID, state); err != nil {
			log.Printf("[SkillRegistry] Warning: failed to save checkpoint for session %s: %v", sessionID, err)
		}
	}

	return resp, nil
}

// findSkill finds the skill that can handle the given sub-intent.
func (r *Registry) findSkill(subIntent string) (Skill, error) {
	for _, skill := range r.skills {
		if skill.CanHandle(subIntent) {
			return skill, nil
		}
	}
	return nil, fmt.Errorf("no skill found for sub-intent: %s", subIntent)
}

// List returns all registered skill names and descriptions.
func (r *Registry) List() map[string]string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make(map[string]string)
	for name, skill := range r.skills {
		result[name] = skill.Description()
	}
	return result
}

// Category returns the category of a registered skill.
func (r *Registry) Category(name string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if s, ok := r.skills[name]; ok {
		return s.Category()
	}
	return ""
}

// Welcome returns the welcome message for a skill, or an empty string if not found.
func (r *Registry) Welcome(name string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if s, ok := r.skills[name]; ok {
		return s.WelcomeMessage()
	}
	return ""
}

// saveCheckpoint persists the skill state to the checkpoint store.
func (r *Registry) saveCheckpoint(ctx context.Context, sessionID string, state *SkillState) error {
	if r.checkpointStore == nil {
		return nil
	}
	key := checkpointKeyPrefix + sessionID
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal skill state: %w", err)
	}
	return r.checkpointStore.Set(ctx, key, data)
}

// loadCheckpoint retrieves a saved skill state from the checkpoint store.
func (r *Registry) loadCheckpoint(ctx context.Context, sessionID string) (*SkillState, error) {
	if r.checkpointStore == nil {
		return nil, nil
	}
	key := checkpointKeyPrefix + sessionID
	data, found, err := r.checkpointStore.Get(ctx, key)
	if err != nil || !found || data == nil {
		return nil, nil
	}
	var state SkillState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("unmarshal skill state: %w", err)
	}
	// Ensure maps are initialized after deserialization
	if state.Data == nil {
		state.Data = make(map[string]any)
	}
	log.Printf("[SkillRegistry] Checkpoint loaded for session %s (round=%d, skill=%s)", sessionID, state.Round, state.SkillName)
	return &state, nil
}

// deleteCheckpoint removes a completed session's checkpoint.
func (r *Registry) deleteCheckpoint(ctx context.Context, sessionID string) error {
	if r.checkpointStore == nil {
		return nil
	}
	key := checkpointKeyPrefix + sessionID
	return r.checkpointStore.Set(ctx, key, nil) // nil data effectively deletes
}
