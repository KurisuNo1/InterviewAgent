package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/KurisuNo1/InterviewAgent/internal/capability/store"
	"github.com/KurisuNo1/InterviewAgent/internal/model"
)

// LongTermMemory stores interview results in MySQL.
type LongTermMemory struct {
	mysql      store.MySQLClient
	maxHistory int
}

// LongTermConfig holds configuration for long-term memory.
type LongTermConfig struct {
	MaxHistory int
}

// NewLongTermMemory creates a new long-term memory store.
func NewLongTermMemory(mysql store.MySQLClient, cfg LongTermConfig) *LongTermMemory {
	return &LongTermMemory{mysql: mysql, maxHistory: cfg.MaxHistory}
}

// SaveSession saves an interview session to MySQL. Uses INSERT IGNORE so it is
// safe to call multiple times for the same session.
func (l *LongTermMemory) SaveSession(ctx context.Context, session *model.Session) error {
	_, err := l.mysql.Exec(ctx,
		"INSERT IGNORE INTO interview_sessions (id, user_id, status, jd_text, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
		session.ID, session.UserID, string(session.Status), session.JDText, session.CreatedAt, session.UpdatedAt,
	)
	return err
}

// EnsureSession creates a minimal session row if one does not exist.
// This prevents foreign key violations when saving results for a session
// whose creation may have failed to persist.
func (l *LongTermMemory) EnsureSession(ctx context.Context, sessionID string) error {
	_, err := l.mysql.Exec(ctx,
		"INSERT IGNORE INTO interview_sessions (id, status, created_at, updated_at) VALUES (?, 'completed', NOW(), NOW())",
		sessionID,
	)
	return err
}

// UpdateSession updates session status.
func (l *LongTermMemory) UpdateSession(ctx context.Context, sessionID string, status model.InterviewPhase) error {
	_, err := l.mysql.Exec(ctx,
		"UPDATE interview_sessions SET status = ?, updated_at = ? WHERE id = ?",
		string(status), time.Now(), sessionID,
	)
	return err
}

// SaveReport saves interview results to MySQL.
// Uses ON DUPLICATE KEY UPDATE so repeated calls (e.g. GetReport then GetReviewPlan)
// safely update the existing row instead of failing.
func (l *LongTermMemory) SaveReport(ctx context.Context, report *model.Report) error {
	evalJSON, _ := json.Marshal(report.Evaluations)
	dimJSON, _ := json.Marshal(report.DimensionScore)
	reportJSON, _ := json.Marshal(report)

	id := fmt.Sprintf("result-%s", report.SessionID)
	_, err := l.mysql.Exec(ctx,
		`INSERT INTO interview_results (id, session_id, evaluations, overall_score, dimension_scores, report_json, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE
		   evaluations = VALUES(evaluations),
		   overall_score = VALUES(overall_score),
		   dimension_scores = VALUES(dimension_scores),
		   report_json = VALUES(report_json)`,
		id, report.SessionID, string(evalJSON), report.OverallScore, string(dimJSON), string(reportJSON), time.Now(),
	)
	return err
}

// SaveReviewPlan saves a review plan to MySQL.
// Uses ON DUPLICATE KEY UPDATE for the same idempotency reason as SaveReport.
func (l *LongTermMemory) SaveReviewPlan(ctx context.Context, plan *model.ReviewPlan) error {
	planJSON, _ := json.Marshal(plan.PlanItems)
	resJSON, _ := json.Marshal(plan.Resources)

	id := fmt.Sprintf("plan-%s", plan.SessionID)
	_, err := l.mysql.Exec(ctx,
		`INSERT INTO review_plans (id, session_id, plan_json, resources_json, created_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE
		   plan_json = VALUES(plan_json),
		   resources_json = VALUES(resources_json)`,
		id, plan.SessionID, string(planJSON), string(resJSON), time.Now(),
	)
	return err
}

// GetReport retrieves a stored report from MySQL by session ID.
func (l *LongTermMemory) GetReport(ctx context.Context, sessionID string) (*model.Report, error) {
	var reportJSON string
	err := l.mysql.QueryRow(ctx,
		"SELECT report_json FROM interview_results WHERE session_id = ?", sessionID,
	).Scan(&reportJSON)
	if err != nil {
		return nil, fmt.Errorf("report not found for session %s: %w", sessionID, err)
	}
	var report model.Report
	if err := json.Unmarshal([]byte(reportJSON), &report); err != nil {
		return nil, fmt.Errorf("failed to parse report: %w", err)
	}
	return &report, nil
}

