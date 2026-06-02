package skill

import (
	"context"
	"fmt"
	"sync"
)

// Registry manages all registered skills and dispatches requests to the appropriate handler.
type Registry struct {
	mu     sync.RWMutex
	skills map[string]Skill       // name -> skill
	states map[string]*SkillState // sessionID -> state
}

// NewRegistry creates a new skill registry.
func NewRegistry() *Registry {
	return &Registry{
		skills: make(map[string]Skill),
		states: make(map[string]*SkillState),
	}
}

// Register adds a skill to the registry.
func (r *Registry) Register(skill Skill) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.skills[skill.Name()] = skill
}

// Dispatch routes a request to the correct skill based on sub-intent.
func (r *Registry) Dispatch(ctx context.Context, sessionID string, subIntent string, input string) (*SkillResponse, error) {
	r.mu.RLock()
	skill, err := r.findSkill(subIntent)
	r.mu.RUnlock()
	if err != nil {
		return nil, err
	}

	// Get or create state
	r.mu.Lock()
	state, ok := r.states[sessionID]
	if !ok {
		state, err = skill.NewSession(ctx, subIntent)
		if err != nil {
			r.mu.Unlock()
			return nil, fmt.Errorf("failed to create skill session: %w", err)
		}
		r.states[sessionID] = state
	}
	r.mu.Unlock()

	// Handle the input
	resp, err := skill.Handle(ctx, state, input)
	if err != nil {
		return nil, err
	}

	// Cleanup if complete
	if resp.IsComplete {
		r.mu.Lock()
		delete(r.states, sessionID)
		r.mu.Unlock()
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
