package rest

import (
	"encoding/json"
	"net/http"

	"github.com/KurisuNo1/InterviewAgent/internal/interaction"
	"github.com/KurisuNo1/InterviewAgent/internal/interaction/rest/auth"
	"github.com/KurisuNo1/InterviewAgent/internal/model"
	"github.com/gin-gonic/gin"
)

// Handler implements REST API handlers backed by an InterviewService.
type Handler struct {
	svc interaction.InterviewService
}

// NewHandler creates a new REST handler.
func NewHandler(svc interaction.InterviewService) *Handler {
	return &Handler{svc: svc}
}

// CreateSession handles POST /api/sessions
func (h *Handler) CreateSession(c *gin.Context) {
	var req CreateSessionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse{Code: 400, Message: err.Error()})
		return
	}

	// Use authenticated user ID from JWT if available, otherwise fall back to request body
	userID := req.UserID
	if authUser := auth.GetUserID(c); authUser != "" {
		userID = authUser
	}
	if userID == "" {
		userID = "anonymous"
	}

	session, err := h.svc.CreateSession(c.Request.Context(), interaction.CreateSessionReq{
		UserID: userID,
		JDText: req.JDText,
		JDURL:  req.JDURL,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{Code: 500, Message: err.Error()})
		return
	}

	c.JSON(http.StatusCreated, APIResponse{Code: 200, Message: "ok", Data: session})
}

