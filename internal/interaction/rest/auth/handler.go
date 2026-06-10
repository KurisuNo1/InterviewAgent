package auth

import (
	"net/http"

	"github.com/KurisuNo1/InterviewAgent/internal/capability/store"
	"github.com/gin-gonic/gin"
)

// AuthHandler handles authentication endpoints with MySQL-backed user storage.
type AuthHandler struct {
	manager      *JWTManager
	userStore    *store.UserStore
	wechatAppID  string
	wechatSecret string
}

// NewAuthHandler creates a new auth handler.
func NewAuthHandler(manager *JWTManager, userStore *store.UserStore, wechatAppID, wechatSecret string) *AuthHandler {
	return &AuthHandler{
		manager:      manager,
		userStore:    userStore,
		wechatAppID:  wechatAppID,
		wechatSecret: wechatSecret,
	}
}

// LoginReq is the login request body.
type LoginReq struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// LoginResp is the login response body.
type LoginResp struct {
	Token    string `json:"token"`
	UserID   string `json:"user_id"`
	Username string `json:"username"`
}

// Login handles POST /api/auth/login
func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	valid, err := h.userStore.ValidateCredentials(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "internal error"})
		return
	}
	if !valid {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "invalid credentials"})
		return
	}

	// Use username as the stable user_id so sessions survive server restarts
	token, err := h.manager.GenerateToken(req.Username, req.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "failed to generate token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data": LoginResp{
			Token:    token,
			UserID:   req.Username,
			Username: req.Username,
		},
	})
}

// RegisterReq is the register request body.
type RegisterReq struct {
	Username string `json:"username" binding:"required,min=3"`
	Password string `json:"password" binding:"required,min=6"`
}

// Register handles POST /api/auth/register
func (h *AuthHandler) Register(c *gin.Context) {
	var req RegisterReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	if err := h.userStore.CreateUser(c.Request.Context(), req.Username, req.Password); err != nil {
		c.JSON(http.StatusConflict, gin.H{"code": 409, "message": err.Error()})
		return
	}

	token, err := h.manager.GenerateToken(req.Username, req.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "failed to generate token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data": LoginResp{
			Token:    token,
			UserID:   req.Username,
			Username: req.Username,
		},
	})
}

// Me handles GET /api/auth/me
func (h *AuthHandler) Me(c *gin.Context) {
	userID := GetUserID(c)
	username, _ := c.Get(ContextKeyUsername)
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data": gin.H{
			"user_id":  userID,
			"username": username,
		},
	})
}
