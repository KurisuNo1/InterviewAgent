package contextmanager

import (
	"context"
	"log"

	"github.com/KurisuNo1/InterviewAgent/internal/model"
	"github.com/KurisuNo1/InterviewAgent/internal/orchestration/memory"
)

// MemoryHierarchy coordinates the three memory layers during LLM context assembly.
//
//	Layer 0: Working Memory — assembled fresh per LLM call by ContextBuilder
//	Layer 1: Short-Term   — Redis, full recent messages (30 turns)
//	Layer 2: Long-Term    — MySQL, full archive + summaries + reports
type MemoryHierarchy struct {
	mgr        *memory.Manager
	compressor *ConversationCompressor
}

// NewMemoryHierarchy creates a new memory hierarchy coordinator.
func NewMemoryHierarchy(mgr *memory.Manager, compressor *ConversationCompressor) *MemoryHierarchy {
	return &MemoryHierarchy{mgr: mgr, compressor: compressor}
}

// FetchWorkingMemory loads conversation history from short-term storage
// and prepares it for ContextBuilder consumption.
// window is the number of recent messages to fetch.
func (h *MemoryHierarchy) FetchWorkingMemory(ctx context.Context, sessionID string, window int) ([]model.Message, error) {
	if h.mgr == nil {
		return nil, nil
	}
	msgs, err := h.mgr.GetConversationContext(ctx, sessionID, window)
	if err != nil {
		log.Printf("[MemoryHierarchy] FetchWorkingMemory error: %v", err)
		return nil, nil
	}
	return msgs, nil
}

// ArchiveSessionSummary generates and persists a summary after a session ends.
// Called asynchronously; failures are logged but not propagated.
func (h *MemoryHierarchy) ArchiveSessionSummary(ctx context.Context, sessionID string, messages []model.Message) {
	if h.compressor == nil || h.mgr == nil || len(messages) == 0 {
		return
	}

	summary, err := h.compressor.SummarizeWithLLM(ctx, messages)
	if err != nil {
		log.Printf("[MemoryHierarchy] ArchiveSessionSummary failed for %s: %v", sessionID, err)
		return
	}
	if summary == "" {
		return
	}

	// Persist summary as a system message in long-term memory
	summaryMsg := model.Message{
		Role:    model.RoleSystem,
		Content: "[Session Summary] " + summary,
	}
	if err := h.mgr.AppendConversation(ctx, sessionID, summaryMsg); err != nil {
		log.Printf("[MemoryHierarchy] Failed to persist summary for %s: %v", sessionID, err)
	} else {
		log.Printf("[MemoryHierarchy] Session summary archived for %s (%d chars)", sessionID, len(summary))
	}
}

// LoadUserContext fetches relevant long-term context for a returning user.
// Returns a summary string suitable for injection into the system prompt.
func (h *MemoryHierarchy) LoadUserContext(ctx context.Context, userID string) string {
	if h.mgr == nil || userID == "" {
		return ""
	}

	reports, err := h.mgr.GetUserHistory(ctx, userID, 3)
	if err != nil || len(reports) == 0 {
		return ""
	}

	var weakAreas []string
	for _, r := range reports {
		weakAreas = append(weakAreas, r.WeakAreas...)
	}

	if len(weakAreas) == 0 {
		return ""
	}

	// Deduplicate
	seen := make(map[string]bool)
	unique := make([]string, 0, len(weakAreas))
	for _, w := range weakAreas {
		if !seen[w] {
			seen[w] = true
			unique = append(unique, w)
		}
	}

	result := "## User Context (from previous sessions)\n"
	result += "Previously identified weak areas: "
	for i, w := range unique {
		if i > 0 {
			result += ", "
		}
		result += w
	}
	result += ".\nFocus on these areas during the interview."

	return result
}
