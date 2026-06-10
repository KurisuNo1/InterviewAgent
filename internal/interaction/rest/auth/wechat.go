package auth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// WeChatLoginReq is the WeChat code login request body.
type WeChatLoginReq struct {
	Code string `json:"code" binding:"required"`
}

// WeChatLoginResp is the WeChat login response body.
type WeChatLoginResp struct {
	Token    string `json:"token"`
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	IsNew    bool   `json:"is_new"`
}

// weChatSession is the response from WeChat code2Session API.
type weChatSession struct {
	OpenID     string `json:"openid"`
	SessionKey string `json:"session_key"`
	UnionID    string `json:"unionid"`
	ErrCode    int    `json:"errcode"`
	ErrMsg     string `json:"errmsg"`
}

// WeChatLogin handles POST /api/auth/wechat-login.
func (h *AuthHandler) WeChatLogin(c *gin.Context) {
	var req WeChatLoginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	// Exchange code for openid/session_key via WeChat API
	session, err := code2Session(h.wechatAppID, h.wechatSecret, req.Code)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"code": 502, "message": "微信服务暂不可用，请稍后重试"})
		return
	}
	if session.ErrCode != 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   wechatErrMsg(session.ErrCode),
			"errcode":  session.ErrCode,
		})
		return
	}

	// Find or create user by openid
	userID, isNew, err := h.userStore.FindOrCreateByWeChat(c.Request.Context(), session.OpenID, session.UnionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "登录失败，请重试"})
		return
	}

	// Generate JWT token
	token, err := h.manager.GenerateToken(userID, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "failed to generate token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data": WeChatLoginResp{
			Token:    token,
			UserID:   userID,
			Username: userID,
			IsNew:    isNew,
		},
	})
}

// code2Session calls the WeChat jscode2session API.
func code2Session(appID, appSecret, code string) (*weChatSession, error) {
	url := fmt.Sprintf(
		"https://api.weixin.qq.com/sns/jscode2session?appid=%s&secret=%s&js_code=%s&grant_type=authorization_code",
		appID, appSecret, code,
	)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("wechat api request failed: %w", err)
	}
	defer resp.Body.Close()

	var session weChatSession
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return nil, fmt.Errorf("failed to decode wechat response: %w", err)
	}

	return &session, nil
}

// wechatErrMsg maps WeChat error codes to Chinese messages.
func wechatErrMsg(errcode int) string {
	switch errcode {
	case -1:
		return "微信系统繁忙，请稍后重试"
	case 40029:
		return "登录凭证已过期，请重新授权"
	case 40163:
		return "登录凭证已被使用，请重新授权"
	case 40226:
		return "高风险用户，登录已被拦截"
	case 45011:
		return "操作过于频繁，请稍后重试"
	default:
		return fmt.Sprintf("微信登录失败 (err=%d)", errcode)
	}
}
