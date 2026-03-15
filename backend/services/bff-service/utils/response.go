package utils

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type ResponseMeta struct {
	Page       int `json:"page"`
	PageSize   int `json:"page_size"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

type APIResponse struct {
	Success bool          `json:"success"`
	Data    interface{}   `json:"data"`
	Error   string        `json:"error"`
	Meta    *ResponseMeta `json:"meta,omitempty"`
}

func SuccessResponse(c *gin.Context, data interface{}, meta *ResponseMeta) {
	c.JSON(http.StatusOK, APIResponse{
		Success: true,
		Data:    data,
		Error:   "",
		Meta:    meta,
	})
}

func CreatedResponse(c *gin.Context, data interface{}) {
	c.JSON(http.StatusCreated, APIResponse{
		Success: true,
		Data:    data,
		Error:   "",
	})
}

func ErrorResponse(c *gin.Context, statusCode int, message string) {
	c.JSON(statusCode, APIResponse{
		Success: false,
		Data:    nil,
		Error:   message,
	})
}

func PaginationMeta(page, pageSize, total int) *ResponseMeta {
	totalPages := 0
	if pageSize > 0 {
		totalPages = (total + pageSize - 1) / pageSize
	}

	return &ResponseMeta{
		Page:       page,
		PageSize:   pageSize,
		Total:      total,
		TotalPages: totalPages,
	}
}
