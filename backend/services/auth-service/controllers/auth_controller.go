package controllers

import (
	"auth-service/database"
	"auth-service/models"
	"auth-service/services"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"net/smtp"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"golang.org/x/crypto/bcrypt"
)

// Struct to represent the login request body
type LoginRequest struct {
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// Struct for user registration
type RegisterRequest struct {
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
	// PhoneNumber string `json:"phone_number"`
	Role string `json:"role"`
}

type AddressRequest struct {
	Type       string `json:"type" binding:"required,oneof=billing shipping"`
	Street     string `json:"street" binding:"required"`
	City       string `json:"city" binding:"required"`
	State      string `json:"state" binding:"required"`
	PostalCode string `json:"postal_code" binding:"required"`
	Country    string `json:"country" binding:"required"`
}

// Login handles user authentication and JWT generation
func Login(c *gin.Context) {
	var loginReq LoginRequest

	if err := c.ShouldBindJSON(&loginReq); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON body"})
		return
	}

	var user models.User

	err := database.DB.Where("email = ?", loginReq.Email).First(&user).Error

	if err != nil {
		if err.Error() == "record not found" {
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
			return
		}
	}

	if err := database.DB.Where("email = ?", loginReq.Email).First(&user).Error; err != nil {

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
	log.Println("Generated token:", token)

	// Set token in HTTP-Only cookie
	c.SetCookie("token", token, 86400, "/", "localhost", false, true)
	c.JSON(http.StatusOK, gin.H{"message": "Logged in"})
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
	if err := database.DB.Where("email = ?", registerReq.Email).First(&existingUser).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Email already exists"})
		return
	}

	// Hash the password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(registerReq.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
		return
	}

	if err := godotenv.Load(); err != nil {
		log.Println("Warning: No .env file found, using system environment variables")
	}

	// Create new user
	newUser := models.User{
		ID:       uuid.New(),
		Email:    registerReq.Email,
		Password: string(hashedPassword),
		// PhoneNumber: registerReq.PhoneNumber,
		Role: registerReq.Role,
	}
	newUser.VerificationCode = generateRandomCode(6)

	// Send verification email
	if err := sendVerificationEmail(newUser.Email, newUser.VerificationCode); err != nil {
		log.Println("Error sending verification email:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send verification email"})
		return
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
			"id":    newUser.ID,
			"email": newUser.Email,
			"role":  newUser.Role,
		},
	})
}

//Helper functions for verification code generation and email sending

func generateRandomCode(length int) string {
	log.Println("Generating random code of length:", length)
	code := ""

	for i := 0; i < length; i++ {
		code += fmt.Sprintf("%d", rand.Intn(10))
	}

	return code
}

// Helper function to send verification email
func sendVerificationEmail(to string, code string) error {
	log.Println("Attempting to send verification email to:", to)
	log.Println("Verification code:", code)

	from := os.Getenv("SMTP_EMAIL")
	password := os.Getenv("SMTP_PASSWORD")
	smtpServer := "smtp.gmail.com"
	port := "587"
	log.Println(from, password)
	if from == "" || password == "" {
		log.Println("SMTP_EMAIL or SMTP_PASSWORD environment variable is missing")
		return fmt.Errorf("SMTP configuration is missing")
	}

	// Set up email content
	subject := "Email Verification"
	body := fmt.Sprintf("Your verification code is: %s", code)
	message := []byte("Subject: " + subject + "\r\n" + "To: " + to + "\r\n" + "\r\n" + body)

	// Auth configuration for SMTP
	auth := smtp.PlainAuth("", from, password, smtpServer)
	err := smtp.SendMail(smtpServer+":"+port, auth, from, []string{to}, message)
	if err != nil {
		log.Println("Failed to send email:", err)
		return err
	}

	log.Println("Verification email sent successfully to:", to)
	return nil
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
