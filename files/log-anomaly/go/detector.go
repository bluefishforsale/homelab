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

// SourceStats tracks statistics for a discovered log source
type SourceStats struct {
	FirstSeen    time.Time `json:"first_seen"`
	LastSeen     time.Time `json:"last_seen"`
	LogCount     int64     `json:"log_count"`
	AnomalyCount int64     `json:"anomaly_count"`
}

// DiscoveredSources tracks all discovered hosts and services
type DiscoveredSources struct {
	Hosts    map[string]*SourceStats            `json:"hosts"`
	Services map[string]map[string]*SourceStats `json:"services"` // host -> service -> stats
	mutex    sync.RWMutex
}

// NewDiscoveredSources creates a new discovered sources tracker
func NewDiscoveredSources() *DiscoveredSources {
	return &DiscoveredSources{
		Hosts:    make(map[string]*SourceStats),
		Services: make(map[string]map[string]*SourceStats),
	}
}

// RecordLog records a log entry from a host/service
func (ds *DiscoveredSources) RecordLog(host, service string) {
	ds.mutex.Lock()
	defer ds.mutex.Unlock()
	
	now := time.Now()
	
	// Track host
	if _, exists := ds.Hosts[host]; !exists {
		ds.Hosts[host] = &SourceStats{FirstSeen: now}
	}
	ds.Hosts[host].LastSeen = now
	ds.Hosts[host].LogCount++
	
	// Track service under host
	if _, exists := ds.Services[host]; !exists {
		ds.Services[host] = make(map[string]*SourceStats)
	}
	if _, exists := ds.Services[host][service]; !exists {
		ds.Services[host][service] = &SourceStats{FirstSeen: now}
	}
	ds.Services[host][service].LastSeen = now
	ds.Services[host][service].LogCount++
}

// RecordAnomaly records an anomaly for a host/service
func (ds *DiscoveredSources) RecordAnomaly(host, service string) {
	ds.mutex.Lock()
	defer ds.mutex.Unlock()
	
	if stats, exists := ds.Hosts[host]; exists {
		stats.AnomalyCount++
	}
	if hostServices, exists := ds.Services[host]; exists {
		if stats, exists := hostServices[service]; exists {
			stats.AnomalyCount++
		}
	}
}

// GetSummary returns a summary of discovered sources
func (ds *DiscoveredSources) GetSummary() map[string]interface{} {
	ds.mutex.RLock()
	defer ds.mutex.RUnlock()
	
	hostList := make([]string, 0, len(ds.Hosts))
	for host := range ds.Hosts {
		hostList = append(hostList, host)
	}
	
	serviceList := make([]string, 0)
	serviceSeen := make(map[string]bool)
	for _, services := range ds.Services {
		for service := range services {
			if !serviceSeen[service] {
				serviceList = append(serviceList, service)
				serviceSeen[service] = true
			}
		}
	}
	
	return map[string]interface{}{
		"hosts":          hostList,
		"services":       serviceList,
		"host_count":     len(ds.Hosts),
		"service_count":  len(serviceSeen),
		"host_details":   ds.Hosts,
		"service_details": ds.Services,
	}
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
	// Alert deduplication
	alertCooldowns      map[string]time.Time
	alertCooldownsMutex sync.RWMutex
	// Auto-discovery
	discoveredSources   *DiscoveredSources
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
		alertCooldowns:      make(map[string]time.Time),
		discoveredSources:   NewDiscoveredSources(),
	}
}

// GetDiscoveredSources returns the discovered sources summary
func (ad *AnomalyDetector) GetDiscoveredSources() map[string]interface{} {
	return ad.discoveredSources.GetSummary()
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
		// Auto-discover hosts and services from logs
		ad.discoveredSources.RecordLog(logEntry.Host, logEntry.Service)
		
		// Check if service is suppressed
		if ad.isServiceSuppressed(logEntry.Service) {
			continue
		}

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

		// Send alerts for detected anomalies (with filtering)
		for _, anomaly := range anomalies {
			// Record anomaly against discovered source
			ad.discoveredSources.RecordAnomaly(anomaly.LogEntry.Host, anomaly.LogEntry.Service)
			
			// Update anomaly metrics
			anomaliesDetectedTotal.WithLabelValues(anomaly.AnomalyType, anomaly.Severity, anomaly.LogEntry.Host, anomaly.LogEntry.Service).Inc()
			anomalyScore.WithLabelValues(anomaly.AnomalyType, anomaly.LogEntry.Host, anomaly.LogEntry.Service).Observe(anomaly.Score)
			
			// Filter by minimum severity
			if !ad.meetsMinSeverity(anomaly.Severity) {
				log.Debugf("Skipping alert: severity %s below minimum %s", anomaly.Severity, ad.config.MinSeverity)
				continue
			}
			
			// Check deduplication cooldown
			alertKey := ad.getAlertKey(anomaly)
			if ad.isAlertInCooldown(alertKey) {
				log.Debugf("Skipping alert: in cooldown for key %s", alertKey)
				continue
			}
			
			if err := ad.sendAlert(anomaly); err != nil {
				log.Errorf("Error sending alert: %v", err)
			} else {
				// Mark alert as sent
				ad.markAlertSent(alertKey)
			}
		}
	}

	// Update recent logs count metric
	logsRecentCount.Set(float64(ad.GetRecentLogsCount()))
	
	// Cleanup expired cooldowns periodically
	ad.cleanupExpiredCooldowns()
	
	return nil
}

