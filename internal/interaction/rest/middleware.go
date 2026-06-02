package rest

import (
	"log"
	"time"

	"github.com/gin-gonic/gin"
)

// CORS returns a CORS middleware.
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}

// RequestLogger returns a request logging middleware.
func RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()
		log.Printf("[%d] %s %s | %v", status, c.Request.Method, path, latency)
	}
}

// Recovery returns a panic recovery middleware.
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("panic recovered: %v", err)
				c.JSON(500, APIResponse{Code: 500, Message: "internal server error"})
				c.Abort()
			}
		}()
		c.Next()
	}
}
