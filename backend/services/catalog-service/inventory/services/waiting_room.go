package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"time"

	"catalog-service/inventory/models"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

var (
	ErrFlashSaleInactive = errors.New("flash sale is not active for this product")
	ErrInvalidLeaseToken = errors.New("invalid or expired lease token")
)

type WaitingRoomService interface {
	Configure(ctx context.Context, req *models.ConfigureFlashSaleRequest) error
	EnterQueue(ctx context.Context, req *models.EnterQueueRequest) (*models.EnterQueueResponse, error)
	GetStatus(ctx context.Context, productID, userID string) (*models.QueueStatusResponse, error)
	ClaimLease(ctx context.Context, req *models.ClaimLeaseRequest) (bool, error)
	ReleaseLease(ctx context.Context, req *models.ReleaseLeaseRequest) error
	Reset(ctx context.Context, productID string) error
}

type waitingRoomServiceImpl struct {
	rdb    *redis.Client
	logger *zap.Logger
}

func NewWaitingRoomService(rdb *redis.Client, logger *zap.Logger) WaitingRoomService {
	return &waitingRoomServiceImpl{
		rdb:    rdb,
		logger: logger,
	}
}

// Key helpers
func configKey(productID string) string       { return fmt.Sprintf("flash:config:%s", productID) }
func queueKey(productID string) string        { return fmt.Sprintf("flash:queue:%s", productID) }
func userTokenKey(productID, uID string) string { return fmt.Sprintf("flash:user_token:%s:%s", productID, uID) }
func leaseKey(productID, token string) string { return fmt.Sprintf("flash:lease:%s:%s", productID, token) }
func activeLeasesKey(productID string) string { return fmt.Sprintf("flash:active_leases:%s", productID) }

// Lua script to enter queue or immediately acquire lease if slot available
const enterQueueLua = `
local configKey = KEYS[1]
local queueKey = KEYS[2]
local userTokenKey = KEYS[3]
local leaseKey = KEYS[4]
local activeLeasesKey = KEYS[5]

local userId = ARGV[1]
local nowMs = tonumber(ARGV[2])
local candidateToken = ARGV[3]
local quantity = tonumber(ARGV[4])
local leaseTtl = tonumber(ARGV[5])

local config = redis.call('HMGET', configKey, 'active', 'available_slots', 'lease_ttl')
local active = config[1]
if active ~= 'true' then
    return {'INACTIVE', 0, '', 0, 0}
end
local leaseTtl = tonumber(config[3]) or tonumber(ARGV[5]) or 300

-- 1. Self-heal: reclaim expired leases from activeLeasesKey
local expired = redis.call('ZRANGEBYSCORE', activeLeasesKey, '-inf', nowMs)
for _, item in ipairs(expired) do
    local sep = string.find(item, ':')
    if sep then
        local qty = tonumber(string.sub(item, sep + 1)) or 1
        redis.call('HINCRBY', configKey, 'available_slots', qty)
    end
end
redis.call('ZREMRANGEBYSCORE', activeLeasesKey, '-inf', nowMs)

-- 2. Check if user already holds a valid token
local existingToken = redis.call('GET', userTokenKey)
if existingToken and existingToken ~= '' then
    local ttl = redis.call('TTL', userTokenKey)
    if ttl > 0 then
        return {'GRANTED', 0, existingToken, ttl, 0}
    end
end

-- 3. Re-read available slots after expiry reclamation
local updatedConfig = redis.call('HMGET', configKey, 'available_slots')
local availableSlots = tonumber(updatedConfig[1] or '0')
local queueLen = redis.call('ZCARD', queueKey)

-- 4. If queue is empty and enough slots exist, grant immediate lease
if queueLen == 0 and availableSlots >= quantity then
    redis.call('HINCRBY', configKey, 'available_slots', -quantity)
    redis.call('SET', userTokenKey, candidateToken, 'EX', leaseTtl)
    redis.call('HSET', leaseKey, 'user_id', userId, 'quantity', quantity, 'created_at', nowMs)
    redis.call('EXPIRE', leaseKey, leaseTtl)
    local expiresAtMs = nowMs + (leaseTtl * 1000)
    redis.call('ZADD', activeLeasesKey, expiresAtMs, candidateToken .. ':' .. quantity)
    return {'GRANTED', 0, candidateToken, leaseTtl, 0}
end

-- 5. Otherwise, enqueue user into FIFO queue
redis.call('ZADD', queueKey, nowMs, userId)
local rank = redis.call('ZRANK', queueKey, userId)
local pos = (rank and (rank + 1)) or 1
local total = redis.call('ZCARD', queueKey)
return {'QUEUED', pos, '', 0, total}
`

