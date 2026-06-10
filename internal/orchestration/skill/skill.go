package skill

import "context"

// SkillState holds the state for a multi-round skill practice session.
type SkillState struct {
	SessionID  string         `json:"session_id"`
	SkillName  string         `json:"skill_name"`
	SubIntent  string         `json:"sub_intent"`
	Round      int            `json:"round"`
	History    []string       `json:"history"`
	Data       map[string]any `json:"data"`
	IsComplete bool           `json:"is_complete"`
}

// SkillResponse is the response from a skill handler.
type SkillResponse struct {
	Message      string `json:"message"`
	IsComplete   bool   `json:"is_complete"`
	NextPrompt   string `json:"next_prompt,omitempty"`
	CaptureInput bool   `json:"capture_input,omitempty"` // when true, input is raw topic extraction
}

// Skill defines the interface for a pluggable practice module.
type Skill interface {
	// Name returns the skill's unique identifier.
	Name() string
	// Description describes what this skill does.
	Description() string
	// Category returns the skill category: "core" for interview agent skills, "training" for specialized training.
	Category() string
	// WelcomeMessage returns the initial prompt shown when starting this skill.
	WelcomeMessage() string
	// CanHandle checks if this skill can handle the given sub-intent.
	CanHandle(subIntent string) bool
	// Handle processes one round of the skill interaction.
	Handle(ctx context.Context, state *SkillState, input string) (*SkillResponse, error)
	// NewSession initializes a new skill session.
	NewSession(ctx context.Context, subIntent string) (*SkillState, error)
}
