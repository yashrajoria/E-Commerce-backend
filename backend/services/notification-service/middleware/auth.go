package middleware

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

const (
	UserContextKey  = "userID"
	RoleContextKey  = "role"
	EmailContextKey = "email"
	AdminRole       = "admin"
)

func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetHeader("X-User-ID")
		role := c.GetHeader("X-User-Role")
		email := c.GetHeader("X-User-Email")

		// Cookie fallback (only if behind api-gateway, never publicly exposed)
		if userID == "" {
			if v, err := c.Cookie("user_id"); err == nil {
				userID = v
			}
		}
		if role == "" {
			if v, err := c.Cookie("user_role"); err == nil {
				role = v
			}
		}
		if email == "" {
			if v, err := c.Cookie("user_email"); err == nil {
				email = v
			}
		}

		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			c.Abort()
			return
		}

		// Parse userID to int64 once here — avoids repeated parsing in controllers
		userIDInt, err := strconv.ParseInt(userID, 10, 64)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid user ID format"})
			c.Abort()
			return
		}

		c.Set(UserContextKey, userIDInt)
		c.Set(RoleContextKey, role)
		c.Set(EmailContextKey, email)
		c.Next()
	}
}

func AdminOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get(RoleContextKey)
		if !exists || role != AdminRole {
			c.JSON(http.StatusForbidden, gin.H{"error": "admin role required"})
			c.Abort()
			return
		}
		c.Next()
	}
}

// Helper functions for controllers

func GetUserIDInt(c *gin.Context) int64 {
	if val, ok := c.Get(UserContextKey); ok {
		if id, ok := val.(int64); ok {
			return id
		}
	}
	return 0
}

func GetUserID(c *gin.Context) (string, error) {
	if val, ok := c.Get(UserContextKey); ok {
		if id, ok := val.(int64); ok {
			return strconv.FormatInt(id, 10), nil
		}
	}
	return "", errors.New("user ID not found in context")
}

func GetRole(c *gin.Context) (string, error) {
	if val, ok := c.Get(RoleContextKey); ok {
		if role, ok := val.(string); ok {
			return role, nil
		}
	}
	return "", errors.New("role not found in context")
}

func GetEmail(c *gin.Context) (string, error) {
	if val, ok := c.Get(EmailContextKey); ok {
		if email, ok := val.(string); ok {
			return email, nil
		}
	}
	return "", errors.New("email not found in context")
}
