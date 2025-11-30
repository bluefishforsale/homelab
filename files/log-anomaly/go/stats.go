package main

import (
	"database/sql"
	"fmt"
	"math"
	"time"

	_ "github.com/mattn/go-sqlite3"
	log "github.com/sirupsen/logrus"
)

// StatisticalAnalyzer performs statistical analysis on log patterns
type StatisticalAnalyzer struct {
	db             *sql.DB
	baselineWindow time.Duration
	analysisWindow time.Duration
}

// NewStatisticalAnalyzer creates a new statistical analyzer
func NewStatisticalAnalyzer(dbPath string) (*StatisticalAnalyzer, error) {
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_sync=NORMAL&_cache_size=-64000")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Test the connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	sa := &StatisticalAnalyzer{
		db:             db,
		baselineWindow: 24 * time.Hour,
		analysisWindow: 5 * time.Minute,
	}

	if err := sa.initDatabase(); err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	log.Info("Statistical analyzer initialized with SQLite database")
	return sa, nil
}

// initDatabase initializes the SQLite database
func (sa *StatisticalAnalyzer) initDatabase() error {
	queries := []string{
		// Enable foreign keys
		`PRAGMA foreign_keys = ON`,
		
		// Create tables
		`CREATE TABLE IF NOT EXISTS log_patterns (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			pattern_name TEXT NOT NULL,
			host TEXT NOT NULL,
			service TEXT NOT NULL,
			count INTEGER NOT NULL DEFAULT 1,
			timestamp DATETIME NOT NULL,
			hour_bucket INTEGER NOT NULL,
			UNIQUE(pattern_name, host, service, hour_bucket) ON CONFLICT REPLACE
		)`,
		
		`CREATE TABLE IF NOT EXISTS log_frequency_baselines (
			pattern_name TEXT NOT NULL,
			host TEXT NOT NULL,
			service TEXT NOT NULL,
			mean_count REAL NOT NULL,
			std_count REAL NOT NULL,
			min_count INTEGER NOT NULL,
			max_count INTEGER NOT NULL,
			sample_count INTEGER NOT NULL,
			last_updated DATETIME NOT NULL,
			PRIMARY KEY(pattern_name, host, service)
		)`,
		
		// Create indexes for better performance
		`CREATE INDEX IF NOT EXISTS idx_log_patterns_timestamp ON log_patterns(timestamp)`,
		`CREATE INDEX IF NOT EXISTS idx_log_patterns_lookup ON log_patterns(pattern_name, host, service, hour_bucket)`,
		`CREATE INDEX IF NOT EXISTS idx_baselines_lookup ON log_frequency_baselines(pattern_name, host, service)`,
	}

	for _, query := range queries {
		if _, err := sa.db.Exec(query); err != nil {
			return fmt.Errorf("failed to execute query '%s': %w", query, err)
		}
	}

	log.Info("Database schema initialized successfully")
	return nil
}

// Close closes the database connection
func (sa *StatisticalAnalyzer) Close() error {
	return sa.db.Close()
}

// UpdatePatternCount updates pattern count for current hour bucket
func (sa *StatisticalAnalyzer) UpdatePatternCount(patternName, host, service string) error {
	now := time.Now()
	hourBucket := now.Hour()

	query := `
		INSERT OR REPLACE INTO log_patterns 
		(pattern_name, host, service, count, timestamp, hour_bucket)
		VALUES (?, ?, ?, 
			COALESCE((SELECT count FROM log_patterns 
					 WHERE pattern_name=? AND host=? AND service=? AND hour_bucket=?), 0) + 1,
			?, ?)
	`

	_, err := sa.db.Exec(query, 
		patternName, host, service,           // INSERT values
		patternName, host, service, hourBucket, // SELECT parameters
		now, hourBucket,                      // remaining INSERT values
	)
	
	if err != nil {
		return fmt.Errorf("failed to update pattern count: %w", err)
	}
	
	return nil
}

