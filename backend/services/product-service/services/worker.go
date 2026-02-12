package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/go-redis/redis/v8"
	"go.uber.org/zap"
)

// StartBulkImportWorker starts a background worker that consumes job IDs from Redis queue
// and processes persisted CSV files using the provided product service.
func StartBulkImportWorker(ctx context.Context, rdb *redis.Client, productSvc *ProductServiceDDB, storageDir string) {
	if rdb == nil || productSvc == nil {
		zap.L().Warn("bulk import worker not started: missing dependencies")
		return
	}

	// ensure storage dir exists
	if storageDir == "" {
		storageDir = "./data/bulk_imports"
	}
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		zap.L().Error("failed to create bulk storage dir", zap.Error(err))
		return
	}

	go func() {
		queueKey := "bulk_import:queue"
		zap.L().Info("bulk import worker started", zap.String("queue", queueKey), zap.String("dir", storageDir))
		for {
			select {
			case <-ctx.Done():
				zap.L().Info("bulk import worker stopping")
				return
			default:
			}

			// BLPop with no timeout will block until an item is available
			res, err := rdb.BLPop(ctx, 0*time.Second, queueKey).Result()
			if err != nil {
				if err == context.Canceled || err == context.DeadlineExceeded {
					return
				}
				zap.L().Error("redis BLPop failed", zap.Error(err))
				time.Sleep(500 * time.Millisecond)
				continue
			}
			if len(res) < 2 {
				continue
			}
			jobID := res[1]
			jobKey := fmt.Sprintf("bulk_import:job:%s", jobID)

			// Fetch job metadata
			val, err := rdb.Get(ctx, jobKey).Result()
			if err != nil {
				zap.L().Error("failed to read job metadata", zap.String("job", jobID), zap.Error(err))
				continue
			}
			var meta map[string]interface{}
			if err := json.Unmarshal([]byte(val), &meta); err != nil {
				zap.L().Error("failed to parse job metadata", zap.String("job", jobID), zap.Error(err))
				continue
			}

			filePath, _ := meta["file_path"].(string)
			// update status -> processing
			meta["status"] = "processing"
			metaB, _ := json.Marshal(meta)
			rdb.Set(ctx, jobKey, metaB, 24*time.Hour)

			// open file and process
			f, err := os.Open(filepath.Clean(filePath))
			if err != nil {
				zap.L().Error("failed to open job file", zap.String("job", jobID), zap.String("path", filePath), zap.Error(err))
				meta["status"] = "failed"
				meta["error"] = err.Error()
				if b, _ := json.Marshal(meta); b != nil {
					rdb.Set(ctx, jobKey, b, 24*time.Hour)
				}
				continue
			}

			result, err := productSvc.ProcessBulkImport(ctx, f)
			// close + remove file
			io.ReadAll(f)
			f.Close()
			if err != nil {
				zap.L().Error("bulk import processing failed", zap.String("job", jobID), zap.Error(err))
				meta["status"] = "failed"
				meta["error"] = err.Error()
				if b, _ := json.Marshal(meta); b != nil {
					rdb.Set(ctx, jobKey, b, 24*time.Hour)
				}
				// attempt to remove file
				_ = os.Remove(filePath)
				continue
			}

			// store result and mark done
			meta["status"] = "done"
			meta["result"] = result
			if b, err := json.Marshal(meta); err == nil {
				rdb.Set(ctx, jobKey, b, 24*time.Hour)
			} else {
				zap.L().Error("failed to marshal job result", zap.Error(err))
			}
			_ = os.Remove(filePath)
		}
	}()
}
