package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

// LogEntry represents a single log entry from Loki
type LogEntry struct {
	Timestamp time.Time         `json:"timestamp"`
	Message   string            `json:"message"`
	Labels    map[string]string `json:"labels"`
	Host      string            `json:"host"`
	Service   string            `json:"service"`
	Level     string            `json:"level"`
}

// PatternMatch represents a successful pattern match
type PatternMatch struct {
	PatternName   string  `json:"pattern_name"`
	PatternRegex  string  `json:"pattern_regex"`
	AnomalyScore  float64 `json:"anomaly_score"`
	Description   string  `json:"description"`
	MatchedText   string  `json:"matched_text"`
}

// Anomaly represents a detected anomaly
type Anomaly struct {
	Timestamp         time.Time               `json:"timestamp"`
	LogEntry          LogEntry                `json:"log_entry"`
	AnomalyType       string                  `json:"anomaly_type"`
	Score             float64                 `json:"score"`
	PatternMatches    []PatternMatch          `json:"pattern_matches"`
	StatisticalScores map[string]float64      `json:"statistical_scores"`
	Description       string                  `json:"description"`
	Severity          string                  `json:"severity"`
}

// AnomalyDetector is the main anomaly detection engine
type AnomalyDetector struct {
	patternManager      *StructuredPatternManager
	statisticalAnalyzer StatisticalAnalyzerInterface
	config              Config
	recentLogs          []LogEntry
	recentLogsMutex     sync.RWMutex
	maxRecentLogs       int
	httpClient          *http.Client
}

// NewAnomalyDetector creates a new anomaly detector
func NewAnomalyDetector(config Config, patternManager *StructuredPatternManager, statisticalAnalyzer StatisticalAnalyzerInterface) *AnomalyDetector {
	return &AnomalyDetector{
		patternManager:      patternManager,
		statisticalAnalyzer: statisticalAnalyzer,
		config:              config,
		recentLogs:          make([]LogEntry, 0),
		maxRecentLogs:       10000,
		httpClient:          &http.Client{Timeout: 30 * time.Second},
	}
}

// Start starts the anomaly detection service
func (ad *AnomalyDetector) Start(ctx context.Context) {
	log.Info("Starting log anomaly detection service")

	// Start periodic analysis
	go ad.periodicAnalysis(ctx)

	// Start baseline calculation
	go ad.baselineCalculation(ctx)
}

// GetPatternsCount returns the number of loaded patterns
func (ad *AnomalyDetector) GetPatternsCount() int {
	return ad.patternManager.GetPatternsCount()
}

// GetRecentLogsCount returns the number of recent logs in cache
func (ad *AnomalyDetector) GetRecentLogsCount() int {
	ad.recentLogsMutex.RLock()
	defer ad.recentLogsMutex.RUnlock()
	return len(ad.recentLogs)
}

// GetPatterns returns pattern information
func (ad *AnomalyDetector) GetPatterns() map[string]interface{} {
	return ad.patternManager.GetPatterns()
}

// periodicAnalysis performs periodic log analysis
func (ad *AnomalyDetector) periodicAnalysis(ctx context.Context) {
	ticker := time.NewTicker(time.Duration(ad.config.CheckInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info("Stopping periodic analysis")
			return
		case <-ticker.C:
			if err := ad.analyzeRecentLogs(); err != nil {
				log.Errorf("Error in periodic analysis: %v", err)
			}
		}
	}
}

// baselineCalculation periodically updates statistical baselines
func (ad *AnomalyDetector) baselineCalculation(ctx context.Context) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info("Stopping baseline calculation")
			return
		case <-ticker.C:
			if err := ad.statisticalAnalyzer.CalculateBaselines(); err != nil {
				log.Errorf("Error updating baselines: %v", err)
			}
		}
	}
}

// analyzeRecentLogs fetches and analyzes recent logs from Loki
func (ad *AnomalyDetector) analyzeRecentLogs() error {
	startProcessing := time.Now()
	defer func() {
		processingDuration.WithLabelValues("analyze_logs").Observe(time.Since(startProcessing).Seconds())
	}()

	endTime := time.Now()
	startTime := endTime.Add(-time.Duration(ad.config.CheckInterval+60) * time.Second)

	logs, err := ad.queryLoki(startTime, endTime)
	if err != nil {
		return fmt.Errorf("failed to query Loki: %w", err)
	}

	log.Debugf("Processing %d log entries", len(logs))

	for _, logEntry := range logs {
		// Update metrics for processed logs
		logsProcessedTotal.WithLabelValues(logEntry.Host, logEntry.Service, logEntry.Level).Inc()
		
		// Add to recent logs cache
		ad.addRecentLog(logEntry)

		// Detect anomalies
		anomalies, err := ad.detectAnomalies(logEntry)
		if err != nil {
			log.Errorf("Error detecting anomalies: %v", err)
			continue
		}

		// Send alerts for detected anomalies
		for _, anomaly := range anomalies {
			// Update anomaly metrics
			anomaliesDetectedTotal.WithLabelValues(anomaly.AnomalyType, anomaly.Severity, anomaly.LogEntry.Host, anomaly.LogEntry.Service).Inc()
			anomalyScore.WithLabelValues(anomaly.AnomalyType, anomaly.LogEntry.Host, anomaly.LogEntry.Service).Observe(anomaly.Score)
			
			if err := ad.sendAlert(anomaly); err != nil {
				log.Errorf("Error sending alert: %v", err)
			}
		}
	}

	// Update recent logs count metric
	logsRecentCount.Set(float64(ad.GetRecentLogsCount()))
	
	return nil
}

