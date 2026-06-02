package rest

import (
	"github.com/KurisuNo1/InterviewAgent/internal/interaction"
	"github.com/gin-gonic/gin"
)

// NewRouter creates and configures a Gin router with all API routes.
func NewRouter(svc interaction.InterviewService) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	// Global middleware
	r.Use(Recovery())
	r.Use(RequestLogger())
	r.Use(CORS())

	h := NewHandler(svc)

	api := r.Group("/api")
	{
		api.POST("/sessions", h.CreateSession)
		api.GET("/sessions/:id", h.GetSession)
		api.POST("/sessions/:id/jd", h.ParseJD)
		api.POST("/sessions/:id/resume", h.UploadResume)
		api.GET("/sessions/:id/plan", h.GetQuestionPlan)
		api.POST("/sessions/:id/start", h.StartInterview)
		api.POST("/sessions/:id/answer", h.SubmitAnswer)
		api.POST("/sessions/:id/skip", h.SkipQuestion)
		api.GET("/sessions/:id/report", h.GetReport)
		api.GET("/sessions/:id/review-plan", h.GetReviewPlan)
		api.POST("/sessions/:id/restore", h.ResumeSession)
	}

	return r
}
