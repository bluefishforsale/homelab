package main

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"
)

// RedisStatisticalAnalyzer performs statistical analysis using Redis
type RedisStatisticalAnalyzer struct {
	client         *redis.Client
	baselineWindow time.Duration
	analysisWindow time.Duration
	ctx            context.Context
}

// NewRedisStatisticalAnalyzer creates a new Redis-based statistical analyzer
func NewRedisStatisticalAnalyzer(redisAddr, redisPassword string) (*RedisStatisticalAnalyzer, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: redisPassword,
		DB:       0,
		PoolSize: 10,
	})

	ctx := context.Background()
	
	// Test connection
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	rsa := &RedisStatisticalAnalyzer{
		client:         client,
		baselineWindow: 24 * time.Hour,
		analysisWindow: 5 * time.Minute,
		ctx:            ctx,
	}

	log.Info("Redis statistical analyzer initialized")
	return rsa, nil
}

// Close closes the Redis connection
func (rsa *RedisStatisticalAnalyzer) Close() error {
	return rsa.client.Close()
}

// UpdatePatternCount updates pattern count for current hour bucket using Redis counters
func (rsa *RedisStatisticalAnalyzer) UpdatePatternCount(patternName, host, service string) error {
	now := time.Now()
	hourBucket := now.Hour()
	
	// Redis key: pattern:{pattern_name}:{host}:{service}:{hour}
	key := fmt.Sprintf("pattern:%s:%s:%s:%d", patternName, host, service, hourBucket)
	
	// Atomic increment with expiration (25 hours to cover timezone changes)
	pipe := rsa.client.Pipeline()
	pipe.Incr(rsa.ctx, key)
	pipe.Expire(rsa.ctx, key, 25*time.Hour)
	
	_, err := pipe.Exec(rsa.ctx)
	if err != nil {
		return fmt.Errorf("failed to update pattern count in Redis: %w", err)
	}
	
	// Also update a timestamp key for this pattern
	timestampKey := fmt.Sprintf("pattern_ts:%s:%s:%s", patternName, host, service)
	if err := rsa.client.Set(rsa.ctx, timestampKey, now.Unix(), 25*time.Hour).Err(); err != nil {
		log.Warnf("Failed to update timestamp for pattern %s: %v", patternName, err)
	}
	
	return nil
}

// CalculateBaselines calculates statistical baselines using Redis data
func (rsa *RedisStatisticalAnalyzer) CalculateBaselines() error {
	log.Info("Calculating baselines from Redis pattern data")
	
	// Get all pattern timestamp keys to find active patterns
	timestampKeys, err := rsa.client.Keys(rsa.ctx, "pattern_ts:*").Result()
	if err != nil {
		return fmt.Errorf("failed to get pattern timestamp keys: %w", err)
	}
	
	baselineCount := 0
	cutoffTime := time.Now().Add(-rsa.baselineWindow)
	
	for _, timestampKey := range timestampKeys {
		// Parse the timestamp key to get pattern info
		parts := strings.Split(timestampKey, ":")
		if len(parts) != 4 {
			continue
		}
		
		patternName := parts[1]
		host := parts[2] 
		service := parts[3]
		
		// Get pattern counts for the last 24 hours
		var counts []int
		now := time.Now()
		
		for h := 0; h < 24; h++ {
			hourTime := now.Add(-time.Duration(h) * time.Hour)
			if hourTime.Before(cutoffTime) {
				continue
			}
			
			hourBucket := hourTime.Hour()
			key := fmt.Sprintf("pattern:%s:%s:%s:%d", patternName, host, service, hourBucket)
			
			countStr, err := rsa.client.Get(rsa.ctx, key).Result()
			if err == redis.Nil {
				counts = append(counts, 0)
			} else if err != nil {
				log.Warnf("Failed to get count for %s: %v", key, err)
				continue
			} else {
				count, err := strconv.Atoi(countStr)
				if err != nil {
					log.Warnf("Invalid count value for %s: %s", key, countStr)
					continue
				}
				counts = append(counts, count)
			}
		}
		
		// Need at least 5 data points for statistical analysis
		if len(counts) < 5 {
			continue
		}
		
		// Calculate statistics
		meanCount, stdCount, minCount, maxCount := rsa.calculateStatistics(counts)
		
		// Store baseline in Redis hash
		baselineKey := fmt.Sprintf("baseline:%s:%s:%s", patternName, host, service)
		baselineData := map[string]interface{}{
			"mean_count":    meanCount,
			"std_count":     stdCount,
			"min_count":     minCount,
			"max_count":     maxCount,
			"sample_count":  len(counts),
			"last_updated":  time.Now().Unix(),
		}
		
		if err := rsa.client.HMSet(rsa.ctx, baselineKey, baselineData).Err(); err != nil {
			log.Errorf("Failed to store baseline for %s/%s/%s: %v", patternName, host, service, err)
			continue
		}
		
		// Set expiration on baseline (48 hours)
		rsa.client.Expire(rsa.ctx, baselineKey, 48*time.Hour)
		
		baselineCount++
	}
	
	log.Infof("Updated %d Redis baselines", baselineCount)
	return nil
}

