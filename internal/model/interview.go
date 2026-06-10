package model

import "time"

// InterviewPhase represents the current phase of an interview session.
type InterviewPhase string

const (
	PhaseCreated          InterviewPhase = "created"
	PhaseActive           InterviewPhase = "active"
	PhaseJDParsing        InterviewPhase = "jd_parsing"
	PhaseResumeMatching   InterviewPhase = "resume_matching"
	PhaseQuestionPlanning InterviewPhase = "question_planning"
	PhaseInterviewing     InterviewPhase = "interviewing"
	PhaseCompleted        InterviewPhase = "completed"
)

// Session represents an interview session.
type Session struct {
	ID        string         `json:"id"`
	UserID    string         `json:"user_id,omitempty"`
	Status    InterviewPhase `json:"status"`
	JDText    string         `json:"jd_text,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}
