package router

import "context"

// Specialist handles a specific intent with its own processing logic.
type Specialist interface {
	// Name returns the specialist's identifier.
	Name() string
	// Description describes what this specialist handles.
	Description() string
	// CanHandle checks if this specialist can handle the given intent/subIntent.
	CanHandle(intent Intent, subIntent string) bool
	// Handle processes the request and returns a response.
	Handle(ctx context.Context, sessionID string, input string, metadata map[string]string) (string, error)
}