// CalculateBaselines calculates statistical baselines for all patterns
func (sa *StatisticalAnalyzer) CalculateBaselines() error {
	cutoffTime := time.Now().Add(-sa.baselineWindow)

	// First, clean up old data beyond the baseline window
	cleanupQuery := `DELETE FROM log_patterns WHERE timestamp < ?`
	if _, err := sa.db.Exec(cleanupQuery, cutoffTime.Add(-24*time.Hour)); err != nil {
		log.Warnf("Failed to cleanup old pattern data: %v", err)
	}

	query := `
		SELECT 
			pattern_name, host, service, 
			AVG(count) as mean_count,
			CASE WHEN COUNT(*) > 1 THEN 
				SQRT(SUM((count - avg_sub.avg_count) * (count - avg_sub.avg_count)) / (COUNT(*) - 1))
			ELSE 1.0 END as std_count,
			MIN(count) as min_count, 
			MAX(count) as max_count, 
			COUNT(*) as sample_count
		FROM (
			SELECT 
				pattern_name, host, service, count,
				AVG(count) OVER (PARTITION BY pattern_name, host, service) as avg_count
			FROM log_patterns 
			WHERE timestamp >= ?
		) as avg_sub
		GROUP BY pattern_name, host, service
		HAVING COUNT(*) >= 3
	`

	rows, err := sa.db.Query(query, cutoffTime)
	if err != nil {
		return fmt.Errorf("failed to query baselines: %w", err)
	}
	defer rows.Close()

	// Prepare the update statement
	updateQuery := `
		INSERT OR REPLACE INTO log_frequency_baselines
		(pattern_name, host, service, mean_count, std_count, min_count, max_count, sample_count, last_updated)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	stmt, err := sa.db.Prepare(updateQuery)
	if err != nil {
		return fmt.Errorf("failed to prepare baseline update statement: %w", err)
	}
	defer stmt.Close()

	baselineCount := 0
	for rows.Next() {
		var patternName, host, service string
		var meanCount, stdCount float64
		var minCount, maxCount, sampleCount int

		if err := rows.Scan(&patternName, &host, &service, &meanCount, &stdCount, &minCount, &maxCount, &sampleCount); err != nil {
			log.Errorf("Failed to scan baseline row: %v", err)
			continue
		}

		// Ensure std_count is never zero to avoid division by zero
		if stdCount <= 0 {
			stdCount = 1.0
		}

		if _, err := stmt.Exec(patternName, host, service, meanCount, stdCount, minCount, maxCount, sampleCount, time.Now()); err != nil {
			log.Errorf("Failed to update baseline for %s/%s/%s: %v", patternName, host, service, err)
			continue
		}

		baselineCount++
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating baseline rows: %w", err)
	}

	log.Infof("Updated %d statistical baselines", baselineCount)
	return nil
}

// DetectFrequencyAnomalies detects if current pattern frequency is anomalous
func (sa *StatisticalAnalyzer) DetectFrequencyAnomalies(patternName, host, service string, currentCount int, sigmaThreshold float64) (bool, float64) {
	query := `
		SELECT mean_count, std_count, min_count, max_count, sample_count
		FROM log_frequency_baselines
		WHERE pattern_name=? AND host=? AND service=?
	`

	var meanCount, stdCount float64
	var minCount, maxCount, sampleCount int

	err := sa.db.QueryRow(query, patternName, host, service).Scan(&meanCount, &stdCount, &minCount, &maxCount, &sampleCount)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, 0.0 // No baseline available
		}
		log.Errorf("Failed to query baseline for %s/%s/%s: %v", patternName, host, service, err)
		return false, 0.0
	}

	// Need sufficient samples for reliable statistical analysis
	if sampleCount < 5 {
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

// DetectRateChangeAnomalies detects sudden rate changes in pattern frequency
func (sa *StatisticalAnalyzer) DetectRateChangeAnomalies(patternName, host, service string, rateThreshold float64) (bool, float64) {
	recentCutoff := time.Now().Add(-6 * time.Hour)

	query := `
		SELECT count, timestamp
		FROM log_patterns
		WHERE pattern_name=? AND host=? AND service=? AND timestamp >= ?
		ORDER BY timestamp DESC
		LIMIT 6
	`

	rows, err := sa.db.Query(query, patternName, host, service, recentCutoff)
	if err != nil {
		log.Errorf("Failed to query rate change data for %s/%s/%s: %v", patternName, host, service, err)
		return false, 0.0
	}
	defer rows.Close()

	var counts []int
	for rows.Next() {
		var count int
		var timestamp time.Time
		if err := rows.Scan(&count, &timestamp); err != nil {
			log.Errorf("Failed to scan rate change row: %v", err)
			continue
		}
		counts = append(counts, count)
	}

	if err := rows.Err(); err != nil {
		log.Errorf("Error iterating rate change rows: %v", err)
		return false, 0.0
	}

	if len(counts) < 3 {
		return false, 0.0 // Not enough data
	}

	// Calculate recent rate vs historical rate
	recentCount := 3
	if len(counts) < recentCount {
		recentCount = len(counts)
	}
	
	recentRate := average(counts[:recentCount])     // Most recent hours
	historicalRate := average(counts[recentCount:]) // Previous hours

	if historicalRate <= 0 {
		if recentRate > 0 {
			return true, 999.0 // Use large finite value instead of +Inf (JSON incompatible)
		}
		return false, 0.0
	}

	rateChange := recentRate / historicalRate
	isAnomaly := rateChange > rateThreshold || rateChange < (1.0/rateThreshold)

	return isAnomaly, rateChange
}

// GetBaselinesCount returns the number of established baselines
func (sa *StatisticalAnalyzer) GetBaselinesCount() (int, error) {
	var count int
	err := sa.db.QueryRow("SELECT COUNT(*) FROM log_frequency_baselines").Scan(&count)
	return count, err
}

// GetPatternStats returns statistics about pattern storage
func (sa *StatisticalAnalyzer) GetPatternStats() (map[string]interface{}, error) {
	stats := make(map[string]interface{})
	
	// Get pattern count
	var patternCount int
	if err := sa.db.QueryRow("SELECT COUNT(*) FROM log_patterns").Scan(&patternCount); err != nil {
		return nil, fmt.Errorf("failed to get pattern count: %w", err)
	}
	stats["pattern_entries"] = patternCount
	
	// Get baseline count
	var baselineCount int
	if err := sa.db.QueryRow("SELECT COUNT(*) FROM log_frequency_baselines").Scan(&baselineCount); err != nil {
		return nil, fmt.Errorf("failed to get baseline count: %w", err)
	}
	stats["baselines"] = baselineCount
	
	// Get oldest and newest patterns
	var oldestTime, newestTime string
	if err := sa.db.QueryRow("SELECT MIN(timestamp), MAX(timestamp) FROM log_patterns").Scan(&oldestTime, &newestTime); err == nil {
		stats["oldest_pattern"] = oldestTime
		stats["newest_pattern"] = newestTime
	}
	
	return stats, nil
}

// PurgeOldData removes data older than the specified duration
func (sa *StatisticalAnalyzer) PurgeOldData(olderThan time.Duration) error {
	cutoffTime := time.Now().Add(-olderThan)
	
	// Remove old patterns
	result, err := sa.db.Exec("DELETE FROM log_patterns WHERE timestamp < ?", cutoffTime)
	if err != nil {
		return fmt.Errorf("failed to purge old patterns: %w", err)
	}
	
	if rowsAffected, err := result.RowsAffected(); err == nil && rowsAffected > 0 {
		log.Infof("Purged %d old pattern entries", rowsAffected)
	}
	
	// Remove baselines that haven't been updated recently
	result, err = sa.db.Exec("DELETE FROM log_frequency_baselines WHERE last_updated < ?", cutoffTime)
	if err != nil {
		return fmt.Errorf("failed to purge old baselines: %w", err)
	}
	
	if rowsAffected, err := result.RowsAffected(); err == nil && rowsAffected > 0 {
		log.Infof("Purged %d old baseline entries", rowsAffected)
	}
	
	return nil
}

// average calculates the average of a slice of integers
func average(nums []int) float64 {
	if len(nums) == 0 {
		return 0.0
	}
	
	sum := 0
	for _, num := range nums {
		sum += num
	}
	
	return float64(sum) / float64(len(nums))
}

// FlushAll clears all SQLite data
func (sa *StatisticalAnalyzer) FlushAll() error {
	_, err := sa.db.Exec("DELETE FROM pattern_counts; DELETE FROM baselines;")
	return err
}

// IsEmpty checks if baselines are empty
func (sa *StatisticalAnalyzer) IsEmpty() (bool, error) {
	var count int
	err := sa.db.QueryRow("SELECT COUNT(*) FROM baselines").Scan(&count)
	if err != nil {
		return true, err
	}
	return count == 0, nil
}

// GetDeadLetterSize returns dead letter queue size (not applicable for SQLite)
func (sa *StatisticalAnalyzer) GetDeadLetterSize() (int64, error) {
	return 0, nil
}
