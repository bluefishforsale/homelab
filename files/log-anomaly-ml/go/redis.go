package main

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"
)

const (
	deadLetterKey     = "ml:anomalies:dead_letter"
	deadLetterTTL     = 24 * time.Hour
	dedupeKeyPrefix   = "ml:anomaly:seen:"
	dedupeTTL         = 1 * time.Hour
	alertKeyPrefix    = "ml:alert:sent:"
	alertTTL          = 4 * time.Hour
	rateLimitPrefix   = "ml:rate:"
	rateLimitWindow   = 5 * time.Minute
)

// RedisClient handles Redis operations
type RedisClient struct {
	client *redis.Client
	ctx    context.Context
}

// NewRedisClient creates a new Redis client
func NewRedisClient(addr, password string) (*RedisClient, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       1, // Use DB 1 to separate from Tier 1
		PoolSize: 10,
	})

	ctx := context.Background()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	log.Info("Redis connection established")
	return &RedisClient{client: client, ctx: ctx}, nil
}

// Close closes the Redis connection
func (r *RedisClient) Close() error {
	return r.client.Close()
}

// Ping checks Redis connectivity
func (r *RedisClient) Ping() error {
	return r.client.Ping(r.ctx).Err()
}

// IsDuplicate checks if an anomaly has been seen recently
func (r *RedisClient) IsDuplicate(hash string) (bool, error) {
	key := dedupeKeyPrefix + hash
	exists, err := r.client.Exists(r.ctx, key).Result()
	if err != nil {
		return false, err
	}
	return exists > 0, nil
}

// MarkSeen marks an anomaly as seen
func (r *RedisClient) MarkSeen(hash string) error {
	key := dedupeKeyPrefix + hash
	return r.client.Set(r.ctx, key, "1", dedupeTTL).Err()
}

// HasAlerted checks if we've already alerted for this problem recently
func (r *RedisClient) HasAlerted(problemID string) (bool, error) {
	key := alertKeyPrefix + problemID
	exists, err := r.client.Exists(r.ctx, key).Result()
	if err != nil {
		return false, err
	}
	return exists > 0, nil
}

// MarkAlerted marks a problem as alerted
func (r *RedisClient) MarkAlerted(problemID string) error {
	key := alertKeyPrefix + problemID
	return r.client.Set(r.ctx, key, "1", alertTTL).Err()
}

// CheckRateLimit checks if we're within rate limits
func (r *RedisClient) CheckRateLimit(host, service string, limit int) (bool, error) {
	key := fmt.Sprintf("%s%s:%s", rateLimitPrefix, host, service)
	
	count, err := r.client.Incr(r.ctx, key).Result()
	if err != nil {
		return false, err
	}

	// Set expiry on first increment
	if count == 1 {
		r.client.Expire(r.ctx, key, rateLimitWindow)
	}

	return count <= int64(limit), nil
}

// Dead Letter Queue operations

// PushDeadLetter adds an anomaly to the dead letter queue
func (r *RedisClient) PushDeadLetter(data []byte) error {
	pipe := r.client.Pipeline()
	pipe.LPush(r.ctx, deadLetterKey, data)
	pipe.Expire(r.ctx, deadLetterKey, deadLetterTTL)
	_, err := pipe.Exec(r.ctx)
	return err
}

// PopDeadLetter removes and returns an item from the dead letter queue
func (r *RedisClient) PopDeadLetter() (string, error) {
	return r.client.RPop(r.ctx, deadLetterKey).Result()
}

// GetDeadLetterSize returns the current size of the dead letter queue
func (r *RedisClient) GetDeadLetterSize() (int64, error) {
	return r.client.LLen(r.ctx, deadLetterKey).Result()
}

// Buffer operations for when PostgreSQL is down

const bufferKey = "ml:problems:buffer"
const bufferTTL = 24 * time.Hour

// PushBuffer adds data to the buffer queue
func (r *RedisClient) PushBuffer(data []byte) error {
	pipe := r.client.Pipeline()
	pipe.LPush(r.ctx, bufferKey, data)
	pipe.Expire(r.ctx, bufferKey, bufferTTL)
	_, err := pipe.Exec(r.ctx)
	return err
}

// PopBuffer removes and returns an item from the buffer
func (r *RedisClient) PopBuffer() (string, error) {
	return r.client.RPop(r.ctx, bufferKey).Result()
}

// GetBufferSize returns the current size of the buffer
func (r *RedisClient) GetBufferSize() (int64, error) {
	return r.client.LLen(r.ctx, bufferKey).Result()
}

// FlushAll clears all ML processor Redis data
func (r *RedisClient) FlushAll() error {
	patterns := []string{"ml:*", dedupeKeyPrefix + "*", rateLimitPrefix + "*", alertKeyPrefix + "*"}
	
	for _, pattern := range patterns {
		keys, err := r.client.Keys(r.ctx, pattern).Result()
		if err != nil {
			return fmt.Errorf("failed to get keys for pattern %s: %w", pattern, err)
		}
		if len(keys) > 0 {
			if err := r.client.Del(r.ctx, keys...).Err(); err != nil {
				return fmt.Errorf("failed to delete keys for pattern %s: %w", pattern, err)
			}
		}
	}
	
	return nil
}
