package controllers

import (
	"auth-service/database"
	"auth-service/models"
	"auth-service/services"
	"auth-service/types"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// --- LOGIN HANDLER --- //
func Login(c *gin.Context) {
	var loginReq types.LoginRequest

	// 1. Validate JSON input
	if err := c.ShouldBindJSON(&loginReq); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	// 2. Find user by email
	var user models.User
	err := database.DB.Where("email = ?", loginReq.Email).First(&user).Error
	if err != nil {
		// Always give generic error (avoid leaking which field failed)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid email or password"})
		return
	}

	// 3. Check email verification
	if !user.EmailVerified {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Email not verified"})
		return
	}

	// 4. Validate password
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(loginReq.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid email or password"})
		return
	}

	// 5. Generate access and refresh JWT tokens
	accessToken, err := services.GenerateTokenPair(user.ID.String(), user.Email, user.Role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}
	refreshToken, err := services.RefreshTokens(accessToken.RefreshToken)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate refresh token"})
		return
	}

	// 6. Set cookies (domain and secure flag controlled by environment)
	domain := os.Getenv("COOKIE_DOMAIN")
	isSecure := os.Getenv("ENV") == "production"
	c.SetCookie("token", accessToken.AccessToken, 900, "/", domain, isSecure, true)              // 15min access token
	c.SetCookie("refresh_token", refreshToken.RefreshToken, 604800, "/", domain, isSecure, true) // 7d refresh token

	// 7. Success response (do not leak tokens or sensitive info)
	c.JSON(http.StatusOK, gin.H{"message": "Logged in successfully"})
}

// --- REGISTER HANDLER --- //
func Register(c *gin.Context) {
	var registerReq types.RegisterRequest

	// 1. Validate JSON input
	if err := c.ShouldBindJSON(&registerReq); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON body"})
		return
	}

	// 2. Validate password strength
	validator := services.NewPasswordValidator()
	if err := validator.ValidatePassword(registerReq.Password); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 3. Check if email already exists
	var existingUser models.User
	if err := database.DB.Where("email = ?", registerReq.Email).First(&existingUser).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Email already exists"})
		return
	}

	// 4. Hash password securely
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(registerReq.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
		return
	}

	// 5. Create new user (default: email not verified until code entered)
	newUser := models.User{
		ID:               uuid.New(),
		Email:            registerReq.Email,
		Name:             registerReq.Name,
		Password:         string(hashedPassword),
		Role:             registerReq.Role, // Or default, if you don't let client choose
		EmailVerified:    false,
		VerificationCode: services.GenerateRandomCode(6),
	}

	// 6. Send verification email (do not halt registration on email failure in some business cases)
	if err := services.SendVerificationEmail(newUser.Email, newUser.VerificationCode); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send verification email"})
		return
	}

	// 7. Persist user to DB
	if err := database.DB.Create(&newUser).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create account"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Account created successfully"})
}

// --- EMAIL VERIFICATION HANDLER --- //
func VerifyEmail(c *gin.Context) {
	type VerifyRequest struct {
		Email string `json:"email"`
		Code  string `json:"code"`
	}

	var req VerifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid data"})
		return
	}

	var user models.User
	if err := database.DB.Where("email = ?", req.Email).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	if user.VerificationCode != req.Code {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid verification code"})
		return
	}

	user.EmailVerified = true
	user.VerificationCode = ""

	if err := database.DB.Save(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update user"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Email verified successfully"})
}

// --- LOGOUT HANDLER --- //
func Logout(c *gin.Context) {
	domain := os.Getenv("COOKIE_DOMAIN")
	isSecure := os.Getenv("ENV") == "production"
	// Clear both tokens (access + refresh)
	c.SetCookie("token", "", -1, "/", domain, isSecure, true)
	c.SetCookie("refresh_token", "", -1, "/", domain, isSecure, true)
	c.JSON(http.StatusOK, gin.H{"message": "Logged out successfully"})
}
