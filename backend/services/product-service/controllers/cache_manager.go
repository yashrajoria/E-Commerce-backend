package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"product-service/models"

	"github.com/go-redis/redis/v8"
	"go.uber.org/zap"
)

const (
	ProductCachePrefix     = "product:detail:"
	ProductListCachePrefix = "products:v:"
	CacheVersionKey        = "products:version"
)

// CacheManager handles all Redis caching operations
type CacheManager struct {
	redis *redis.Client
	ttl   time.Duration
}

func NewCacheManager(redis *redis.Client) *CacheManager {
	return &CacheManager{
		redis: redis,
		ttl:   DefaultCacheTTL,
	}
}

// GetProductList retrieves a cached product list
func (cm *CacheManager) GetProductList(ctx context.Context, page, perPage int, filters *ProductFilters) (map[string]interface{}, bool) {
	version, err := cm.getCacheVersion(ctx)
	if err != nil || version == 0 {
		return nil, false
	}

	cacheKey := cm.generateListCacheKey(version, page, perPage, filters)
	cachedData, err := cm.redis.Get(ctx, cacheKey).Result()
	if err != nil {
		return nil, false
	}

	var response map[string]interface{}
	if err := json.Unmarshal([]byte(cachedData), &response); err != nil {
		zap.L().Warn("Failed to unmarshal cached product list", zap.Error(err))
		return nil, false
	}

	return response, true
}

// SetProductListAsync caches a product list asynchronously
func (cm *CacheManager) SetProductListAsync(page, perPage int, filters *ProductFilters, response map[string]interface{}) {
	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		version, err := cm.getCacheVersion(bgCtx)
		if err != nil || version == 0 {
			return
		}

		cacheKey := cm.generateListCacheKey(version, page, perPage, filters)
		jsonBytes, err := json.Marshal(response)
		if err != nil {
			zap.L().Warn("Failed to marshal product list for cache", zap.Error(err))
			return
		}

		if err := cm.redis.Set(bgCtx, cacheKey, jsonBytes, cm.ttl).Err(); err != nil {
			zap.L().Warn("Failed to cache product list", zap.Error(err))
		}
	}()
}

// SetProductAsync caches a single product asynchronously
func (cm *CacheManager) SetProductAsync(productID string, product *models.Product) {
	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		cacheKey := ProductCachePrefix + productID
		productJSON, err := json.Marshal(product)
		if err != nil {
			zap.L().Warn("Failed to marshal product for cache", zap.Error(err), zap.String("product_id", productID))
			return
		}

		if err := cm.redis.Set(bgCtx, cacheKey, productJSON, cm.ttl).Err(); err != nil {
			zap.L().Warn("Failed to cache product", zap.Error(err), zap.String("product_id", productID))
		}
	}()
}

// Invalidate invalidates all product caches by bumping the version
func (cm *CacheManager) Invalidate(ctx context.Context) error {
	newVersion, err := cm.redis.Incr(ctx, CacheVersionKey).Result()
	if err != nil {
		return fmt.Errorf("failed to invalidate cache: %w", err)
	}

	zap.L().Info("Cache invalidated", zap.Int64("new_version", newVersion))
	return nil
}

// InvalidateProduct invalidates both list cache and specific product cache
func (cm *CacheManager) InvalidateProduct(ctx context.Context, productID string) {
	// Invalidate list caches
	if err := cm.Invalidate(ctx); err != nil {
		zap.L().Error("CRITICAL: Failed to invalidate cache", zap.Error(err), zap.String("product_id", productID))
	}

	// Delete specific product cache asynchronously
	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		productCacheKey := ProductCachePrefix + productID
		if err := cm.redis.Del(bgCtx, productCacheKey).Err(); err != nil {
			zap.L().Warn("Failed to delete product cache", zap.Error(err), zap.String("product_id", productID))
		}
	}()
}

// getCacheVersion retrieves the current cache version with retry logic
func (cm *CacheManager) getCacheVersion(ctx context.Context) (int64, error) {
	const maxRetries = 3

	for i := 0; i < maxRetries; i++ {
		ver, err := cm.redis.Get(ctx, CacheVersionKey).Int64()
		if err == nil && ver > 0 {
			return ver, nil
		}

		if err == redis.Nil {
			// Initialize version key if it doesn't exist
			if err := cm.redis.Set(ctx, CacheVersionKey, 1, 0).Err(); err == nil {
				return 1, nil
			}
		}

		if i < maxRetries-1 {
			time.Sleep(time.Millisecond * 50)
		}
	}

	return 0, fmt.Errorf("failed to get cache version after %d retries", maxRetries)
}

// generateListCacheKey creates a unique cache key for product lists
func (cm *CacheManager) generateListCacheKey(version int64, page, perPage int, filters *ProductFilters) string {
	return fmt.Sprintf(
		"%s%d:p:%d:l:%d:f:%s:c:%s:s:%s:min:%s:max:%s",
		ProductListCachePrefix,
		version,
		page,
		perPage,
		filters.IsFeatured,
		filters.CategoryKey,
		filters.SortParam,
		formatFloatForCache(filters.MinPrice),
		formatFloatForCache(filters.MaxPrice),
	)
}

func formatFloatForCache(value *float64) string {
	if value == nil {
		return ""
	}
	return strconv.FormatFloat(*value, 'f', -1, 64)
}
