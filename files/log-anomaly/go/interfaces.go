package main

import "time"

// StatisticalAnalyzerInterface defines the interface for statistical analysis backends
type StatisticalAnalyzerInterface interface {
	// Pattern count management
	UpdatePatternCount(patternName, host, service string) error
	
	// Statistical analysis
	CalculateBaselines() error
	DetectFrequencyAnomalies(patternName, host, service string, currentCount int, sigmaThreshold float64) (bool, float64)
	DetectRateChangeAnomalies(patternName, host, service string, rateThreshold float64) (bool, float64)
	
	// Statistics and cleanup
	GetBaselinesCount() (int, error)
	GetPatternStats() (map[string]interface{}, error)
	PurgeOldData(olderThan time.Duration) error
	
	// Connection management
	Close() error
}

// Ensure both implementations satisfy the interface
var (
	_ StatisticalAnalyzerInterface = (*StatisticalAnalyzer)(nil)
	_ StatisticalAnalyzerInterface = (*RedisStatisticalAnalyzer)(nil)
)
