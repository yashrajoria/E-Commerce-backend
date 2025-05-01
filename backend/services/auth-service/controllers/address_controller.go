// /controllers/address_controller.go

package controllers

import (
	"auth-service/database"
	"auth-service/models"
	"auth-service/services"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// CreateAddress handles creating a new address
func CreateAddress(c *gin.Context) {
	// Step 1: Validate the incoming request body (address details)
	var input struct {
		Type       string `json:"type" binding:"required,oneof=shipping billing"`
		Street     string `json:"street" binding:"required"`
		City       string `json:"city" binding:"required"`
		State      string `json:"state" binding:"required"`
		PostalCode string `json:"postal_code" binding:"required"`
		Country    string `json:"country" binding:"required"`
	}

	// Bind the JSON input to the struct
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
		return
	}

	// Step 2: Get the user ID from the JWT token using middleware
	userID, err := services.GetUserIDFromJWT(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Convert userID from string to uuid.UUID if necessary
	parsedUserID, err := uuid.Parse(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid user ID"})
		return
	}

	// Step 3: Create a new address and associate it with the user
	address := models.Address{
		ID:         uuid.New(),       // New UUID for address
		UserID:     parsedUserID,     // Set the user ID
		Type:       input.Type,       // billing or shipping
		Street:     input.Street,     // Street address
		City:       input.City,       // City
		State:      input.State,      // State
		PostalCode: input.PostalCode, // Postal code
		Country:    input.Country,    // Country
	}

	// Step 4: Save the address to the database
	if err := database.DB.Create(&address).Error; err != nil {
		log.Println("Error creating address:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create address"})
		return
	}

	// Step 5: Return success response with the created address details
	c.JSON(http.StatusOK, gin.H{
		"message": "Address created successfully",
		"address": gin.H{
			"id":          address.ID,
			"user_id":     address.UserID,
			"type":        address.Type,
			"street":      address.Street,
			"city":        address.City,
			"state":       address.State,
			"postal_code": address.PostalCode,
			"country":     address.Country,
		},
	})
}