// Lua script to check status and auto-promote head of queue if slot became available
const checkStatusLua = `
local configKey = KEYS[1]
local queueKey = KEYS[2]
local userTokenKey = KEYS[3]
local leaseKeyPrefix = KEYS[4]
local activeLeasesKey = KEYS[5]

local userId = ARGV[1]
local nowMs = tonumber(ARGV[2])
local candidateToken = ARGV[3]
local quantity = tonumber(ARGV[4] or '1')
local config = redis.call('HMGET', configKey, 'active', 'available_slots', 'lease_ttl')
local active = config[1]
if active ~= 'true' then
    return {'INACTIVE', 0, '', 0, 0}
end
local leaseTtl = tonumber(config[3]) or tonumber(ARGV[5]) or 300


-- 1. Self-heal: reclaim expired leases
local expired = redis.call('ZRANGEBYSCORE', activeLeasesKey, '-inf', nowMs)
for _, item in ipairs(expired) do
    local sep = string.find(item, ':')
    if sep then
        local qty = tonumber(string.sub(item, sep + 1)) or 1
        redis.call('HINCRBY', configKey, 'available_slots', qty)
    end
end
redis.call('ZREMRANGEBYSCORE', activeLeasesKey, '-inf', nowMs)

-- 2. Check if user already holds a valid token
local existingToken = redis.call('GET', userTokenKey)
if existingToken and existingToken ~= '' then
    local ttl = redis.call('TTL', userTokenKey)
    if ttl > 0 then
        return {'GRANTED', 0, existingToken, ttl, 0}
    end
end

-- 3. Check rank in queue
local rank = redis.call('ZRANK', queueKey, userId)
if not rank then
    return {'NOT_IN_QUEUE', 0, '', 0, 0}
end

local updatedConfig = redis.call('HMGET', configKey, 'available_slots')
local availableSlots = tonumber(updatedConfig[1] or '0')

-- 4. If user is head of queue (rank == 0) and slot is available, promote!
if rank == 0 and availableSlots >= quantity then
    redis.call('ZREM', queueKey, userId)
    redis.call('HINCRBY', configKey, 'available_slots', -quantity)
    local tokenKey = leaseKeyPrefix .. candidateToken
    redis.call('SET', userTokenKey, candidateToken, 'EX', leaseTtl)
    redis.call('HSET', tokenKey, 'user_id', userId, 'quantity', quantity, 'created_at', nowMs)
    redis.call('EXPIRE', tokenKey, leaseTtl)
    local expiresAtMs = nowMs + (leaseTtl * 1000)
    redis.call('ZADD', activeLeasesKey, expiresAtMs, candidateToken .. ':' .. quantity)
    return {'GRANTED', 0, candidateToken, leaseTtl, 0}
end

local pos = rank + 1
local total = redis.call('ZCARD', queueKey)
return {'QUEUED', pos, '', 0, total}
`