// calculateStatistics calculates mean, std dev, min, max from counts
func (rsa *RedisStatisticalAnalyzer) calculateStatistics(counts []int) (float64, float64, int, int) {
	if len(counts) == 0 {
		return 0, 1, 0, 0
	}
	
	// Calculate mean
	sum := 0
	minCount := counts[0]
	maxCount := counts[0]
	
	for _, count := range counts {
		sum += count
		if count < minCount {
			minCount = count
		}
		if count > maxCount {
			maxCount = count
		}
	}
	
	meanCount := float64(sum) / float64(len(counts))
	
	// Calculate standard deviation
	if len(counts) < 2 {
		return meanCount, 1.0, minCount, maxCount
	}
	
	sumSquaredDiff := 0.0
	for _, count := range counts {
		diff := float64(count) - meanCount
		sumSquaredDiff += diff * diff
	}
	
	variance := sumSquaredDiff / float64(len(counts)-1)
	stdCount := math.Sqrt(variance)
	
	if stdCount <= 0 {
		stdCount = 1.0
	}
	
	return meanCount, stdCount, minCount, maxCount
}

// DetectFrequencyAnomalies detects if current pattern frequency is anomalous using Redis
func (rsa *RedisStatisticalAnalyzer) DetectFrequencyAnomalies(patternName, host, service string, currentCount int, sigmaThreshold float64) (bool, float64) {
	baselineKey := fmt.Sprintf("baseline:%s:%s:%s", patternName, host, service)
	
	baseline, err := rsa.client.HGetAll(rsa.ctx, baselineKey).Result()
	if err != nil || len(baseline) == 0 {
		return false, 0.0 // No baseline available
	}
	
	meanCountStr, exists := baseline["mean_count"]
	if !exists {
		return false, 0.0
	}
	
	meanCount, err := strconv.ParseFloat(meanCountStr, 64)
	if err != nil {
		return false, 0.0
	}
	
	stdCountStr, exists := baseline["std_count"]
	if !exists {
		return false, 0.0
	}
	
	stdCount, err := strconv.ParseFloat(stdCountStr, 64)
	if err != nil {
		return false, 0.0
	}
	
	sampleCountStr, exists := baseline["sample_count"]
	if !exists {
		return false, 0.0
	}
	
	sampleCount, err := strconv.Atoi(sampleCountStr)
	if err != nil || sampleCount < 5 {
		return false, 0.0
	}
	
	// Ensure std_count is never zero
	if stdCount <= 0 {
		stdCount = 1.0
	}
	
	// Calculate z-score
	zScore := math.Abs(float64(currentCount)-meanCount) / stdCount
	isAnomaly := zScore > sigmaThreshold
	
	return isAnomaly, zScore
}

// DetectRateChangeAnomalies detects sudden rate changes using Redis sorted sets
func (rsa *RedisStatisticalAnalyzer) DetectRateChangeAnomalies(patternName, host, service string, rateThreshold float64) (bool, float64) {
	// Get recent counts (last 6 hours)
	var recentCounts []int
	var olderCounts []int
	
	now := time.Now()
	
	for h := 0; h < 6; h++ {
		hourTime := now.Add(-time.Duration(h) * time.Hour)
		hourBucket := hourTime.Hour()
		key := fmt.Sprintf("pattern:%s:%s:%s:%d", patternName, host, service, hourBucket)
		
		countStr, err := rsa.client.Get(rsa.ctx, key).Result()
		count := 0
		if err == nil {
			if parsedCount, parseErr := strconv.Atoi(countStr); parseErr == nil {
				count = parsedCount
			}
		}
		
		if h < 3 {
			recentCounts = append(recentCounts, count)
		} else {
			olderCounts = append(olderCounts, count)
		}
	}
	
	if len(recentCounts) < 3 || len(olderCounts) == 0 {
		return false, 0.0
	}
	
	recentRate := rsa.average(recentCounts)
	historicalRate := rsa.average(olderCounts)
	
	if historicalRate <= 0 {
		if recentRate > 0 {
			return true, math.Inf(1)
		}
		return false, 0.0
	}
	
	rateChange := recentRate / historicalRate
	isAnomaly := rateChange > rateThreshold || rateChange < (1.0/rateThreshold)
	
	return isAnomaly, rateChange
}

// average calculates the average of a slice of integers
func (rsa *RedisStatisticalAnalyzer) average(nums []int) float64 {
	if len(nums) == 0 {
		return 0.0
	}
	
	sum := 0
	for _, num := range nums {
		sum += num
	}
	
	return float64(sum) / float64(len(nums))
}

