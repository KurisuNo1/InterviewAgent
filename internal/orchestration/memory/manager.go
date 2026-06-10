package memory

import (
	"context"
	"fmt"
	"log"

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
// It writes to both Redis (fast, ephemeral) and MySQL (durable, for recovery).
func (m *Manager) AppendConversation(ctx context.Context, sessionID string, msg model.Message) error {
	if err := m.short.Append(ctx, sessionID, msg); err != nil {
		return err
	}
	// MySQL save is best-effort; don't fail if it errors
	if err := m.long.SaveMessage(ctx, sessionID, msg); err != nil {
		log.Printf("Warning: failed to persist message to MySQL for session %s: %v", sessionID, err)
	}
	return nil
}

// GetConversationContext returns recent conversation messages for LLM prompting.
// Falls back to MySQL if Redis returns no messages (e.g. after TTL expiry).
func (m *Manager) GetConversationContext(ctx context.Context, sessionID string, window int) ([]model.Message, error) {
	msgs, err := m.short.GetRecent(ctx, sessionID, window)
	if err != nil || len(msgs) == 0 {
		// Redis miss — fall back to MySQL
		mysqlMsgs, mysqlErr := m.long.GetMessages(ctx, sessionID, window)
		if mysqlErr != nil {
			return msgs, err // return original result if MySQL also fails
		}
		return mysqlMsgs, nil
	}
	return msgs, nil
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

// GetInterviewResult retrieves a stored report from MySQL by session ID.
func (m *Manager) GetInterviewResult(ctx context.Context, sessionID string) (*model.Report, error) {
	return m.long.GetReport(ctx, sessionID)
}

// GetReviewPlanFromDB retrieves a stored review plan from MySQL by session ID.
func (m *Manager) GetReviewPlanFromDB(ctx context.Context, sessionID string) (*model.ReviewPlan, error) {
	return m.long.GetReviewPlan(ctx, sessionID)
}

// ListSessionSummaries returns all sessions for a user with basic info.
func (m *Manager) ListSessionSummaries(ctx context.Context, userID string) ([]SessionSummary, error) {
	return m.long.ListSessionSummaries(ctx, userID)
}

// ClearConversation removes a session's conversation history.
func (m *Manager) ClearConversation(ctx context.Context, sessionID string) error {
	return m.short.Clear(ctx, sessionID)
}
