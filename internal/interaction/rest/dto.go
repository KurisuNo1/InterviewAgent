package rest

import "github.com/KurisuNo1/InterviewAgent/internal/model"

// CreateSessionReq is the REST request to create a session.
type CreateSessionReq struct {
	UserID string `json:"user_id"`
	JDText string `json:"jd_text"`
	JDURL  string `json:"jd_url"`
}

// UploadResumeReq is the REST request to upload a resume.
type UploadResumeReq struct {
	FileName string `json:"file_name"`
	Content  []byte `json:"content"`
}

// AnswerReq is the REST request to submit an answer.
type AnswerReq struct {
	Answer string `json:"answer" binding:"required"`
}

// APIResponse is the standard API response envelope.
type APIResponse struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// SessionResponse wraps a session.
type SessionResponse struct {
	ID        string               `json:"id"`
	Status    model.InterviewPhase `json:"status"`
	CreatedAt string               `json:"created_at"`
}
