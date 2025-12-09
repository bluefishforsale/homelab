package main

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestJobPayloadJSON(t *testing.T) {
	job := JobPayload{
		RunID:      uuid.New(),
		TemplateID: "test-template",
		Priority:   1,
		CreatedAt:  time.Now(),
	}

	// Verify fields are set correctly
	if job.TemplateID != "test-template" {
		t.Errorf("Expected template ID 'test-template', got %s", job.TemplateID)
	}
	if job.Priority != 1 {
		t.Errorf("Expected priority 1, got %d", job.Priority)
	}
	if job.RunID == uuid.Nil {
		t.Error("Expected RunID to be set")
	}
}

func TestRedisKeyConstants(t *testing.T) {
	// Verify key prefixes are set correctly
	if jobQueueKey != "aicorp:jobs:pending" {
		t.Errorf("Expected jobQueueKey 'aicorp:jobs:pending', got %s", jobQueueKey)
	}
	if jobProcessingKey != "aicorp:jobs:processing" {
		t.Errorf("Expected jobProcessingKey 'aicorp:jobs:processing', got %s", jobProcessingKey)
	}
	if rateLimitPrefix != "aicorp:ratelimit:" {
		t.Errorf("Expected rateLimitPrefix 'aicorp:ratelimit:', got %s", rateLimitPrefix)
	}
	if cachePrefix != "aicorp:cache:" {
		t.Errorf("Expected cachePrefix 'aicorp:cache:', got %s", cachePrefix)
	}
}

// Integration tests require actual Redis connection
// These tests verify the interface contracts

func TestRedisClientInterface(t *testing.T) {
	// Verify RedisClient has expected methods
	var _ interface {
		Close() error
		Ping() error
		EnqueueJob(JobPayload) error
		DequeueJob() (*JobPayload, error)
		CompleteJob(JobPayload) error
		RequeueJob(JobPayload) error
		GetQueueDepth() (int64, error)
		CheckRateLimit(string, int, time.Duration) (bool, error)
		GetCache(string) (string, error)
		SetCache(string, string, time.Duration) error
		DeleteCache(string) error
		PublishUpdate(string, WebSocketMessage) error
		FlushAll() error
		RecoverProcessingJobs() (int, error)
		PersistState() error
		AddToDeadLetter(JobPayload, string) error
		GetDeadLetterSize() (int64, error)
		CacheContext(uuid.UUID, map[string]interface{}, time.Duration) error
		GetCachedContext(uuid.UUID) (map[string]interface{}, error)
	} = (*RedisClient)(nil)
}
