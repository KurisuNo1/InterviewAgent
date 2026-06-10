package interaction

import (
	"context"

	"github.com/KurisuNo1/InterviewAgent/internal/model"
	"github.com/cloudwego/eino/schema"
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

// SkillInfo describes an available skill practice module.
type SkillInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
}

// ToolInfo describes an available MCP tool for the frontend.
type ToolInfo struct {
	Name        string `json:"name"`
	Server      string `json:"server"`
	Description string `json:"description"`
}

// UploadFile represents a single file to be ingested.
type UploadFile struct {
	FileName string `json:"file_name"`
	Content  []byte `json:"content"`
}

// UploadResult summarizes a batch document upload.
type UploadResult struct {
	TotalFiles  int      `json:"total_files"`
	TotalChunks int      `json:"total_chunks"`
	Files       []string `json:"files"`
	Errors      []string `json:"errors,omitempty"`
}

// DocInfo is minimal metadata for a stored document.
type DocInfo struct {
	ID         string `json:"id"`
	SourceFile string `json:"source_file"`
}

// SessionSummary is a lightweight view of a past interview session.
type SessionSummary struct {
	ID           string  `json:"id"`
	Status       string  `json:"status"`
	OverallScore float64 `json:"overall_score"`
	CreatedAt    string  `json:"created_at"`
	LastMessage  string  `json:"last_message,omitempty"`
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
	StreamSubmitAnswer(ctx context.Context, sessionID string, answer string) (*schema.StreamReader[*schema.Message], error)
	SkipQuestion(ctx context.Context, sessionID string) (*InterviewEvent, error)
	CompleteInterview(ctx context.Context, sessionID string) (*InterviewEvent, error)

	// Results
	GetReport(ctx context.Context, sessionID string) (*model.Report, error)
	GetReviewPlan(ctx context.Context, sessionID string) (*model.ReviewPlan, error)

	// Multi-purpose message with intent routing
	HandleMessage(ctx context.Context, sessionID string, msg string) (*MessageResponse, error)

	// List available skill practice modules
	ListSkillInfos(ctx context.Context) ([]SkillInfo, error)

	// List available MCP tools
	ListAvailableTools(ctx context.Context) ([]ToolInfo, error)

	// Upload & knowledge base
	UploadDocuments(ctx context.Context, files []UploadFile) (*UploadResult, error)
	ListDocuments(ctx context.Context) ([]DocInfo, error)
	DeleteDocument(ctx context.Context, docID string) error

	// Session history
	ListSessions(ctx context.Context, userID string) ([]SessionSummary, error)
	GetConversationHistory(ctx context.Context, sessionID string) ([]model.Message, error)

	// Streaming message for real-time LLM output
	StreamMessage(ctx context.Context, sessionID string, msg string) (*schema.StreamReader[*schema.Message], error)

	// Subscribe to real-time events for a session
	Subscribe(ctx context.Context, sessionID string) (<-chan *InterviewEvent, error)
}