// isServiceSuppressed checks if a service is in the suppression list
func (ad *AnomalyDetector) isServiceSuppressed(service string) bool {
	serviceLower := strings.ToLower(service)
	for _, suppressed := range ad.config.SuppressedServices {
		if strings.Contains(serviceLower, strings.ToLower(suppressed)) {
			return true
		}
	}
	return false
}

// meetsMinSeverity checks if anomaly severity meets minimum threshold
func (ad *AnomalyDetector) meetsMinSeverity(severity string) bool {
	severityOrder := map[string]int{"low": 1, "medium": 2, "high": 3, "critical": 4}
	minLevel := severityOrder[ad.config.MinSeverity]
	anomalyLevel := severityOrder[severity]
	return anomalyLevel >= minLevel
}

// getAlertKey generates a unique key for deduplication
func (ad *AnomalyDetector) getAlertKey(anomaly Anomaly) string {
	return fmt.Sprintf("%s:%s:%s:%s", anomaly.LogEntry.Host, anomaly.LogEntry.Service, anomaly.AnomalyType, anomaly.Description)
}

// isAlertInCooldown checks if an alert was recently sent
func (ad *AnomalyDetector) isAlertInCooldown(key string) bool {
	ad.alertCooldownsMutex.RLock()
	defer ad.alertCooldownsMutex.RUnlock()
	
	if lastSent, exists := ad.alertCooldowns[key]; exists {
		cooldownDuration := time.Duration(ad.config.AlertCooldownMinutes) * time.Minute
		return time.Since(lastSent) < cooldownDuration
	}
	return false
}

// markAlertSent records when an alert was sent
func (ad *AnomalyDetector) markAlertSent(key string) {
	ad.alertCooldownsMutex.Lock()
	defer ad.alertCooldownsMutex.Unlock()
	ad.alertCooldowns[key] = time.Now()
}