// queryLoki queries Loki for log entries
func (ad *AnomalyDetector) queryLoki(startTime, endTime time.Time) ([]LogEntry, error) {
	startQuery := time.Now()
	defer func() {
		lokiQueryDuration.Observe(time.Since(startQuery).Seconds())
	}()

	url := fmt.Sprintf("%s/loki/api/v1/query_range", ad.config.LokiURL)
	
	// Build query parameters
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		lokiQueriesTotal.WithLabelValues("error").Inc()
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	
	q := req.URL.Query()
	q.Add("query", `{job=~"systemd-journal|syslog"}`)
	q.Add("start", strconv.FormatInt(startTime.UnixNano(), 10))
	q.Add("end", strconv.FormatInt(endTime.UnixNano(), 10))
	q.Add("limit", strconv.Itoa(ad.config.BatchSize))
	req.URL.RawQuery = q.Encode()
	
	// Make request
	resp, err := ad.httpClient.Do(req)
	if err != nil {
		lokiQueriesTotal.WithLabelValues("error").Inc()
		return nil, fmt.Errorf("failed to query Loki: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		lokiQueriesTotal.WithLabelValues("error").Inc()
		return nil, fmt.Errorf("Loki returned status %d", resp.StatusCode)
	}
	
	// Parse response
	var lokiResponse struct {
		Status string `json:"status"`
		Data   struct {
			Result []struct {
				Stream map[string]string `json:"stream"`
				Values [][]string        `json:"values"`
			} `json:"result"`
		} `json:"data"`
	}
	
	if err := json.NewDecoder(resp.Body).Decode(&lokiResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	
	if lokiResponse.Status != "success" {
		return nil, fmt.Errorf("Loki query failed with status: %s", lokiResponse.Status)
	}
	
	// Convert to LogEntry structs
	var logEntries []LogEntry
	for _, stream := range lokiResponse.Data.Result {
		labels := stream.Stream
		
		for _, entry := range stream.Values {
			if len(entry) != 2 {
				continue
			}
			
			timestampNs, err := strconv.ParseInt(entry[0], 10, 64)
			if err != nil {
				log.Warnf("Invalid timestamp: %s", entry[0])
				continue
			}
			
			timestamp := time.Unix(0, timestampNs)
			message := entry[1]
			
			logEntry := LogEntry{
				Timestamp: timestamp,
				Message:   message,
				Labels:    labels,
				Host:      getLabel(labels, "hostname", "unknown"),
				Service:   getLabel(labels, "unit", getLabel(labels, "container", "unknown")),
				Level:     getLabel(labels, "level", "info"),
			}
			
			logEntries = append(logEntries, logEntry)
		}
	}
	
	lokiQueriesTotal.WithLabelValues("success").Inc()
	return logEntries, nil
}

// getLabel gets a label value with fallback
func getLabel(labels map[string]string, key, fallback string) string {
	if value, exists := labels[key]; exists {
		return value
	}
	return fallback
}

// addRecentLog adds a log entry to the recent logs cache
func (ad *AnomalyDetector) addRecentLog(logEntry LogEntry) {
	ad.recentLogsMutex.Lock()
	defer ad.recentLogsMutex.Unlock()

	ad.recentLogs = append(ad.recentLogs, logEntry)
	
	// Keep only the most recent logs
	if len(ad.recentLogs) > ad.maxRecentLogs {
		ad.recentLogs = ad.recentLogs[len(ad.recentLogs)-ad.maxRecentLogs:]
	}
}

// detectAnomalies detects anomalies in a single log entry
func (ad *AnomalyDetector) detectAnomalies(logEntry LogEntry) ([]Anomaly, error) {
	var anomalies []Anomaly

	// Pattern-based detection
	patternMatches := ad.patternManager.MatchPatterns(logEntry.Message, logEntry.Labels)
	
	for _, match := range patternMatches {
		statisticalScores := make(map[string]float64)
		
		// Update pattern counts
		if err := ad.statisticalAnalyzer.UpdatePatternCount(match.PatternName, logEntry.Host, logEntry.Service); err != nil {
			log.Errorf("Failed to update pattern count: %v", err)
		}
		
		// Check for frequency anomalies
		isFreqAnomaly, zScore := ad.statisticalAnalyzer.DetectFrequencyAnomalies(
			match.PatternName, logEntry.Host, logEntry.Service, 1, ad.config.FrequencySigma)
		
		if isFreqAnomaly {
			statisticalScores["frequency_z_score"] = zScore
		}
		
		// Check for rate change anomalies
		isRateAnomaly, rateChange := ad.statisticalAnalyzer.DetectRateChangeAnomalies(
			match.PatternName, logEntry.Host, logEntry.Service, ad.config.RateChangeThreshold)
		
		if isRateAnomaly {
			statisticalScores["rate_change"] = rateChange
		}
		
		// Create anomaly if score is high enough or statistical anomaly detected
		if match.AnomalyScore >= 2.0 || isFreqAnomaly || isRateAnomaly {
			totalScore := match.AnomalyScore
			if isFreqAnomaly {
				totalScore += math.Min(zScore, 5.0) // Cap contribution
			}
			if isRateAnomaly && !math.IsInf(rateChange, 1) {
				if rateChange > 1 {
					totalScore += math.Min(math.Log(rateChange), 3.0)
				}
			}
			
			severity := calculateSeverity(totalScore)
			
			anomaly := Anomaly{
				Timestamp:         logEntry.Timestamp,
				LogEntry:          logEntry,
				AnomalyType:       "pattern_based",
				Score:             totalScore,
				PatternMatches:    []PatternMatch{match},
				StatisticalScores: statisticalScores,
				Description:       fmt.Sprintf("Pattern %s: %s", match.PatternName, match.Description),
				Severity:          severity,
			}
			
			anomalies = append(anomalies, anomaly)
		}
	}
	
	// Rule-less detection
	rulelessAnomalies := ad.detectRulelessAnomalies(logEntry)
	anomalies = append(anomalies, rulelessAnomalies...)
	
	return anomalies, nil
}

// detectRulelessAnomalies detects anomalies without predefined patterns
func (ad *AnomalyDetector) detectRulelessAnomalies(logEntry LogEntry) []Anomaly {
	var anomalies []Anomaly
	
	ad.recentLogsMutex.RLock()
	recentLogs := make([]LogEntry, len(ad.recentLogs))
	copy(recentLogs, ad.recentLogs)
	ad.recentLogsMutex.RUnlock()
	
	// Entropy analysis
	if len(recentLogs) > 0 {
		// Get recent messages for entropy comparison
		recentMessages := make([]string, 0, 100)
		for i := len(recentLogs) - 100; i < len(recentLogs) && i >= 0; i++ {
			if i >= 0 {
				recentMessages = append(recentMessages, recentLogs[i].Message)
			}
		}
		
		if len(recentMessages) > 0 {
			entropy := calculateEntropy([]string{logEntry.Message})
			avgEntropy := calculateEntropy(recentMessages) / float64(len(recentMessages))
			
			if entropy > ad.config.EntropyThreshold && entropy > avgEntropy*2 {
				score := math.Min(entropy, 10.0)
				anomaly := Anomaly{
					Timestamp:   logEntry.Timestamp,
					LogEntry:    logEntry,
					AnomalyType: "entropy_based",
					Score:       score,
					PatternMatches: []PatternMatch{},
					StatisticalScores: map[string]float64{
						"entropy":     entropy,
						"avg_entropy": avgEntropy,
					},
					Description: fmt.Sprintf("High entropy log message (entropy=%.2f)", entropy),
					Severity:    calculateSeverity(score),
				}
				anomalies = append(anomalies, anomaly)
			}
		}
	}
	
	// Levenshtein distance analysis (find very similar repeated messages)
	if len(recentLogs) > 10 {
		similarCount := 0
		checkCount := 50
		if len(recentLogs) < checkCount {
			checkCount = len(recentLogs)
		}
		
		for i := len(recentLogs) - checkCount; i < len(recentLogs); i++ {
			if i >= 0 && recentLogs[i].Host == logEntry.Host && recentLogs[i].Service == logEntry.Service {
				distance := levenshteinDistance(logEntry.Message, recentLogs[i].Message)
				maxLen := len(logEntry.Message)
				if len(recentLogs[i].Message) > maxLen {
					maxLen = len(recentLogs[i].Message)
				}
				
				if maxLen > 0 {
					similarity := 1.0 - (float64(distance) / float64(maxLen))
					if similarity > ad.config.LevenshteinThreshold {
						similarCount++
					}
				}
			}
		}
		
		if similarCount >= 3 { // Found 3+ very similar messages recently
			score := math.Min(float64(similarCount), 8.0)
			anomaly := Anomaly{
				Timestamp:   logEntry.Timestamp,
				LogEntry:    logEntry,
				AnomalyType: "repetition_based",
				Score:       score,
				PatternMatches: []PatternMatch{},
				StatisticalScores: map[string]float64{
					"similar_message_count": float64(similarCount),
				},
				Description: fmt.Sprintf("Repeated similar messages detected (%d similar)", similarCount),
				Severity:    calculateSeverity(score),
			}
			anomalies = append(anomalies, anomaly)
		}
	}
	
	return anomalies
}

// calculateSeverity calculates anomaly severity based on score
func calculateSeverity(score float64) string {
	if score >= 5.0 {
		return "critical"
	} else if score >= 3.5 {
		return "high"
	} else if score >= 2.0 {
		return "medium"
	}
	return "low"
}

// sendAlert sends alert for detected anomaly
func (ad *AnomalyDetector) sendAlert(anomaly Anomaly) error {
	startWebhook := time.Now()
	defer func() {
		webhookDuration.Observe(time.Since(startWebhook).Seconds())
	}()

	if ad.config.WebhookURL == "" {
		log.Warning("No webhook URL configured, skipping alert")
		return nil
	}

	alertData := map[string]interface{}{
		"timestamp":          anomaly.Timestamp.Format(time.RFC3339),
		"host":               anomaly.LogEntry.Host,
		"service":            anomaly.LogEntry.Service,
		"severity":           anomaly.Severity,
		"anomaly_type":       anomaly.AnomalyType,
		"score":              anomaly.Score,
		"description":        anomaly.Description,
		"log_message":        anomaly.LogEntry.Message,
		"pattern_matches":    anomaly.PatternMatches,
		"statistical_scores": anomaly.StatisticalScores,
	}

	jsonData, err := json.Marshal(alertData)
	if err != nil {
		webhooksSentTotal.WithLabelValues("error", anomaly.Severity).Inc()
		return fmt.Errorf("failed to marshal alert data: %w", err)
	}

	resp, err := ad.httpClient.Post(ad.config.WebhookURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		webhooksSentTotal.WithLabelValues("error", anomaly.Severity).Inc()
		return fmt.Errorf("failed to send alert: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		webhooksSentTotal.WithLabelValues("error", anomaly.Severity).Inc()
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	webhooksSentTotal.WithLabelValues("success", anomaly.Severity).Inc()
	log.Infof("Sent anomaly alert: severity=%s, host=%s", anomaly.Severity, anomaly.LogEntry.Host)
	return nil
}

// calculateEntropy calculates entropy of log messages
func calculateEntropy(messages []string) float64 {
	if len(messages) == 0 {
		return 0.0
	}

	// Tokenize and count word frequencies
	wordCounts := make(map[string]int)
	totalWords := 0

	wordRegex := regexp.MustCompile(`\w+`)
	for _, message := range messages {
		words := wordRegex.FindAllString(strings.ToLower(message), -1)
		for _, word := range words {
			wordCounts[word]++
			totalWords++
		}
	}

	if totalWords == 0 {
		return 0.0
	}

	// Calculate entropy
	entropy := 0.0
	for _, count := range wordCounts {
		probability := float64(count) / float64(totalWords)
		if probability > 0 {
			entropy -= probability * math.Log2(probability)
		}
	}

	return entropy
}

// levenshteinDistance calculates the Levenshtein distance between two strings
func levenshteinDistance(s1, s2 string) int {
	if len(s1) == 0 {
		return len(s2)
	}
	if len(s2) == 0 {
		return len(s1)
	}

	// Create a matrix
	matrix := make([][]int, len(s1)+1)
	for i := range matrix {
		matrix[i] = make([]int, len(s2)+1)
	}

	// Initialize first row and column
	for i := 0; i <= len(s1); i++ {
		matrix[i][0] = i
	}
	for j := 0; j <= len(s2); j++ {
		matrix[0][j] = j
	}

	// Fill the matrix
	for i := 1; i <= len(s1); i++ {
		for j := 1; j <= len(s2); j++ {
			cost := 0
			if s1[i-1] != s2[j-1] {
				cost = 1
			}

			matrix[i][j] = min3(
				matrix[i-1][j]+1,      // deletion
				matrix[i][j-1]+1,      // insertion
				matrix[i-1][j-1]+cost, // substitution
			)
		}
	}

	return matrix[len(s1)][len(s2)]
}

// min3 returns the minimum of three integers
func min3(a, b, c int) int {
	if a < b && a < c {
		return a
	}
	if b < c {
		return b
	}
	return c
}
