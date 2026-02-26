package controllers

import (
	"math"
	"net/http"
	"notification-service/middleware"
	"notification-service/models"
	"notification-service/services"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type NotificationController struct {
	notificationService services.NotificationService
	logger              *zap.Logger
}

func NewNotificationController(svc services.NotificationService, logger *zap.Logger) *NotificationController {
	return &NotificationController{notificationService: svc, logger: logger}
}

const (
	maxPageSize     = 100
	defaultPage     = 1
	defaultPageSize = 20
)

func parsePaginationParams(ctx *gin.Context) (int, int) {
	page := defaultPage
	pageSize := defaultPageSize

	if p, err := strconv.Atoi(ctx.DefaultQuery("page", "1")); err == nil && p > 0 {
		page = p
	}
	if l, err := strconv.Atoi(ctx.DefaultQuery("page_size", "20")); err == nil && l > 0 {
		pageSize = l
		if pageSize > maxPageSize {
			pageSize = maxPageSize
		}
	}
	return page, pageSize
}

func (cc *NotificationController) GetNotificationLogs(ctx *gin.Context) {
	// Parse optional user_id filter
	var userID int64
	if userIDStr := ctx.Query("user_id"); userIDStr != "" {
		parsed, err := strconv.ParseInt(userIDStr, 10, 64)
		if err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid user_id"})
			return
		}
		userID = parsed
	}

	page, pageSize := parsePaginationParams(ctx)

	filter := models.NotificationFilter{
		UserID:   userID,
		Status:   ctx.Query("status"),
		Channel:  ctx.Query("channel"),
		Page:     page,
		PageSize: pageSize,
	}

	logs, total, err := cc.notificationService.GetLogs(ctx.Request.Context(), filter)
	if err != nil {
		cc.logger.Error("failed to get notification logs",
			zap.Error(err),
			zap.Int64("requested_by", middleware.GetUserIDInt(ctx)),
		)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	totalPages := int(math.Ceil(float64(total) / float64(pageSize)))

	ctx.JSON(http.StatusOK, gin.H{
		"data":        logs,
		"total":       total,
		"page":        page,
		"page_size":   pageSize,
		"total_pages": totalPages,
	})
}
