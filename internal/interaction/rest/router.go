package rest

import (
	"github.com/KurisuNo1/InterviewAgent/internal/capability/store"
	"github.com/KurisuNo1/InterviewAgent/internal/interaction"
	"github.com/KurisuNo1/InterviewAgent/internal/interaction/rest/auth"
	"github.com/gin-gonic/gin"
)

// NewRouter creates and configures a Gin router with all API routes.
func NewRouter(svc interaction.InterviewService, jwtManager *auth.JWTManager, userStore *store.UserStore, wechatAppID, wechatSecret string) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	// Global middleware
	r.Use(Recovery())
	r.Use(RequestLogger())
	r.Use(CORS())

	h := NewHandler(svc)
	authH := auth.NewAuthHandler(jwtManager, userStore, wechatAppID, wechatSecret)

	// Auth routes (no JWT required)
	authGroup := r.Group("/api/auth")
	{
		authGroup.POST("/login", authH.Login)
		authGroup.POST("/register", authH.Register)
		authGroup.POST("/wechat-login", authH.WeChatLogin)
	}

	// Protected API routes (JWT optional in dev)
	api := r.Group("/api")
	api.Use(auth.OptionalAuth(jwtManager))
	{
		api.GET("/auth/me", authH.Me)

		api.POST("/sessions", h.CreateSession)
		api.GET("/sessions/:id", h.GetSession)
		api.POST("/sessions/:id/jd", h.ParseJD)
		api.POST("/sessions/:id/resume", h.UploadResume)
		api.GET("/sessions/:id/plan", h.GetQuestionPlan)
		api.POST("/sessions/:id/start", h.StartInterview)
		api.POST("/sessions/:id/answer", h.SubmitAnswer)
		api.POST("/sessions/:id/answer/stream", h.StreamAnswer)
		api.POST("/sessions/:id/skip", h.SkipQuestion)
		api.GET("/sessions/:id/report", h.GetReport)
		api.GET("/sessions/:id/review-plan", h.GetReviewPlan)
		api.POST("/sessions/:id/complete", h.CompleteInterview)
		api.POST("/sessions/:id/restore", h.ResumeSession)
		api.POST("/sessions/:id/message", h.HandleMessage)
		api.POST("/sessions/:id/stream", h.StreamMessage)
		api.GET("/sessions/:id/messages", h.GetMessages)
		api.GET("/sessions/:id/context/stats", h.GetSessionContextStats)
	}

	api.GET("/sessions", h.ListSessions)
	api.GET("/skills", h.ListSkills)
	api.GET("/tools", h.ListTools)

	api.POST("/documents/upload", h.UploadDocuments)
	api.GET("/documents", h.ListDocuments)
	api.DELETE("/documents/:id", h.DeleteDocument)

	api.GET("/context/stats", h.GetContextStats)

	return r
}