// cleanupExpiredCooldowns removes old entries from cooldown map
func (ad *AnomalyDetector) cleanupExpiredCooldowns() {
	ad.alertCooldownsMutex.Lock()
	defer ad.alertCooldownsMutex.Unlock()
	
	cooldownDuration := time.Duration(ad.config.AlertCooldownMinutes) * time.Minute
	now := time.Now()
	
	for key, lastSent := range ad.alertCooldowns {
		if now.Sub(lastSent) > cooldownDuration*2 {
			delete(ad.alertCooldowns, key)
		}
	}
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
		if match.AnomalyScore >= ad.config.MinPatternScore || isFreqAnomaly || isRateAnomaly {
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
		
		if similarCount >= ad.config.MinRepetitionCount { // Found many similar messages recently
			score := math.Min(float64(similarCount)/2.0, 8.0) // Scale score based on count
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
// Raised thresholds to reduce noise:
// - critical: >= 8.0 (was 5.0)
// - high: >= 5.0 (was 3.5)
// - medium: >= 3.0 (was 2.0)
func calculateSeverity(score float64) string {
	if score >= 8.0 {
		return "critical"
	} else if score >= 5.0 {
		return "high"
	} else if score >= 3.0 {
		return "medium"
	}
	return "low"
}

// ActionableAlert contains structured, actionable alert information
type ActionableAlert struct {
	// Identity
	Host      string `json:"host"`
	Service   string `json:"service"`
	Severity  string `json:"severity"`
	Timestamp string `json:"timestamp"`
	
	// Category for grouping
	Category    string `json:"category"` // security, database, container, resource, network, application, system
	
	// Problem Summary
	Title       string `json:"title"`
	Summary     string `json:"summary"`
	Impact      string `json:"impact"`
	
	// Remediation
	Actions      []string `json:"actions"`
	DashboardURL string   `json:"dashboard_url"`
	
	// Technical Details
	AnomalyType       string             `json:"anomaly_type"`
	Score             float64            `json:"score"`
	LogSample         string             `json:"log_sample"`
	PatternMatches    []PatternMatch     `json:"pattern_matches,omitempty"`
	StatisticalScores map[string]float64 `json:"statistical_scores,omitempty"`
}

// generateActionableAlert creates a structured, actionable alert from an anomaly
func (ad *AnomalyDetector) generateActionableAlert(anomaly Anomaly) ActionableAlert {
	alert := ActionableAlert{
		Host:              anomaly.LogEntry.Host,
		Service:           anomaly.LogEntry.Service,
		Severity:          anomaly.Severity,
		Timestamp:         anomaly.Timestamp.Format(time.RFC3339),
		AnomalyType:       anomaly.AnomalyType,
		Score:             anomaly.Score,
		LogSample:         truncateString(anomaly.LogEntry.Message, 500),
		PatternMatches:    anomaly.PatternMatches,
		StatisticalScores: anomaly.StatisticalScores,
		DashboardURL:      ad.config.DashboardURL,
	}
	
	// Generate actionable content based on anomaly type and patterns
	switch anomaly.AnomalyType {
	case "pattern_based":
		alert.Category, alert.Title, alert.Summary, alert.Impact, alert.Actions = ad.getPatternBasedActions(anomaly)
	case "entropy_based":
		alert.Category = "application"
		alert.Title = fmt.Sprintf("Unusual Log Pattern on %s", anomaly.LogEntry.Host)
		alert.Summary = fmt.Sprintf("Service '%s' is generating logs with unusual content patterns (entropy=%.2f). This may indicate corrupted output, binary data in logs, or application malfunction.",
			anomaly.LogEntry.Service, anomaly.StatisticalScores["entropy"])
		alert.Impact = "Log analysis may be impaired; possible application instability"
		alert.Actions = []string{
			fmt.Sprintf("Check %s service status: systemctl status %s", anomaly.LogEntry.Service, anomaly.LogEntry.Service),
			fmt.Sprintf("Review recent logs: journalctl -u %s -n 100 --no-pager", anomaly.LogEntry.Service),
			"Look for application crashes, encoding issues, or unexpected binary output",
			"Restart service if logs appear corrupted: systemctl restart " + anomaly.LogEntry.Service,
		}
	case "repetition_based":
		alert.Category = "application"
		count := int(anomaly.StatisticalScores["similar_message_count"])
		alert.Title = fmt.Sprintf("Log Spam Detected on %s", anomaly.LogEntry.Host)
		alert.Summary = fmt.Sprintf("Service '%s' is repeating the same log message %d times. This typically indicates a retry loop, stuck process, or persistent error condition.",
			anomaly.LogEntry.Service, count)
		alert.Impact = "Potential disk fill from log spam; underlying issue may need attention"
		alert.Actions = []string{
			fmt.Sprintf("Check service status: systemctl status %s", anomaly.LogEntry.Service),
			"Identify the repeating error and address root cause",
			fmt.Sprintf("Check for stuck processes: ps aux | grep %s", anomaly.LogEntry.Service),
			"Consider restarting if stuck: systemctl restart " + anomaly.LogEntry.Service,
		}
	default:
		alert.Category = "system"
		alert.Title = fmt.Sprintf("Anomaly Detected on %s/%s", anomaly.LogEntry.Host, anomaly.LogEntry.Service)
		alert.Summary = anomaly.Description
		alert.Impact = "Unknown - review logs for context"
		alert.Actions = []string{
			fmt.Sprintf("Review service logs: journalctl -u %s -n 50", anomaly.LogEntry.Service),
			"Check service health and dependencies",
		}
	}
	
	return alert
}

// getPatternBasedActions returns actionable content for pattern-based anomalies
func (ad *AnomalyDetector) getPatternBasedActions(anomaly Anomaly) (category, title, summary, impact string, actions []string) {
	if len(anomaly.PatternMatches) == 0 {
		return "system", "Unknown Pattern Match", anomaly.Description, "Unknown", []string{"Review logs manually"}
	}
	
	pattern := anomaly.PatternMatches[0]
	host := anomaly.LogEntry.Host
	service := anomaly.LogEntry.Service
	
	// Map pattern names to actionable responses
	switch {
	// Database issues
	case strings.HasPrefix(pattern.PatternName, "DB_") || strings.Contains(pattern.PatternName, "DATABASE"):
		category = "database"
		title = fmt.Sprintf("Database Issue on %s", host)
		switch pattern.PatternName {
		case "DB_CONNECTION_POOL":
			summary = fmt.Sprintf("Database connection pool exhaustion detected for '%s'. Connections may be leaking or load exceeds pool capacity.", service)
			impact = "Service may become unresponsive; new requests will fail"
			actions = []string{
				"Check active DB connections: SELECT count(*) FROM pg_stat_activity;",
				fmt.Sprintf("Restart service to reset connections: systemctl restart %s", service),
				"Review connection pool settings and increase if needed",
				"Check for connection leaks in application code",
			}
		case "DB_ERROR":
			summary = fmt.Sprintf("Database error detected in '%s'. Check database connectivity and query logs.", service)
			impact = "Data operations may be failing"
			actions = []string{
				"Check database status: systemctl status mysql/postgresql",
				"Review database logs for errors",
				"Test connectivity: mysql -e 'SELECT 1' or psql -c 'SELECT 1'",
				"Check disk space: df -h",
			}
		default:
			summary = fmt.Sprintf("Database-related issue in '%s': %s", service, pattern.Description)
			impact = "Database operations may be affected"
			actions = []string{
				"Check database service status",
				"Review database and application logs",
			}
		}
		
	// Authentication/Security issues
	case strings.HasPrefix(pattern.PatternName, "AUTH_") || strings.HasPrefix(pattern.PatternName, "SECURITY_"):
		category = "security"
		title = fmt.Sprintf("Security Event on %s", host)
		switch pattern.PatternName {
		case "AUTH_LOCKOUT":
			summary = fmt.Sprintf("Account lockout detected on '%s'. Possible brute force attack or user lockout.", service)
			impact = "User access blocked; potential security incident"
			actions = []string{
				"Review auth logs: grep -i 'lock\\|fail' /var/log/auth.log | tail -50",
				"Check source IPs of failed attempts",
				"Consider blocking suspicious IPs: fail2ban-client status",
				"Unlock account if legitimate: passwd -u <username>",
			}
		case "AUTH_FAIL":
			summary = fmt.Sprintf("Authentication failures detected on '%s'. Monitor for brute force patterns.", service)
			impact = "Potential unauthorized access attempt"
			actions = []string{
				"Review failed login attempts: lastb | head -20",
				"Check if fail2ban is active: fail2ban-client status sshd",
				"Verify legitimate users aren't locked out",
			}
		default:
			summary = fmt.Sprintf("Security event in '%s': %s", service, pattern.Description)
			impact = "Security posture may be affected"
			actions = []string{"Review security logs immediately", "Check for unauthorized access"}
		}
		
	// Container/Docker issues
	case strings.HasPrefix(pattern.PatternName, "CONTAINER_") || strings.HasPrefix(pattern.PatternName, "DOCKER_"):
		category = "container"
		title = fmt.Sprintf("Container Issue on %s", host)
		summary = fmt.Sprintf("Docker/container issue detected: %s", pattern.Description)
		impact = "Container workloads may be affected"
		actions = []string{
			"Check container status: docker ps -a",
			fmt.Sprintf("View container logs: docker logs %s --tail 50", service),
			"Check Docker daemon: systemctl status docker",
			"Review resource usage: docker stats --no-stream",
		}
		
	// Resource exhaustion
	case strings.Contains(pattern.PatternName, "MEMORY") || strings.Contains(pattern.PatternName, "CPU") || strings.Contains(pattern.PatternName, "DISK"):
		category = "resource"
		title = fmt.Sprintf("Resource Exhaustion on %s", host)
		summary = fmt.Sprintf("Resource issue detected on '%s': %s", service, pattern.Description)
		impact = "Service performance degraded; potential outage"
		actions = []string{
			"Check system resources: htop or top",
			"Check memory: free -h",
			"Check disk: df -h",
			fmt.Sprintf("Review service resource usage: systemctl status %s", service),
			"Consider scaling or restarting heavy services",
		}
		
	// Network issues
	case strings.HasPrefix(pattern.PatternName, "NET_") || strings.Contains(pattern.PatternName, "NETWORK") || strings.Contains(pattern.PatternName, "TIMEOUT"):
		category = "network"
		title = fmt.Sprintf("Network Issue on %s", host)
		summary = fmt.Sprintf("Network connectivity issue detected for '%s': %s", service, pattern.Description)
		impact = "Network-dependent operations may fail"
		actions = []string{
			"Check network connectivity: ping -c 3 8.8.8.8",
			"Check DNS resolution: nslookup google.com",
			"Review firewall rules: iptables -L -n",
			"Check listening ports: ss -tlnp",
		}
		
	// Service errors (generic)
	case strings.HasPrefix(pattern.PatternName, "APP_") || strings.Contains(pattern.PatternName, "ERROR"):
		category = "application"
		title = fmt.Sprintf("Application Error on %s", host)
		summary = fmt.Sprintf("Application error in '%s': %s", service, pattern.Description)
		impact = "Application functionality may be impaired"
		actions = []string{
			fmt.Sprintf("Check service status: systemctl status %s", service),
			fmt.Sprintf("View recent logs: journalctl -u %s -n 100 --no-pager", service),
			"Check application health endpoint if available",
			fmt.Sprintf("Restart if needed: systemctl restart %s", service),
		}
		
	default:
		category = "system"
		title = fmt.Sprintf("Issue Detected on %s/%s", host, service)
		summary = fmt.Sprintf("Pattern '%s' detected: %s", pattern.PatternName, pattern.Description)
		impact = "Review required to assess impact"
		actions = []string{
			fmt.Sprintf("Review service logs: journalctl -u %s -n 50", service),
			"Check service health and dependencies",
		}
	}
	
	return category, title, summary, impact, actions
}

// truncateString truncates a string to maxLen characters
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// sendAlert sends alert for detected anomaly with retry and dead letter queue
func (ad *AnomalyDetector) sendAlert(anomaly Anomaly) error {
	startWebhook := time.Now()
	defer func() {
		webhookDuration.Observe(time.Since(startWebhook).Seconds())
	}()

	if ad.config.WebhookURL == "" {
		log.Warning("No webhook URL configured, skipping alert")
		return nil
	}

	// Generate actionable alert
	actionableAlert := ad.generateActionableAlert(anomaly)
	
	alertData := map[string]interface{}{
		// Actionable fields (primary)
		"host":          actionableAlert.Host,
		"service":       actionableAlert.Service,
		"severity":      actionableAlert.Severity,
		"timestamp":     actionableAlert.Timestamp,
		"category":      actionableAlert.Category, // security, database, container, resource, network, application, system
		"title":         actionableAlert.Title,
		"summary":       actionableAlert.Summary,
		"impact":        actionableAlert.Impact,
		"actions":       actionableAlert.Actions,
		"dashboard_url": actionableAlert.DashboardURL,
		
		// Technical details
		"anomaly_type":       actionableAlert.AnomalyType,
		"score":              actionableAlert.Score,
		"log_sample":         actionableAlert.LogSample,
		"pattern_matches":    actionableAlert.PatternMatches,
		"statistical_scores": actionableAlert.StatisticalScores,
	}

	jsonData, err := json.Marshal(alertData)
	if err != nil {
		webhooksSentTotal.WithLabelValues("error", anomaly.Severity).Inc()
		return fmt.Errorf("failed to marshal alert data: %w", err)
	}

	// Retry logic: 3 attempts with exponential backoff (1s, 2s)
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			webhookRetriesTotal.WithLabelValues(strconv.Itoa(attempt)).Inc()
			backoff := time.Duration(1<<(attempt-1)) * time.Second // 1s, 2s
			log.Warnf("Webhook retry attempt %d after %v", attempt+1, backoff)
			time.Sleep(backoff)
		}

		resp, err := ad.httpClient.Post(ad.config.WebhookURL, "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			lastErr = fmt.Errorf("attempt %d: failed to send alert: %w", attempt+1, err)
			continue
		}
		resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			webhooksSentTotal.WithLabelValues("success", anomaly.Severity).Inc()
			log.Infof("Sent anomaly alert: severity=%s, host=%s", anomaly.Severity, anomaly.LogEntry.Host)
			return nil
		}
		lastErr = fmt.Errorf("attempt %d: webhook returned status %d", attempt+1, resp.StatusCode)
	}

	// All retries failed - push to dead letter queue
	webhooksSentTotal.WithLabelValues("dead_letter", anomaly.Severity).Inc()
	log.Errorf("Webhook failed after 3 attempts, pushing to dead letter queue: %v", lastErr)

	if err := ad.pushToDeadLetter(jsonData); err != nil {
		log.Errorf("Failed to push to dead letter queue: %v", err)
		webhooksSentTotal.WithLabelValues("error", anomaly.Severity).Inc()
		return fmt.Errorf("webhook failed and dead letter failed: %w", err)
	}

	deadLetterTotal.Inc()
	return lastErr
}

// pushToDeadLetter pushes failed anomaly to Redis dead letter queue
func (ad *AnomalyDetector) pushToDeadLetter(jsonData []byte) error {
	if rsa, ok := ad.statisticalAnalyzer.(*RedisStatisticalAnalyzer); ok {
		return rsa.PushDeadLetter(jsonData)
	}
	return fmt.Errorf("statistical analyzer does not support dead letter queue")
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
