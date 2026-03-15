package controllers

import (
	"errors"
	"net/http"
	"strconv"

	"user-service/database"
	"user-service/middleware"
	"user-service/models"
	"user-service/services"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// GetProfile returns the logged-in user's profile
func GetProfile(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var user models.User
	err = database.DB.WithContext(c.Request.Context()).
		Where("id = ? AND deleted_at IS NULL", userID).
		First(&user).Error

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":           user.ID,
		"name":         user.Name,
		"email":        user.Email,
		"phone_number": user.PhoneNumber,
		"created_at":   user.CreatedAt,
		"role":         user.Role,
	})
}

func UpdateProfile(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req struct {
		Name        *string `json:"name"`
		PhoneNumber *string `json:"phone_number"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload", "details": err.Error()})
		return
	}

	var user models.User
	err = database.DB.WithContext(c.Request.Context()).
		Where("id = ? AND deleted_at IS NULL", userID).
		First(&user).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	if req.Name != nil {
		user.Name = *req.Name
	}
	if req.PhoneNumber != nil {
		user.PhoneNumber = req.PhoneNumber
	}

	err = database.DB.WithContext(c.Request.Context()).Save(&user).Error
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update user"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Profile updated",
		"user": gin.H{
			"id":           user.ID,
			"name":         user.Name,
			"phone_number": user.PhoneNumber,
		},
	})
}

func ChangePassword(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req struct {
		OldPassword string `json:"old_password" binding:"required"`
		NewPassword string `json:"new_password" binding:"required,min=8"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request", "details": err.Error()})
		return
	}

	var user models.User
	err = database.DB.WithContext(c.Request.Context()).
		Where("id = ? AND deleted_at IS NULL", userID).
		First(&user).Error

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.OldPassword)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Old password incorrect"})
		return
	}

	validator := services.NewPasswordValidator()
	if err := validator.ValidatePassword(req.NewPassword); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Weak password", "details": err.Error()})
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash new password"})
		return
	}

	user.Password = string(hashedPassword)
	err = database.DB.WithContext(c.Request.Context()).Save(&user).Error
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update password"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Password updated successfully"})
}

// GetAllUsers returns a paginated list of all users (admin only).
// Passwords are excluded from the response.
func GetAllUsers(c *gin.Context) {
	pageStr := c.DefaultQuery("page", "1")
	pageSizeStr := c.DefaultQuery("page_size", "20")

	page := 1
	pageSize := 20
	if v, err := strconv.Atoi(pageStr); err == nil && v > 0 {
		page = v
	}
	if v, err := strconv.Atoi(pageSizeStr); err == nil && v > 0 && v <= 100 {
		pageSize = v
	}

	var total int64
	if err := database.DB.WithContext(c.Request.Context()).
		Model(&models.User{}).
		Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to count users"})
		return
	}

	var users []models.User
	offset := (page - 1) * pageSize
	if err := database.DB.WithContext(c.Request.Context()).
		Order("created_at DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch users"})
		return
	}

	// Build response without passwords
	type userResponse struct {
		ID          string  `json:"id"`
		Email       string  `json:"email"`
		Name        string  `json:"name"`
		Role        string  `json:"role"`
		PhoneNumber *string `json:"phone_number"`
		CreatedAt   string  `json:"created_at"`
	}

	result := make([]userResponse, 0, len(users))
	for _, u := range users {
		result = append(result, userResponse{
			ID:          u.ID.String(),
			Email:       u.Email,
			Name:        u.Name,
			Role:        u.Role,
			PhoneNumber: u.PhoneNumber,
			CreatedAt:   u.CreatedAt.Format("2006-01-02T15:04:05Z"),
		})
	}

	totalPages := 0
	if pageSize > 0 {
		totalPages = (int(total) + pageSize - 1) / pageSize
	}

	c.JSON(http.StatusOK, gin.H{
		"users": result,
		"meta": gin.H{
			"page":        page,
			"page_size":   pageSize,
			"total":       total,
			"total_pages": totalPages,
		},
	})
}
