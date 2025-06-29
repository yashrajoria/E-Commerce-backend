package controllers

import (

	// "user-service/middleware"
	"fmt"
	"log"
	"net/http"
	"user-service/database"
	"user-service/models"

	"github.com/gin-gonic/gin"
)

func GetUserClaimsFromHeader(c *gin.Context) (string, error) {
	userID := c.GetHeader("X-User-ID")
	// email := c.GetHeader("X-Email")

	if userID == "" {
		return "", fmt.Errorf("missing user identity headers")
	}
	return userID, nil
}

// GetProfile returns the logged-in user's profile
func GetProfile(c *gin.Context) {
	log.Println("Fetching user profile...")

	userID, err := GetUserClaimsFromHeader(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var user models.User
	if err := database.DB.First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":    user.ID,
		"name":  user.Name,
		"email": user.Email,
		"role":  user.Role,
	})
}

func UpdateProfile(c *gin.Context) {
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req struct {
		Name  *string `json:"name"`
		Email *string `json:"email"` // Uncomment if you want to allow email updates

		// Email is ignored completely
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	var user models.User
	if err := database.DB.First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	if req.Name != nil {
		user.Name = *req.Name
	}

	if err := database.DB.Save(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update user"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Profile updated successfully"})
}