// GetSession handles GET /api/sessions/:id
func (h *Handler) GetSession(c *gin.Context) {
	sessionID := c.Param("id")
	session, err := h.svc.GetSession(c.Request.Context(), sessionID)
	if err != nil {
		c.JSON(http.StatusNotFound, APIResponse{Code: 404, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, APIResponse{Code: 200, Message: "ok", Data: session})
}

// ParseJD handles POST /api/sessions/:id/jd
func (h *Handler) ParseJD(c *gin.Context) {
	sessionID := c.Param("id")
	var req struct {
		JDText string `json:"jd_text" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse{Code: 400, Message: err.Error()})
		return
	}

	result, err := h.svc.ParseJD(c.Request.Context(), sessionID, req.JDText)
	if err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, APIResponse{Code: 200, Message: "ok", Data: result})
}

// UploadResume handles POST /api/sessions/:id/resume
func (h *Handler) UploadResume(c *gin.Context) {
	sessionID := c.Param("id")
	var req UploadResumeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse{Code: 400, Message: err.Error()})
		return
	}

	result, err := h.svc.UploadResume(c.Request.Context(), sessionID, req.Content, req.FileName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, APIResponse{Code: 200, Message: "ok", Data: result})
}

// GetQuestionPlan handles GET /api/sessions/:id/plan
func (h *Handler) GetQuestionPlan(c *gin.Context) {
	sessionID := c.Param("id")
	plan, err := h.svc.GetQuestionPlan(c.Request.Context(), sessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, APIResponse{Code: 200, Message: "ok", Data: plan})
}

// StartInterview handles POST /api/sessions/:id/start
func (h *Handler) StartInterview(c *gin.Context) {
	sessionID := c.Param("id")
	event, err := h.svc.StartInterview(c.Request.Context(), sessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, APIResponse{Code: 200, Message: "ok", Data: event})
}

// SubmitAnswer handles POST /api/sessions/:id/answer
func (h *Handler) SubmitAnswer(c *gin.Context) {
	sessionID := c.Param("id")
	var req AnswerReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse{Code: 400, Message: err.Error()})
		return
	}

	event, err := h.svc.SubmitAnswer(c.Request.Context(), sessionID, req.Answer)
	if err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, APIResponse{Code: 200, Message: "ok", Data: event})
}

// StreamAnswer handles POST /api/sessions/:id/answer/stream (SSE streaming).
func (h *Handler) StreamAnswer(c *gin.Context) {
	sessionID := c.Param("id")
	var req AnswerReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse{Code: 400, Message: err.Error()})
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	stream, err := h.svc.StreamSubmitAnswer(c.Request.Context(), sessionID, req.Answer)
	if err != nil {
		c.SSEvent("error", err.Error())
		return
	}
	if stream == nil {
		c.SSEvent("error", "stream is nil")
		return
	}
	defer stream.Close()

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.SSEvent("error", "streaming not supported")
		return
	}

	for {
		msg, err := stream.Recv()
		if err != nil {
			break
		}
		if msg != nil && msg.Content != "" {
			c.SSEvent("chunk", msg.Content)
			flusher.Flush()
		}
	}
	c.SSEvent("done", "[DONE]")
	flusher.Flush()
}

// SkipQuestion handles POST /api/sessions/:id/skip
func (h *Handler) SkipQuestion(c *gin.Context) {
	sessionID := c.Param("id")
	event, err := h.svc.SkipQuestion(c.Request.Context(), sessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, APIResponse{Code: 200, Message: "ok", Data: event})
}

// GetReport handles GET /api/sessions/:id/report
func (h *Handler) GetReport(c *gin.Context) {
	sessionID := c.Param("id")
	report, err := h.svc.GetReport(c.Request.Context(), sessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, APIResponse{Code: 200, Message: "ok", Data: report})
}

// CompleteInterview handles POST /api/sessions/:id/complete — ends the interview early.
func (h *Handler) CompleteInterview(c *gin.Context) {
	sessionID := c.Param("id")
	event, err := h.svc.CompleteInterview(c.Request.Context(), sessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, APIResponse{Code: 200, Message: "ok", Data: event})
}

// GetReviewPlan handles GET /api/sessions/:id/review-plan
func (h *Handler) GetReviewPlan(c *gin.Context) {
	sessionID := c.Param("id")
	plan, err := h.svc.GetReviewPlan(c.Request.Context(), sessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, APIResponse{Code: 200, Message: "ok", Data: plan})
}

// ResumeSession handles POST /api/sessions/:id/resume
func (h *Handler) ResumeSession(c *gin.Context) {
	sessionID := c.Param("id")
	session, err := h.svc.ResumeSession(c.Request.Context(), sessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, APIResponse{Code: 200, Message: "ok", Data: session})
}

// HandleMessage handles POST /api/sessions/:id/message
func (h *Handler) HandleMessage(c *gin.Context) {
	sessionID := c.Param("id")
	var req MessageReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse{Code: 400, Message: err.Error()})
		return
	}

	resp, err := h.svc.HandleMessage(c.Request.Context(), sessionID, req.Message)
	if err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, APIResponse{Code: 200, Message: "ok", Data: resp})
}

// ListSkills handles GET /api/skills
func (h *Handler) ListSkills(c *gin.Context) {
	skills, err := h.svc.ListSkillInfos(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, APIResponse{Code: 200, Message: "ok", Data: skills})
}

// ListTools handles GET /api/tools
func (h *Handler) ListTools(c *gin.Context) {
	tools, err := h.svc.ListAvailableTools(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{Code: 500, Message: err.Error()})
		return
	}
	if tools == nil {
		tools = []interaction.ToolInfo{}
	}
	c.JSON(http.StatusOK, APIResponse{Code: 200, Message: "ok", Data: tools})
}

// UploadDocuments handles POST /api/documents/upload
func (h *Handler) UploadDocuments(c *gin.Context) {
	var req UploadDocumentsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse{Code: 400, Message: err.Error()})
		return
	}
	if len(req.Files) == 0 {
		c.JSON(http.StatusBadRequest, APIResponse{Code: 400, Message: "no files provided"})
		return
	}

	files := make([]interaction.UploadFile, len(req.Files))
	for i, f := range req.Files {
		files[i] = interaction.UploadFile{FileName: f.FileName, Content: f.Content}
	}

	result, err := h.svc.UploadDocuments(c.Request.Context(), files)
	if err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, APIResponse{Code: 200, Message: "ok", Data: result})
}

// ListDocuments handles GET /api/documents
func (h *Handler) ListDocuments(c *gin.Context) {
	docs, err := h.svc.ListDocuments(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, APIResponse{Code: 200, Message: "ok", Data: docs})
}

// StreamMessage handles POST /api/sessions/:id/stream (SSE).
func (h *Handler) StreamMessage(c *gin.Context) {
	sessionID := c.Param("id")
	var req MessageReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse{Code: 400, Message: err.Error()})
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	stream, err := h.svc.StreamMessage(c.Request.Context(), sessionID, req.Message)
	if err != nil {
		c.SSEvent("error", err.Error())
		return
	}
	if stream == nil {
		c.SSEvent("error", "stream is nil")
		return
	}
	defer stream.Close()

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.SSEvent("error", "streaming not supported")
		return
	}

	for {
		msg, err := stream.Recv()
		if err != nil {
			break
		}
		if msg != nil {
			jsonBytes, _ := json.Marshal(map[string]string{"content": msg.Content})
			c.SSEvent("chunk", string(jsonBytes))
			flusher.Flush()
		}
	}
	c.SSEvent("done", "[DONE]")
	flusher.Flush()
}

// GetMessages handles GET /api/sessions/:id/messages
func (h *Handler) GetMessages(c *gin.Context) {
	sessionID := c.Param("id")
	msgs, err := h.svc.GetConversationHistory(c.Request.Context(), sessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{Code: 500, Message: err.Error()})
		return
	}
	if msgs == nil {
		msgs = []model.Message{}
	}
	c.JSON(http.StatusOK, APIResponse{Code: 200, Message: "ok", Data: msgs})
}

// ListSessions handles GET /api/sessions
func (h *Handler) ListSessions(c *gin.Context) {
	userID := c.Query("user_id")
	sessions, err := h.svc.ListSessions(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, APIResponse{Code: 200, Message: "ok", Data: sessions})
}

// DeleteDocument handles DELETE /api/documents/:id
func (h *Handler) DeleteDocument(c *gin.Context) {
	docID := c.Param("id")
	if err := h.svc.DeleteDocument(c.Request.Context(), docID); err != nil {
		c.JSON(http.StatusInternalServerError, APIResponse{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, APIResponse{Code: 200, Message: "deleted"})
}
