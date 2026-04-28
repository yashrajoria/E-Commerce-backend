package database

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"cart-service/models"

	"github.com/redis/go-redis/v9"
)

type CartRepository struct {
	client *redis.Client
	ttl    time.Duration
}

func NewCartRepository(client *redis.Client, ttl time.Duration) *CartRepository {
	return &CartRepository{
		client: client,
		ttl:    ttl,
	}
}

func (r *CartRepository) getKey(userID string) string {
	return fmt.Sprintf("cart:user:%s", userID)
}

func (r *CartRepository) GetCart(ctx context.Context, userID string) (*models.Cart, error) {
	key := r.getKey(userID)
	data, err := r.client.Get(ctx, key).Result()
	if err == redis.Nil {
		// No cart found
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var cart models.Cart
	if err := json.Unmarshal([]byte(data), &cart); err != nil {
		return nil, err
	}
	return &cart, nil
}

func (r *CartRepository) SaveCart(ctx context.Context, cart *models.Cart) error {
	key := r.getKey(cart.UserID)
	cart.UpdatedAt = time.Now()

	data, err := json.Marshal(cart)
	if err != nil {
		return err
	}

	return r.client.Set(ctx, key, data, r.ttl).Err()
}

func (r *CartRepository) DeleteCart(ctx context.Context, userID string) error {
	key := r.getKey(userID)
	return r.client.Del(ctx, key).Err()
}

// Idempotency helpers
func (r *CartRepository) getIdemKey(key string) string {
	return "idem:cart:" + key
}

func (r *CartRepository) GetIdempotency(ctx context.Context, key string) (string, error) {
	val, err := r.client.Get(ctx, r.getIdemKey(key)).Result()
	if err == redis.Nil {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return val, nil
}

func (r *CartRepository) SetIdempotency(ctx context.Context, key, orderID string, ttl time.Duration) error {
	return r.client.Set(ctx, r.getIdemKey(key), orderID, ttl).Err()
}
