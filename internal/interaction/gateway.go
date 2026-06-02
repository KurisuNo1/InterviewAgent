package interaction

import (
	"context"

	"github.com/KurisuNo1/InterviewAgent/internal/model"
)

// CreateSessionReq is the request to create a new interview session.
type CreateSessionReq struct {
	UserID string `json:"user_id,omitempty"`
	JDText string `json:"jd_text,omitempty"`
	JDURL  string `json:"jd_url,omitempty"`
}

// MessageResponse is the unified response for HandleMessage calls.
type MessageResponse struct {
	Intent     string               `json:"intent"`
	Reply      string               `json:"reply"`
	Phase      model.InterviewPhase `json:"phase,omitempty"`
	StreamChan <-chan string        `json:"-"`
}

// InterviewEvent represents a real-time event during an interview.
type InterviewEvent struct {
	Type      string      `json:"type"`
	SessionID string      `json:"session_id"`
	Data      interface{} `json:"data"`
	Streaming bool        `json:"streaming"`
}

// InterviewService is the unified interface that all interaction channels call into.
// It is implemented by the orchestration layer.
type InterviewService interface {
	// Session lifecycle
	CreateSession(ctx context.Context, req CreateSessionReq) (*model.Session, error)
	GetSession(ctx context.Context, sessionID string) (*model.Session, error)
	ResumeSession(ctx context.Context, sessionID string) (*model.Session, error)

	// Setup phase
	ParseJD(ctx context.Context, sessionID string, rawJD string) (*model.JDAnalysis, error)
	UploadResume(ctx context.Context, sessionID string, fileData []byte, fileName string) (*model.ResumeMatch, error)
	GetQuestionPlan(ctx context.Context, sessionID string) (*model.QuestionPlan, error)

	// Interview loop
	StartInterview(ctx context.Context, sessionID string) (*InterviewEvent, error)
	SubmitAnswer(ctx context.Context, sessionID string, answer string) (*InterviewEvent, error)
	SkipQuestion(ctx context.Context, sessionID string) (*InterviewEvent, error)

	// Results
	GetReport(ctx context.Context, sessionID string) (*model.Report, error)
	GetReviewPlan(ctx context.Context, sessionID string) (*model.ReviewPlan, error)

	// Multi-purpose message with intent routing
	HandleMessage(ctx context.Context, sessionID string, msg string) (*MessageResponse, error)

	// Subscribe to real-time events for a session
	Subscribe(ctx context.Context, sessionID string) (<-chan *InterviewEvent, error)
}