// Lua script to claim lease when placing order
const claimLeaseLua = `
local leaseKey = KEYS[1]
local userTokenKey = KEYS[2]
local activeLeasesKey = KEYS[3]
local userId = ARGV[1]
local token = ARGV[2]

local leaseUser = redis.call('HGET', leaseKey, 'user_id')
if not leaseUser or leaseUser ~= userId then
    return 0
end

local qty = redis.call('HGET', leaseKey, 'quantity') or '1'
redis.call('DEL', leaseKey)
redis.call('DEL', userTokenKey)
redis.call('ZREM', activeLeasesKey, token .. ':' .. qty)
return 1
`

// Lua script to release lease if order fails or is cancelled
const releaseLeaseLua = `
local configKey = KEYS[1]
local leaseKey = KEYS[2]
local userTokenKey = KEYS[3]
local activeLeasesKey = KEYS[4]
local userId = ARGV[1]
local token = ARGV[2]
local fallbackQty = tonumber(ARGV[3]) or 1

local leaseUser = redis.call('HGET', leaseKey, 'user_id')
if leaseUser and leaseUser == userId then
    local qty = tonumber(redis.call('HGET', leaseKey, 'quantity') or fallbackQty)
    redis.call('DEL', leaseKey)
    redis.call('DEL', userTokenKey)
    redis.call('ZREM', activeLeasesKey, token .. ':' .. qty)
    redis.call('HINCRBY', configKey, 'available_slots', qty)
    return 1
end
return 0
`

func generateToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return "fairpass_" + hex.EncodeToString(b)
}

func (s *waitingRoomServiceImpl) Configure(ctx context.Context, req *models.ConfigureFlashSaleRequest) error {
	ttl := req.LeaseTTLSeconds
	if ttl <= 0 {
		ttl = 300 // default 5 minutes
	}

	cKey := configKey(req.ProductID)
	fields := map[string]interface{}{
		"active":          strconv.FormatBool(req.Active),
		"total_slots":     req.TotalSlots,
		"available_slots": req.TotalSlots,
		"lease_ttl":       ttl,
		"updated_at":      time.Now().UTC().Format(time.RFC3339),
	}

	if err := s.rdb.HSet(ctx, cKey, fields).Err(); err != nil {
		return fmt.Errorf("failed to configure flash sale: %w", err)
	}

	s.logger.Info("Configured flash sale waiting room",
		zap.String("product_id", req.ProductID),
		zap.Bool("active", req.Active),
		zap.Int("slots", req.TotalSlots),
		zap.Int("lease_ttl", ttl),
	)
	return nil
}

func (s *waitingRoomServiceImpl) EnterQueue(ctx context.Context, req *models.EnterQueueRequest) (*models.EnterQueueResponse, error) {
	candidateToken := generateToken()
	nowMs := time.Now().UTC().UnixMilli()
	ttl := 300

	keys := []string{
		configKey(req.ProductID),
		queueKey(req.ProductID),
		userTokenKey(req.ProductID, req.UserID),
		leaseKey(req.ProductID, candidateToken),
		activeLeasesKey(req.ProductID),
	}

	res, err := s.rdb.Eval(ctx, enterQueueLua, keys, req.UserID, nowMs, candidateToken, req.Quantity, ttl).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to execute enter queue script: %w", err)
	}

	vals, ok := res.([]interface{})
	if !ok || len(vals) < 5 {
		return nil, fmt.Errorf("unexpected lua result format: %v", res)
	}

	status := fmt.Sprintf("%v", vals[0])
	pos, _ := strconv.ParseInt(fmt.Sprintf("%v", vals[1]), 10, 64)
	token := fmt.Sprintf("%v", vals[2])
	remTTL, _ := strconv.ParseInt(fmt.Sprintf("%v", vals[3]), 10, 64)
	total, _ := strconv.ParseInt(fmt.Sprintf("%v", vals[4]), 10, 64)

	resp := &models.EnterQueueResponse{
		Status:       status,
		ProductID:    req.ProductID,
		Position:     pos,
		TotalInQueue: total,
	}

	if status == "GRANTED" && token != "" {
		resp.LeaseToken = token
		exp := time.Now().UTC().Add(time.Duration(remTTL) * time.Second)
		resp.ExpiresAt = &exp
	} else if status == "QUEUED" {
		// Estimated wait: 15s per person ahead in line
		resp.EstimatedWaitSeconds = pos * 15
	}

	return resp, nil
}

