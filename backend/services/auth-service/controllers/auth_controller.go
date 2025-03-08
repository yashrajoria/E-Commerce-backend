package controllers

import (
	"auth-service/database" // ✅ Import database package
	"auth-service/models"
	"auth-service/services"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// Struct to represent the login request body
type LoginRequest struct {
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// Struct for user registration
type RegisterRequest struct {
	Email       string `json:"email" binding:"required"`
	Password    string `json:"password" binding:"required"`
	PhoneNumber string `json:"phone_number" binding:"required"`
	Role        string `json:"role"`
}

// Login handles user authentication and JWT generation
func Login(c *gin.Context) {
	var loginReq LoginRequest

	// Bind JSON request
	if err := c.ShouldBindJSON(&loginReq); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON body"})
		return
	}

	// Find user by email
	var user models.User
	if err := database.DB.Where("email = ?", loginReq.Email).First(&user).Error; err != nil { // ✅ Use database.DB
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	// Compare hashed password
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(loginReq.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid password"})
		return
	}

	// Generate JWT token
	token, err := services.GenerateJWT(user.ID.String(), user.Email, user.Role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}

	// Return response
	c.JSON(http.StatusOK, gin.H{
		"token": token,
		"user": gin.H{
			"id":           user.ID,
			"email":        user.Email,
			"phone_number": user.PhoneNumber,
			"role":         user.Role,
		},
	})
}

// Register a new user
func Register(c *gin.Context) {
	var registerReq RegisterRequest

	// Bind JSON request
	if err := c.ShouldBindJSON(&registerReq); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON body"})
		return
	}

	// Check if email already exists
	var existingUser models.User
	if err := database.DB.Where("email = ?", registerReq.Email).First(&existingUser).Error; err == nil { // ✅ Use database.DB
		c.JSON(http.StatusConflict, gin.H{"error": "Email already exists"})
		return
	}

	// Hash the password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(registerReq.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
		return
	}

	// Create new user
	newUser := models.User{
		ID:          uuid.New(),
		Email:       registerReq.Email,
		Password:    string(hashedPassword),
		PhoneNumber: registerReq.PhoneNumber,
		Role:        registerReq.Role,
	}

	// Insert into PostgreSQL
	if err := database.DB.Create(&newUser).Error; err != nil { // ✅ Use database.DB
		log.Println("Error inserting user:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create account"})
		return
	}

	// Return success response
	c.JSON(http.StatusCreated, gin.H{
		"message": "Account created successfully",
		"user": gin.H{
			"id":           newUser.ID,
			"email":        newUser.Email,
			"phone_number": newUser.PhoneNumber,
			"role":         newUser.Role,
		},
	})
}
