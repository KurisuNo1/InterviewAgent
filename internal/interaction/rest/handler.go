package rest

import (
	"net/http"

	"github.com/KurisuNo1/InterviewAgent/internal/interaction"
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

	session, err := h.svc.CreateSession(c.Request.Context(), interaction.CreateSessionReq{
		UserID: req.UserID,
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
