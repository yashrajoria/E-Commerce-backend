package controllers

import (
    "errors"
    "net/http"
    "user-service/database"
    "user-service/models"
    "user-service/middleware"
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
