package controllers

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"auth-service/models"
	"auth-service/services"

	"github.com/gin-gonic/gin"
	"github.com/yashrajoria/common/password"
	"go.uber.org/zap"
)

type IAuthService interface {
	Login(ctx context.Context, email, password string) (*services.TokenPair, error)
	Register(ctx context.Context, name, email, password, role string) error
	ProvisionUser(ctx context.Context, in services.ProvisionUserInput) error
	VerifyEmail(ctx context.Context, email, code string) error
	RefreshTokens(ctx context.Context, refreshToken string) (*services.TokenPair, error)
	Logout(ctx context.Context, refreshToken string) error
	ResendVerificationEmail(ctx context.Context, email string) error
	GetUserByID(ctx context.Context, userID string) (*models.User, error)
	RevokeUserTokens(ctx context.Context, userID string) error
}

type AuthController struct {
	service IAuthService
}

func NewAuthController(s IAuthService) *AuthController {
	return &AuthController{service: s}
}

func cookieSettings() (http.SameSite, bool) {
	crossOrigin := os.Getenv("COOKIE_CROSS_ORIGIN") == "true"
	prod := os.Getenv("ENV") == "production"
	if crossOrigin || prod {
		return http.SameSiteNoneMode, true
	}
	return http.SameSiteLaxMode, false
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

	// Normalize email
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))

	tokenPair, err := ctrl.service.Login(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	// Debug logging: tokens and identifiers (remove in production)
	// log.Printf("[AUTH][LOGIN] email=%s access_token=%s", req.Email, tokenPair.AccessToken)
	// log.Printf("[AUTH][LOGIN] email=%s refresh_token=%s", req.Email, tokenPair.RefreshToken)

	domain := os.Getenv("COOKIE_DOMAIN")
	sameSite, secure := cookieSettings()

	c.SetSameSite(sameSite)
	c.SetCookie("__session", tokenPair.AccessToken, 900, "/", domain, secure, true)
	c.SetCookie("refresh_token", tokenPair.RefreshToken, 604800, "/", domain, secure, true)
	// Identity cookies are HttpOnly — authorization must come from JWT via the gateway.
	c.SetCookie("user_id", tokenPair.UserID, 604800, "/", domain, secure, true)
	c.SetCookie("user_role", tokenPair.Role, 604800, "/", domain, secure, true)

	c.JSON(http.StatusOK, gin.H{
		"message": "Logged in successfully",
		"user": gin.H{
			"id":    tokenPair.UserID,
			"email": req.Email,
			"role":  tokenPair.Role,
		},
	})
}

