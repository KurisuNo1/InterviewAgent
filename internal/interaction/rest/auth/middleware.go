package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	// ContextKeyUserID stores the authenticated user ID in Gin context.
	ContextKeyUserID = "auth_user_id"
	// ContextKeyUsername stores the authenticated username.
	ContextKeyUsername = "auth_username"
)

// JWTAuth returns a Gin middleware that validates JWT tokens.
func JWTAuth(manager *JWTManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    401,
				"message": "missing authorization header",
			})
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    401,
				"message": "invalid authorization format, expected: Bearer <token>",
			})
			return
		}

		claims, err := manager.ValidateToken(parts[1])
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    401,
				"message": "invalid or expired token",
			})
			return
		}

		c.Set(ContextKeyUserID, claims.UserID)
		c.Set(ContextKeyUsername, claims.Username)
		c.Next()
	}
}

// OptionalAuth returns a middleware that extracts user info if token is present,
// but does not reject unauthenticated requests.
func OptionalAuth(manager *JWTManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.Next()
			return
		}
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			c.Next()
			return
		}
		claims, err := manager.ValidateToken(parts[1])
		if err != nil {
			c.Next()
			return
		}
		c.Set(ContextKeyUserID, claims.UserID)
		c.Set(ContextKeyUsername, claims.Username)
		c.Next()
	}
}

// GetUserID extracts the authenticated user ID from context.
func GetUserID(c *gin.Context) string {
	if v, ok := c.Get(ContextKeyUserID); ok {
		return v.(string)
	}
	return ""
}
