package rest

import "github.com/KurisuNo1/InterviewAgent/internal/model"

// CreateSessionReq is the REST request to create a session.
type CreateSessionReq struct {
	UserID string `json:"user_id"`
	JDText string `json:"jd_text"`
	JDURL  string `json:"jd_url"`
}

// UploadResumeReq is the REST request to upload a resume.
// Content is base64-encoded file data (as sent by frontend FileReader + btoa).
type UploadResumeReq struct {
	FileName string `json:"file_name"`
	Content  []byte `json:"file_data"`
}

// AnswerReq is the REST request to submit an answer.
type AnswerReq struct {
	Answer string `json:"answer" binding:"required"`
}

// MessageReq is the REST request to send a chat/skill message.
type MessageReq struct {
	Message string `json:"message" binding:"required"`
}

// APIResponse is the standard API response envelope.
type APIResponse struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// UploadDocumentsReq is the REST request to upload files for ingestion.
type UploadDocumentsReq struct {
	Files []UploadFileItem `json:"files"`
}

// UploadFileItem represents a single file in the upload request.
type UploadFileItem struct {
	FileName string `json:"file_name"`
	Content  []byte `json:"content"`
}

// SessionResponse wraps a session.
type SessionResponse struct {
	ID        string               `json:"id"`
	Status    model.InterviewPhase `json:"status"`
	CreatedAt string               `json:"created_at"`
}
