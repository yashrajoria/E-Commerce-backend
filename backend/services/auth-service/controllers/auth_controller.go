package controllers

import (
	"context"
	"net/http"
	"os"
	"strings"

	"auth-service/services"

	"github.com/gin-gonic/gin"
)

type IAuthService interface {
	Login(ctx context.Context, email, password string) (*services.TokenPair, error)
	Register(ctx context.Context, name, email, password, role string) error
	VerifyEmail(ctx context.Context, email, code string) error
	RefreshTokens(ctx context.Context, refreshToken string) (*services.TokenPair, error)
}

type AuthController struct {
	service IAuthService
}

func NewAuthController(s IAuthService) *AuthController {
	return &AuthController{service: s}
}

func (ctrl *AuthController) Login(c *gin.Context) {
	var req struct {
		Email    string `json:"email" binding:"required,email"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body", "details": err.Error()})
		return
	}

	tokenPair, err := ctrl.service.Login(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	domain := os.Getenv("COOKIE_DOMAIN")
	isSecure := os.Getenv("ENV") == "production"

	// Set SameSite for CSRF protection
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("token", tokenPair.AccessToken, 900, "/", domain, isSecure, true)

	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("refresh_token", tokenPair.RefreshToken, 604800, "/", domain, isSecure, true)

	c.JSON(http.StatusOK, gin.H{"message": "Logged in successfully"})
}

func (ctrl *AuthController) Register(c *gin.Context) {
	var req struct {
		Name     string `json:"name" binding:"required"`
		Email    string `json:"email" binding:"required,email"`
		Password string `json:"password" binding:"required,min=8"`
		Role     string `json:"role" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body", "details": err.Error()})
		return
	}

	err := ctrl.service.Register(c.Request.Context(), req.Name, req.Email, req.Password, req.Role)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not create account at this time."})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Account created successfully. Please verify your email."})
}

func (ctrl *AuthController) VerifyEmail(c *gin.Context) {
	var req struct {
		Email string `json:"email" binding:"required,email"`
		Code  string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	err := ctrl.service.VerifyEmail(c.Request.Context(), req.Email, req.Code)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		if strings.Contains(err.Error(), "invalid") {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify email"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Email verified successfully"})
}

func (ctrl *AuthController) Logout(c *gin.Context) {
	domain := os.Getenv("COOKIE_DOMAIN")
	isSecure := os.Getenv("ENV") == "production"

	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("token", "", -1, "/", domain, isSecure, true)

	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("refresh_token", "", -1, "/", domain, isSecure, true)

	c.JSON(http.StatusOK, gin.H{"message": "Logged out successfully"})
}

func (ctrl *AuthController) Refresh(c *gin.Context) {
	refreshToken, err := c.Cookie("refresh_token")
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Refresh token not found"})
		return
	}

	newTokenPair, err := ctrl.service.RefreshTokens(c.Request.Context(), refreshToken)
	if err != nil {
		ctrl.Logout(c)
		return
	}

	domain := os.Getenv("COOKIE_DOMAIN")
	isSecure := os.Getenv("ENV") == "production"

	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("token", newTokenPair.AccessToken, 900, "/", domain, isSecure, true)

	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("refresh_token", newTokenPair.RefreshToken, 604800, "/", domain, isSecure, true)

	c.JSON(http.StatusOK, gin.H{"message": "Token refreshed successfully"})
}

func (ctrl *AuthController) GetAuthStatus(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		// If not in context, try headers (forwarded by API gateway)
		userID = c.GetHeader("X-User-ID")
	}

	// userID := c.GetHeader("X-User-ID")
	email := c.GetHeader("X-User-Email")
	role := c.GetHeader("X-User-Role")

	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authenticated"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"authenticated": true,
		"user": gin.H{
			"id":    userID,
			"email": email,
			"role":  role,
		},
	})
}