// GetReviewPlan retrieves a stored review plan from MySQL by session ID.
func (l *LongTermMemory) GetReviewPlan(ctx context.Context, sessionID string) (*model.ReviewPlan, error) {
	var planJSON, resJSON string
	err := l.mysql.QueryRow(ctx,
		"SELECT plan_json, resources_json FROM review_plans WHERE session_id = ?", sessionID,
	).Scan(&planJSON, &resJSON)
	if err != nil {
		return nil, fmt.Errorf("review plan not found for session %s: %w", sessionID, err)
	}
	var plan model.ReviewPlan
	if err := json.Unmarshal([]byte(planJSON), &plan.PlanItems); err != nil {
		return nil, fmt.Errorf("failed to parse review plan items: %w", err)
	}
	if err := json.Unmarshal([]byte(resJSON), &plan.Resources); err != nil {
		return nil, fmt.Errorf("failed to parse review plan resources: %w", err)
	}
	plan.SessionID = sessionID
	return &plan, nil
}

// SessionSummary is a lightweight view of a past interview session.
type SessionSummary struct {
	ID           string  `json:"id"`
	Status       string  `json:"status"`
	OverallScore float64 `json:"overall_score"`
	CreatedAt    string  `json:"created_at"`
	LastMessage  string  `json:"last_message,omitempty"`
}

// ListSessionSummaries returns all sessions for a user with basic info.
func (l *LongTermMemory) ListSessionSummaries(ctx context.Context, userID string) ([]SessionSummary, error) {
	rows, err := l.mysql.Query(ctx,
		`SELECT s.id, s.status, COALESCE(r.overall_score, 0), s.created_at,
		 COALESCE((SELECT content FROM chat_messages cm WHERE cm.session_id = s.id ORDER BY cm.created_at DESC LIMIT 1), '') AS last_msg
		 FROM interview_sessions s
		 LEFT JOIN interview_results r ON r.session_id = s.id
		 WHERE s.user_id = ?
		 ORDER BY s.created_at DESC LIMIT 50`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list sessions: %w", err)
	}
	defer rows.Close()

	var summaries []SessionSummary
	for rows.Next() {
		var s SessionSummary
		var createdAt time.Time
		var lastMsg string
		if err := rows.Scan(&s.ID, &s.Status, &s.OverallScore, &createdAt, &lastMsg); err != nil {
			continue
		}
		s.CreatedAt = createdAt.Format("2006-01-02 15:04")
		s.LastMessage = lastMsg
		summaries = append(summaries, s)
	}
	return summaries, nil
}

// SaveMessage persists a chat message to MySQL for long-term recovery.
func (l *LongTermMemory) SaveMessage(ctx context.Context, sessionID string, msg model.Message) error {
	_, err := l.mysql.Exec(ctx,
		"INSERT INTO chat_messages (session_id, role, content) VALUES (?, ?, ?)",
		sessionID, string(msg.Role), msg.Content,
	)
	return err
}

// GetMessages retrieves chat messages from MySQL for a session.
func (l *LongTermMemory) GetMessages(ctx context.Context, sessionID string, limit int) ([]model.Message, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := l.mysql.Query(ctx,
		"SELECT role, content FROM chat_messages WHERE session_id = ? ORDER BY created_at ASC LIMIT ?",
		sessionID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []model.Message
	for rows.Next() {
		var m model.Message
		if err := rows.Scan(&m.Role, &m.Content); err != nil {
			continue
		}
		msgs = append(msgs, m)
	}
	return msgs, nil
}

// HasMessages checks if a session has any chat messages in MySQL.
func (l *LongTermMemory) HasMessages(ctx context.Context, sessionID string) (bool, error) {
	row := l.mysql.QueryRow(ctx,
		"SELECT COUNT(1) FROM chat_messages WHERE session_id = ?", sessionID,
	)
	var count int
	if err := row.Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

// GetUserHistory retrieves past interview results for a user.
func (l *LongTermMemory) GetUserHistory(ctx context.Context, userID string, limit int) ([]*model.Report, error) {
	if limit <= 0 {
		limit = l.maxHistory
	}

	rows, err := l.mysql.Query(ctx,
		`SELECT ir.report_json FROM interview_results ir
		 JOIN interview_sessions s ON ir.session_id = s.id
		 WHERE s.user_id = ?
		 ORDER BY ir.created_at DESC LIMIT ?`,
		userID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query user history: %w", err)
	}
	defer rows.Close()

	var reports []*model.Report
	for rows.Next() {
		var reportJSON string
		if err := rows.Scan(&reportJSON); err != nil {
			continue
		}
		var report model.Report
		if err := json.Unmarshal([]byte(reportJSON), &report); err != nil {
			continue
		}
		reports = append(reports, &report)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating user history rows: %w", err)
	}
	return reports, nil
}
