package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// BulkImportHandler handles bulk product import operations
type BulkImportHandler struct {
	productService ProductServiceAPI
	redis          *redis.Client
	cache          *CacheManager
	validator      *RequestValidator
	timeout        time.Duration
}

func NewBulkImportHandler(ps ProductServiceAPI, redis *redis.Client, cache *CacheManager, validator *RequestValidator) *BulkImportHandler {
	return &BulkImportHandler{
		productService: ps,
		redis:          redis,
		cache:          cache,
		validator:      validator,
		timeout:        DefaultContextTimeout,
	}
}

// ValidateBulkImport validates CSV before import
func (h *BulkImportHandler) ValidateBulkImport(c *gin.Context) {
	file, err := h.getAndValidateFile(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	fileHandle, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to open file"})
		return
	}
	defer fileHandle.Close()

	ctx, cancel := context.WithTimeout(c.Request.Context(), h.timeout)
	defer cancel()

	validation, err := h.productService.ValidateBulkImport(ctx, fileHandle)
	if err != nil {
		zap.L().Error("Bulk import validation failed", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, validation)
}

// CreateBulkProducts imports products from CSV
func (h *BulkImportHandler) CreateBulkProducts(c *gin.Context) {
	file, err := h.getAndValidateFile(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	fileHandle, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to open file"})
		return
	}
	defer fileHandle.Close()

	// Check if async processing is requested
	async := strings.ToLower(strings.TrimSpace(c.Query("async"))) == "true"

	if async {
		h.handleAsyncImport(c, fileHandle)
		return
	}

	h.handleSyncImport(c, fileHandle)
}

// GetBulkImportJobStatus returns the job status/result stored in Redis
func (h *BulkImportHandler) GetBulkImportJobStatus(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Job ID required"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	jobKey := fmt.Sprintf("bulk_import:job:%s", id)
	val, err := h.redis.Get(ctx, jobKey).Result()
	if err == redis.Nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Job not found"})
		return
	}
	if err != nil {
		zap.L().Error("Failed to get job status", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve job status"})
		return
	}

	var jobStatus map[string]interface{}
	if err := json.Unmarshal([]byte(val), &jobStatus); err != nil {
		zap.L().Error("Failed to parse job status", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse job result"})
		return
	}

	c.JSON(http.StatusOK, jobStatus)
}

// Private helper methods

func (h *BulkImportHandler) getAndValidateFile(c *gin.Context) (*multipart.FileHeader, error) {
	file, err := c.FormFile("file")
	if err != nil {
		return nil, fmt.Errorf("file is required")
	}

	if !h.validator.IsValidCSVFile(file) {
		return nil, fmt.Errorf("invalid file type. Only CSV files are allowed")
	}

	if err := h.validator.ValidateFileSize(file); err != nil {
		return nil, err
	}

	return file, nil
}

func (h *BulkImportHandler) handleAsyncImport(c *gin.Context, fileHandle multipart.File) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), h.timeout)
	defer cancel()

	jobID, err := h.enqueueJob(ctx, fileHandle)
	if err != nil {
		zap.L().Error("Failed to enqueue async bulk import", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to queue import job"})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"job_id":  jobID,
		"message": "Import queued for processing",
	})
}

func (h *BulkImportHandler) handleSyncImport(c *gin.Context, fileHandle multipart.File) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), h.timeout)
	defer cancel()

	result, err := h.productService.ProcessBulkImport(ctx, fileHandle)
	if err != nil {
		zap.L().Error("Bulk import processing failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Invalidate cache if any products were created
	if result.InsertedCount > 0 {
		if err := h.cache.Invalidate(ctx); err != nil {
			zap.L().Error("CRITICAL: Failed to invalidate cache after bulk import", zap.Error(err))
		}
	}

	c.JSON(http.StatusOK, result)
}

func (h *BulkImportHandler) enqueueJob(ctx context.Context, fileHandle multipart.File) (string, error) {
	// Read file data
	data, err := io.ReadAll(fileHandle)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	// Setup storage directory
	storageDir := os.Getenv("BULK_STORAGE_DIR")
	if storageDir == "" {
		storageDir = "./data/bulk_imports"
	}
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create storage directory: %w", err)
	}

	// Generate job ID and file path
	jobID := uuid.New().String()
	filename := fmt.Sprintf("%s.csv", jobID)
	filePath := filepath.Join(storageDir, filename)

	// Write file to disk
	if err := os.WriteFile(filePath, data, 0o644); err != nil {
		return "", fmt.Errorf("failed to persist file: %w", err)
	}

	// Create and store job metadata
	if err := h.storeJobMetadata(ctx, jobID, filePath); err != nil {
		os.Remove(filePath)
		return "", err
	}

	// Add job to processing queue
	if err := h.addJobToQueue(ctx, jobID); err != nil {
		os.Remove(filePath)
		h.redis.Del(ctx, fmt.Sprintf("bulk_import:job:%s", jobID))
		return "", err
	}

	zap.L().Info("Bulk import job queued", zap.String("job_id", jobID))
	return jobID, nil
}

func (h *BulkImportHandler) storeJobMetadata(ctx context.Context, jobID, filePath string) error {
	jobInfo := map[string]interface{}{
		"status":     "pending",
		"created_at": time.Now().UTC().Format(time.RFC3339),
		"file_path":  filePath,
	}

	jobData, err := json.Marshal(jobInfo)
	if err != nil {
		return fmt.Errorf("failed to marshal job info: %w", err)
	}

	jobKey := fmt.Sprintf("bulk_import:job:%s", jobID)
	if err := h.redis.Set(ctx, jobKey, jobData, 24*time.Hour).Err(); err != nil {
		return fmt.Errorf("failed to store job metadata: %w", err)
	}

	return nil
}

func (h *BulkImportHandler) addJobToQueue(ctx context.Context, jobID string) error {
	queueKey := "bulk_import:queue"
	if err := h.redis.RPush(ctx, queueKey, jobID).Err(); err != nil {
		return fmt.Errorf("failed to enqueue job: %w", err)
	}
	return nil
}
