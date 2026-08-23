package controllers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"user-service/middleware"
	"user-service/services"

	"github.com/gin-gonic/gin"
	"github.com/yashrajoria/common/logger"
	"github.com/yashrajoria/common/password"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type UserController struct {
	userService *services.UserService
}

func NewUserController(us *services.UserService) *UserController {
	return &UserController{userService: us}
}

// GetProfile returns the logged-in user's profile
func (ctrl *UserController) GetProfile(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		logger.Warn(c.Request.Context(), "Unauthorized profile access attempt")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	user, err := ctrl.userService.GetUserProfile(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			logger.Warn(c.Request.Context(), "User profile not found", zap.String("user_id", userID))
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		} else {
			logger.Error(c.Request.Context(), "Database error while fetching profile", err, zap.String("user_id", userID))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		}
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

func (ctrl *UserController) UpdateProfile(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		logger.Warn(c.Request.Context(), "Unauthorized update profile attempt")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req struct {
		Name        *string `json:"name"`
		PhoneNumber *string `json:"phone_number"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Warn(c.Request.Context(), "Invalid profile update payload", zap.Error(err), zap.String("user_id", userID))
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload", "details": err.Error()})
		return
	}

	user, err := ctrl.userService.UpdateUserProfile(c.Request.Context(), userID, req.Name, req.PhoneNumber)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			logger.Warn(c.Request.Context(), "User not found for update", zap.String("user_id", userID))
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		} else if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "23505") || strings.Contains(err.Error(), "unique constraint") {
			logger.Warn(c.Request.Context(), "Duplicate phone number or email on update", zap.Error(err), zap.String("user_id", userID))
			c.JSON(http.StatusConflict, gin.H{"error": "phone number already in use"})
		} else {
			logger.Error(c.Request.Context(), "Failed to update profile", err, zap.String("user_id", userID))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update user"})
		}
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

func (ctrl *UserController) ChangePassword(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		logger.Warn(c.Request.Context(), "Unauthorized password change attempt")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req struct {
		OldPassword string `json:"old_password" binding:"required"`
		NewPassword string `json:"new_password" binding:"required,min=8"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Warn(c.Request.Context(), "Invalid password change request", zap.Error(err), zap.String("user_id", userID))
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request", "details": err.Error()})
		return
	}

	user, err := ctrl.userService.GetUserProfile(c.Request.Context(), userID)
	if err != nil {
		logger.Warn(c.Request.Context(), "User not found for password change", zap.String("user_id", userID))
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.OldPassword)); err != nil {
		logger.Warn(c.Request.Context(), "Old password incorrect", zap.String("user_id", userID))
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Old password incorrect"})
		return
	}

	validator := password.NewValidator()
	if err := validator.ValidatePassword(req.NewPassword); err != nil {
		logger.Warn(c.Request.Context(), "Weak password provided", zap.Error(err), zap.String("user_id", userID))
		c.JSON(http.StatusBadRequest, gin.H{"error": "Weak password", "details": err.Error()})
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		logger.Error(c.Request.Context(), "Failed to hash new password", err, zap.String("user_id", userID))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash new password"})
		return
	}

	if err := ctrl.userService.ChangeUserPassword(c.Request.Context(), userID, string(hashedPassword)); err != nil {
		logger.Error(c.Request.Context(), "Failed to update password in DB", err, zap.String("user_id", userID))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update password"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Password updated successfully"})
}

// GetAllUsers returns a paginated list of all users (admin only).
// Passwords are excluded from the response.
func (ctrl *UserController) GetAllUsers(c *gin.Context) {
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

	users, total, err := ctrl.userService.ListUsers(c.Request.Context(), page, pageSize)
	if err != nil {
		logger.Error(c.Request.Context(), "Failed to fetch users list", err)
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
