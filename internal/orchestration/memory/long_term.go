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