// GetBaselinesCount returns the number of established baselines
func (rsa *RedisStatisticalAnalyzer) GetBaselinesCount() (int, error) {
	keys, err := rsa.client.Keys(rsa.ctx, "baseline:*").Result()
	if err != nil {
		return 0, err
	}
	return len(keys), nil
}

// GetPatternStats returns Redis-specific statistics
func (rsa *RedisStatisticalAnalyzer) GetPatternStats() (map[string]interface{}, error) {
	stats := make(map[string]interface{})
	
	// Get pattern count
	patternKeys, err := rsa.client.Keys(rsa.ctx, "pattern:*").Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get pattern keys: %w", err)
	}
	stats["pattern_entries"] = len(patternKeys)
	
	// Get baseline count
	baselineKeys, err := rsa.client.Keys(rsa.ctx, "baseline:*").Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get baseline keys: %w", err)
	}
	stats["baselines"] = len(baselineKeys)
	
	// Get Redis info
	info, err := rsa.client.Info(rsa.ctx, "memory").Result()
	if err == nil {
		stats["redis_info"] = info
	}
	
	return stats, nil
}

// PurgeOldData removes data older than the specified duration
func (rsa *RedisStatisticalAnalyzer) PurgeOldData(olderThan time.Duration) error {
	cutoffTime := time.Now().Add(-olderThan)
	
	// Clean up old pattern counters (Redis TTL should handle this automatically)
	// Clean up old baselines
	baselineKeys, err := rsa.client.Keys(rsa.ctx, "baseline:*").Result()
	if err != nil {
		return fmt.Errorf("failed to get baseline keys for cleanup: %w", err)
	}
	
	purgedCount := 0
	for _, key := range baselineKeys {
		lastUpdatedStr, err := rsa.client.HGet(rsa.ctx, key, "last_updated").Result()
		if err != nil {
			continue
		}
		
		lastUpdated, err := strconv.ParseInt(lastUpdatedStr, 10, 64)
		if err != nil {
			continue
		}
		
		if time.Unix(lastUpdated, 0).Before(cutoffTime) {
			if err := rsa.client.Del(rsa.ctx, key).Err(); err != nil {
				log.Warnf("Failed to delete old baseline %s: %v", key, err)
			} else {
				purgedCount++
			}
		}
	}
	
	if purgedCount > 0 {
		log.Infof("Purged %d old Redis baseline entries", purgedCount)
	}
	
	return nil
}

// Ping checks Redis connectivity
func (rsa *RedisStatisticalAnalyzer) Ping() error {
	return rsa.client.Ping(rsa.ctx).Err()
}

// GetClient returns the Redis client for dead letter operations
func (rsa *RedisStatisticalAnalyzer) GetClient() *redis.Client {
	return rsa.client
}

// IsEmpty checks if baselines are empty (cold start detection)
func (rsa *RedisStatisticalAnalyzer) IsEmpty() (bool, error) {
	keys, err := rsa.client.Keys(rsa.ctx, "baseline:*").Result()
	if err != nil {
		return false, fmt.Errorf("failed to check baseline keys: %w", err)
	}
	return len(keys) == 0, nil
}

// GetBaselineCount returns the number of baseline keys
func (rsa *RedisStatisticalAnalyzer) GetBaselineCount() (int64, error) {
	keys, err := rsa.client.Keys(rsa.ctx, "baseline:*").Result()
	if err != nil {
		return 0, err
	}
	return int64(len(keys)), nil
}

// Dead Letter Queue operations

const deadLetterKey = "anomalies:dead_letter"
const deadLetterTTL = 24 * time.Hour

// PushDeadLetter adds an anomaly to the dead letter queue
func (rsa *RedisStatisticalAnalyzer) PushDeadLetter(anomalyJSON []byte) error {
	pipe := rsa.client.Pipeline()
	pipe.LPush(rsa.ctx, deadLetterKey, anomalyJSON)
	pipe.Expire(rsa.ctx, deadLetterKey, deadLetterTTL)
	_, err := pipe.Exec(rsa.ctx)
	if err != nil {
		return fmt.Errorf("failed to push to dead letter queue: %w", err)
	}
	return nil
}

// GetDeadLetterSize returns the current size of the dead letter queue
func (rsa *RedisStatisticalAnalyzer) GetDeadLetterSize() (int64, error) {
	return rsa.client.LLen(rsa.ctx, deadLetterKey).Result()
}

// PopDeadLetter removes and returns an item from the dead letter queue (for replay)
func (rsa *RedisStatisticalAnalyzer) PopDeadLetter() (string, error) {
	return rsa.client.RPop(rsa.ctx, deadLetterKey).Result()
}
