package memory

import (
	"context"
	"fmt"

	"github.com/KurisuNo1/InterviewAgent/internal/model"
)

// Manager combines short-term and long-term memory into a single facade.
type Manager struct {
	short *ShortTermMemory
	long  *LongTermMemory
}

// NewManager creates a new memory manager.
func NewManager(short *ShortTermMemory, long *LongTermMemory) *Manager {
	return &Manager{short: short, long: long}
}

// AppendConversation adds a message to the session's conversation history.
func (m *Manager) AppendConversation(ctx context.Context, sessionID string, msg model.Message) error {
	return m.short.Append(ctx, sessionID, msg)
}

// GetConversationContext returns recent conversation messages for LLM prompting.
func (m *Manager) GetConversationContext(ctx context.Context, sessionID string, window int) ([]model.Message, error) {
	return m.short.GetRecent(ctx, sessionID, window)
}

// SaveSession persists a session to long-term storage.
func (m *Manager) SaveSession(ctx context.Context, session *model.Session) error {
	return m.long.SaveSession(ctx, session)
}

// UpdateSessionStatus updates the session phase.
func (m *Manager) UpdateSessionStatus(ctx context.Context, sessionID string, status model.InterviewPhase) error {
	return m.long.UpdateSession(ctx, sessionID, status)
}

// SaveInterviewResult persists the final report and review plan.
// It ensures the parent session exists first to satisfy foreign key constraints.
func (m *Manager) SaveInterviewResult(ctx context.Context, report *model.Report, plan *model.ReviewPlan) error {
	if err := m.long.EnsureSession(ctx, report.SessionID); err != nil {
		return fmt.Errorf("ensure session: %w", err)
	}
	if err := m.long.SaveReport(ctx, report); err != nil {
		return err
	}
	if plan != nil {
		if err := m.long.SaveReviewPlan(ctx, plan); err != nil {
			return err
		}
	}
	return nil
}

// GetUserHistory retrieves past interview results for context.
func (m *Manager) GetUserHistory(ctx context.Context, userID string, limit int) ([]*model.Report, error) {
	return m.long.GetUserHistory(ctx, userID, limit)
}

// ClearConversation removes a session's conversation history.
func (m *Manager) ClearConversation(ctx context.Context, sessionID string) error {
	return m.short.Clear(ctx, sessionID)
}