func (ctrl *AuthController) Register(c *gin.Context) {
	var req struct {
		Name     string `json:"name" binding:"required"`
		Email    string `json:"email" binding:"required,email"`
		Password string `json:"password" binding:"required,min=8"`
		// Role is always set to 'user' server-side — client value ignored
		Role string `json:"role"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body", "details": err.Error()})
		return
	}

	req.Role = "user"

	// Normalize email
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))

	// Validate password strength before proceeding
	pwValidator := password.NewValidator()
	if err := pwValidator.ValidatePassword(req.Password); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err := ctrl.service.Register(c.Request.Context(), req.Name, req.Email, req.Password, req.Role)
	if err != nil {
		if errors.Is(err, services.ErrEmailAlreadyExists) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not create account at this time."})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Account created successfully. Please verify your email.", "email": req.Email})
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

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	err := ctrl.service.VerifyEmail(c.Request.Context(), req.Email, req.Code)
	if err != nil {
		if errors.Is(err, services.ErrUserNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, services.ErrInvalidVerificationCode) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify email"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Email verified successfully"})
}

func (ctrl *AuthController) Logout(c *gin.Context) {
	// Revoke server-side refresh token if present; best-effort — cookies are
	// cleared regardless so the client is logged out even if this fails
	// (e.g. token already revoked/expired, which is not an error worth surfacing).
	refreshToken, _ := c.Cookie("refresh_token")
	if refreshToken != "" {
		if err := ctrl.service.Logout(c.Request.Context(), refreshToken); err != nil {
			zap.L().Debug("logout: refresh token revoke failed", zap.Error(err))
		}
	}

	domain := os.Getenv("COOKIE_DOMAIN")
	sameSite, secure := cookieSettings()

	c.SetSameSite(sameSite)
	c.SetCookie("__session", "", -1, "/", domain, secure, true)
	c.SetCookie("refresh_token", "", -1, "/", domain, secure, true)
	c.SetCookie("user_id", "", -1, "/", domain, secure, false)
	c.SetCookie("user_role", "", -1, "/", domain, secure, false)

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
		zap.L().Warn("refresh token rejected", zap.Error(err))
		ctrl.Logout(c)
		return
	}

	domain := os.Getenv("COOKIE_DOMAIN")
	sameSite, secure := cookieSettings()

	c.SetSameSite(sameSite)
	c.SetCookie("__session", newTokenPair.AccessToken, 900, "/", domain, secure, true)
	c.SetCookie("refresh_token", newTokenPair.RefreshToken, 604800, "/", domain, secure, true)
	c.SetCookie("user_id", newTokenPair.UserID, 604800, "/", domain, secure, true)
	c.SetCookie("user_role", newTokenPair.Role, 604800, "/", domain, secure, true)

	c.JSON(http.StatusOK, gin.H{
		"message": "Token refreshed successfully",
		"user": gin.H{
			"id":   newTokenPair.UserID,
			"role": newTokenPair.Role,
		},
	})
}

// GetAuthStatus trusts only X-User-* headers set by api-gateway's JWTMiddleware
// (see forwarder.go) — never client cookies, which anyone reaching this service
// directly could forge. Route is also gated by internalauth.Require() in main.go.
func (ctrl *AuthController) GetAuthStatus(c *gin.Context) {
	userID := c.GetHeader("X-User-ID")
	email := c.GetHeader("X-User-Email")
	role := c.GetHeader("X-User-Role")

	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authenticated"})
		return
	}

	if u, err := ctrl.service.GetUserByID(c.Request.Context(), userID); err == nil && u != nil {
		role = u.Role
		if email == "" {
			email = u.Email
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"authenticated": true,
		"user": gin.H{
			"id":    userID,
			"email": email,
			"role":  role,
		},
		"timestamp": time.Now().UTC(),
	})
}

func (ctrl *AuthController) ResendVerificationEmail(c *gin.Context) {
	var req struct {
		Email string `json:"email" binding:"required,email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body", "details": err.Error()})
		return
	}

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	err := ctrl.service.ResendVerificationEmail(c.Request.Context(), req.Email)
	if err != nil {
		if errors.Is(err, services.ErrUserNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
			return
		}
		if errors.Is(err, services.ErrEmailAlreadyVerified) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Email already verified"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not resend verification email"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Verification email sent successfully"})
}

func (ctrl *AuthController) AdminCreateUser(c *gin.Context) {
	var req struct {
		Name     string `json:"name" binding:"required"`
		Email    string `json:"email" binding:"required,email"`
		Password string `json:"password" binding:"required,min=8"`
		Role     string `json:"role" binding:"required,oneof=admin user"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body", "details": err.Error()})
		return
	}

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))

	pwValidator := password.NewValidator()
	if err := pwValidator.ValidatePassword(req.Password); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Admin-provisioned accounts are verified so invitees can sign in immediately.
	// Public self-signup still requires email verification via Register.
	err := ctrl.service.ProvisionUser(c.Request.Context(), services.ProvisionUserInput{
		Name:          req.Name,
		Email:         req.Email,
		Password:      req.Password,
		Role:          req.Role,
		EmailVerified: true,
		Notify:        false,
	})
	if err != nil {
		if errors.Is(err, services.ErrEmailAlreadyExists) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "User created successfully", "email": req.Email, "role": req.Role})
}

// RevokeUserTokens revokes every outstanding refresh token for a user.
// Internal-only (gated by internalauth.Require() at the route). Called by
// user-service right after a password change so a stolen session can't
// outlive the password that issued it.
func (ctrl *AuthController) RevokeUserTokens(c *gin.Context) {
	var req struct {
		UserID string `json:"user_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	if err := ctrl.service.RevokeUserTokens(c.Request.Context(), req.UserID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to revoke tokens"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Tokens revoked"})
}
