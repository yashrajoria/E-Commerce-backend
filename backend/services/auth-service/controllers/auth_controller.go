package controllers

import (
	"auth-service/database"
	"auth-service/models"
	"auth-service/services"
	"auth-service/types"
	"errors"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// Struct to represent the login request body

func Login(c *gin.Context) {
	var loginReq types.LoginRequest

	// 1. Validate JSON body
	if err := c.ShouldBindJSON(&loginReq); err != nil {
		log.Println(c, "Invalid login request body", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	var user models.User

	// 2. Fetch user by email
	err := database.DB.Where("email = ?", loginReq.Email).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Avoid leaking user existence
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid email or password"})
		} else {
			log.Println(c, "Database error during login", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		}
		return
	}

	// 3. Check role match
	if user.Role != loginReq.Role {
		log.Println(c, "Login attempt with wrong role",
			"email", loginReq.Email,
			"attempted_role", loginReq.Role,
			"actual_role", user.Role)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "You are not authorized to access this resource"})
		return
	}

	// Check if email is verified
	if !user.EmailVerified {
		log.Println(c, "Login attempt with unverified email", "email", loginReq.Email)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Email not verified"})
		return
	}

	// 4. Validate password
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(loginReq.Password)); err != nil {
		log.Println(c, "Invalid password attempt", "email", loginReq.Email)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid email or password"})
		return
	}

	// 5. Generate JWT token
	token, err := services.GenerateJWT(user.ID.String(), user.Email, user.Role)
	if err != nil {
		log.Println(c, "Failed to generate JWT token", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}

	// 6. Set token in HTTP-only cookie (adjust domain in production)
	c.SetCookie("token", token, 86400, "/", "localhost", false, true)

	log.Println(c, "User logged in successfully", "email", user.Email, "role", user.Role)
	// 7. Respond with success (omit token in response for security)
	c.JSON(http.StatusOK, gin.H{"message": "Logged in successfully"})
}

// Register a new user
func Register(c *gin.Context) {
	var registerReq types.RegisterRequest

	// 1. Bind JSON request
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

	// 4. Hash the password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(registerReq.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
		return
	}

	// 5. Create new user
	newUser := models.User{
		ID:       uuid.New(),
		Email:    registerReq.Email,
		Name:     registerReq.Name,
		Password: string(hashedPassword),
		Role:     registerReq.Role,
	}
	newUser.VerificationCode = services.GenerateRandomCode(6)

	// 6. Send verification email
	if err := services.SendVerificationEmail(newUser.Email, newUser.VerificationCode); err != nil {
		log.Println("Error sending verification email:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send verification email"})
		return
	}

	// 7. Insert into PostgreSQL
	if err := database.DB.Create(&newUser).Error; err != nil {
		log.Println("Error inserting user:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create account"})
		return
	}

	// 8. Success response
	c.JSON(http.StatusCreated, gin.H{
		"message": "Account created successfully",
	})
}

// Verify email with the code
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

	// Check if the verification code matches
	if user.VerificationCode != req.Code {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid verification code"})
		return
	}

	// Mark email as verified
	user.EmailVerified = true
	user.VerificationCode = "" // Clear the verification code after success
	if err := database.DB.Save(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update user"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Email verified successfully"})
}

func Logout(c *gin.Context) {
	// Clear the token
	c.SetCookie("token", "", -1, "/", "localhost", false, true)

	c.JSON(http.StatusOK, gin.H{"message": "Logged out successfully"})
	log.Println(c, "User logged out successfully")
}