func (s *waitingRoomServiceImpl) GetStatus(ctx context.Context, productID, userID string) (*models.QueueStatusResponse, error) {
	candidateToken := generateToken()
	nowMs := time.Now().UTC().UnixMilli()
	prefix := fmt.Sprintf("flash:lease:%s:", productID)

	keys := []string{
		configKey(productID),
		queueKey(productID),
		userTokenKey(productID, userID),
		prefix,
		activeLeasesKey(productID),
	}

	res, err := s.rdb.Eval(ctx, checkStatusLua, keys, userID, nowMs, candidateToken, 1, 300).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to execute status check script: %w", err)
	}

	vals, ok := res.([]interface{})
	if !ok || len(vals) < 5 {
		return nil, fmt.Errorf("unexpected lua result format: %v", res)
	}

	status := fmt.Sprintf("%v", vals[0])
	pos, _ := strconv.ParseInt(fmt.Sprintf("%v", vals[1]), 10, 64)
	token := fmt.Sprintf("%v", vals[2])
	remTTL, _ := strconv.ParseInt(fmt.Sprintf("%v", vals[3]), 10, 64)
	total, _ := strconv.ParseInt(fmt.Sprintf("%v", vals[4]), 10, 64)

	resp := &models.QueueStatusResponse{
		Status:       status,
		ProductID:    productID,
		Position:     pos,
		TotalInQueue: total,
	}

	if status == "GRANTED" && token != "" {
		resp.LeaseToken = token
		exp := time.Now().UTC().Add(time.Duration(remTTL) * time.Second)
		resp.ExpiresAt = &exp
	}

	return resp, nil
}

func (s *waitingRoomServiceImpl) ClaimLease(ctx context.Context, req *models.ClaimLeaseRequest) (bool, error) {
	keys := []string{
		leaseKey(req.ProductID, req.LeaseToken),
		userTokenKey(req.ProductID, req.UserID),
		activeLeasesKey(req.ProductID),
	}

	res, err := s.rdb.Eval(ctx, claimLeaseLua, keys, req.UserID, req.LeaseToken).Result()
	if err != nil {
		return false, fmt.Errorf("failed to claim lease: %w", err)
	}

	code, _ := strconv.ParseInt(fmt.Sprintf("%v", res), 10, 64)
	return code == 1, nil
}

func (s *waitingRoomServiceImpl) ReleaseLease(ctx context.Context, req *models.ReleaseLeaseRequest) error {
	keys := []string{
		configKey(req.ProductID),
		leaseKey(req.ProductID, req.LeaseToken),
		userTokenKey(req.ProductID, req.UserID),
		activeLeasesKey(req.ProductID),
	}

	_, err := s.rdb.Eval(ctx, releaseLeaseLua, keys, req.UserID, req.LeaseToken, req.Quantity).Result()
	if err != nil {
		return fmt.Errorf("failed to release lease: %w", err)
	}
	return nil
}

func (s *waitingRoomServiceImpl) Reset(ctx context.Context, productID string) error {
	keys := []string{
		configKey(productID),
		queueKey(productID),
		activeLeasesKey(productID),
	}

	// Delete queue and active leases, reset available slots to total
	config, err := s.rdb.HGet(ctx, configKey(productID), "total_slots").Result()
	if err == nil {
		total, _ := strconv.Atoi(config)
		_ = s.rdb.HSet(ctx, configKey(productID), "available_slots", total).Err()
	}

	_ = s.rdb.Del(ctx, queueKey(productID), activeLeasesKey(productID)).Err()
	s.logger.Info("Reset flash sale waiting room", zap.String("product_id", productID), zap.Strings("keys", keys))
	return nil
}
