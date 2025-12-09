package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"
)

const (
	jobQueueKey      = "aicorp:jobs:pending"
	jobProcessingKey = "aicorp:jobs:processing"
	rateLimitPrefix  = "aicorp:ratelimit:"
	cachePrefix      = "aicorp:cache:"
)

// RedisClient handles Redis operations
type RedisClient struct {
	client *redis.Client
	ctx    context.Context
}

// JobPayload represents a job in the queue
type JobPayload struct {
	RunID      uuid.UUID `json:"run_id"`
	TemplateID string    `json:"template_id"`
	Priority   int       `json:"priority"`
	CreatedAt  time.Time `json:"created_at"`
}

// NewRedisClient creates a new Redis client
func NewRedisClient(config *Config) (*RedisClient, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     config.RedisAddr,
		Password: config.RedisPassword,
		DB:       config.RedisDB,
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

// EnqueueJob adds a job to the queue
func (r *RedisClient) EnqueueJob(job JobPayload) error {
	data, err := json.Marshal(job)
	if err != nil {
		return err
	}

	// Use LPUSH for FIFO when combined with RPOP
	if err := r.client.LPush(r.ctx, jobQueueKey, data).Err(); err != nil {
		return err
	}

	queueDepth.Inc()
	log.WithFields(log.Fields{
		"run_id":   job.RunID,
		"template": job.TemplateID,
	}).Debug("Job enqueued")

	return nil
}

// DequeueJob removes and returns the next job from the queue
func (r *RedisClient) DequeueJob() (*JobPayload, error) {
	// Move from pending to processing atomically
	data, err := r.client.RPopLPush(r.ctx, jobQueueKey, jobProcessingKey).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var job JobPayload
	if err := json.Unmarshal([]byte(data), &job); err != nil {
		return nil, err
	}

	queueDepth.Dec()
	return &job, nil
}

// CompleteJob removes a job from the processing queue
func (r *RedisClient) CompleteJob(job JobPayload) error {
	data, _ := json.Marshal(job)
	return r.client.LRem(r.ctx, jobProcessingKey, 1, data).Err()
}

// RequeueJob moves a job back to the pending queue (for retries)
func (r *RedisClient) RequeueJob(job JobPayload) error {
	data, _ := json.Marshal(job)
	// Remove from processing
	r.client.LRem(r.ctx, jobProcessingKey, 1, data)
	// Add back to pending
	return r.client.LPush(r.ctx, jobQueueKey, data).Err()
}

// GetQueueDepth returns the current queue size
func (r *RedisClient) GetQueueDepth() (int64, error) {
	return r.client.LLen(r.ctx, jobQueueKey).Result()
}

// CheckRateLimit checks if a request is within rate limits
func (r *RedisClient) CheckRateLimit(key string, limit int, window time.Duration) (bool, error) {
	fullKey := rateLimitPrefix + key

	count, err := r.client.Incr(r.ctx, fullKey).Result()
	if err != nil {
		return false, err
	}

	// Set expiry on first increment
	if count == 1 {
		r.client.Expire(r.ctx, fullKey, window)
	}

	return count <= int64(limit), nil
}

// GetCache retrieves a cached value
func (r *RedisClient) GetCache(key string) (string, error) {
	fullKey := cachePrefix + key
	val, err := r.client.Get(r.ctx, fullKey).Result()
	if err == redis.Nil {
		return "", nil
	}
	return val, err
}

// SetCache stores a value in cache
func (r *RedisClient) SetCache(key string, value string, ttl time.Duration) error {
	fullKey := cachePrefix + key
	return r.client.Set(r.ctx, fullKey, value, ttl).Err()
}

// DeleteCache removes a cached value
func (r *RedisClient) DeleteCache(key string) error {
	fullKey := cachePrefix + key
	return r.client.Del(r.ctx, fullKey).Err()
}

// PublishUpdate publishes a workflow update for WebSocket subscribers
func (r *RedisClient) PublishUpdate(channel string, msg WebSocketMessage) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return r.client.Publish(r.ctx, "aicorp:updates:"+channel, data).Err()
}

// Subscribe subscribes to workflow updates
func (r *RedisClient) Subscribe(channel string) *redis.PubSub {
	return r.client.Subscribe(r.ctx, "aicorp:updates:"+channel)
}

// FlushAll clears all AI Corp Redis data
func (r *RedisClient) FlushAll() error {
	patterns := []string{"aicorp:*"}

	for _, pattern := range patterns {
		keys, err := r.client.Keys(r.ctx, pattern).Result()
		if err != nil {
			return fmt.Errorf("failed to get keys for pattern %s: %w", pattern, err)
		}
		if len(keys) > 0 {
			if err := r.client.Del(r.ctx, keys...).Err(); err != nil {
				return fmt.Errorf("failed to delete keys: %w", err)
			}
		}
	}

	return nil
}

// RecoverProcessingJobs moves any jobs stuck in processing back to pending queue
// Called on startup to recover from unclean shutdown
func (r *RedisClient) RecoverProcessingJobs() (int, error) {
	count := 0
	for {
		data, err := r.client.RPopLPush(r.ctx, jobProcessingKey, jobQueueKey).Result()
		if err == redis.Nil {
			break
		}
		if err != nil {
			return count, err
		}
		if data != "" {
			count++
		}
	}
	if count > 0 {
		log.Infof("Recovered %d processing jobs to pending queue", count)
	}
	return count, nil
}

// PersistState saves current state for recovery
func (r *RedisClient) PersistState() error {
	// Move all processing jobs back to pending for recovery
	count, err := r.RecoverProcessingJobs()
	if err != nil {
		return err
	}
	if count > 0 {
		log.Infof("Persisted %d in-progress jobs for recovery", count)
	}
	return nil
}

// AddToDeadLetter adds a failed job to the dead letter queue
func (r *RedisClient) AddToDeadLetter(job JobPayload, reason string) error {
	dlEntry := map[string]interface{}{
		"job":       job,
		"reason":    reason,
		"failed_at": time.Now(),
	}
	data, err := json.Marshal(dlEntry)
	if err != nil {
		return err
	}
	return r.client.LPush(r.ctx, "aicorp:jobs:dead_letter", data).Err()
}

// GetDeadLetterSize returns the number of jobs in the dead letter queue
func (r *RedisClient) GetDeadLetterSize() (int64, error) {
	return r.client.LLen(r.ctx, "aicorp:jobs:dead_letter").Result()
}

// CacheContext stores workflow context for continuation
func (r *RedisClient) CacheContext(runID uuid.UUID, ctx map[string]interface{}, ttl time.Duration) error {
	data, err := json.Marshal(ctx)
	if err != nil {
		return err
	}
	return r.client.Set(r.ctx, "aicorp:context:"+runID.String(), data, ttl).Err()
}

// GetCachedContext retrieves cached workflow context
func (r *RedisClient) GetCachedContext(runID uuid.UUID) (map[string]interface{}, error) {
	data, err := r.client.Get(r.ctx, "aicorp:context:"+runID.String()).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var ctx map[string]interface{}
	if err := json.Unmarshal([]byte(data), &ctx); err != nil {
		return nil, err
	}
	return ctx, nil
}
