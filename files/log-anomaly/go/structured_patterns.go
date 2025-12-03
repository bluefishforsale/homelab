package main

import (
	"fmt"
	"regexp"
	"strings"
	"sync"

	log "github.com/sirupsen/logrus"
)

// StructuredPattern represents a pattern that can match against structured fields
type StructuredPattern struct {
	Name        string
	FieldRules  map[string]*regexp.Regexp // field_name -> regex
	MessageRegex *regexp.Regexp           // fallback for message field
	Score       float64
	Description string
	RequiredFields []string               // fields that must be present
}

// StructuredPatternManager manages structured log pattern matching
type StructuredPatternManager struct {
	structuredPatterns map[string][]StructuredPattern
	fallbackPatterns   map[string][]Pattern // original regex patterns for non-JSON logs
	jsonParser         *LogParser
	mutex              sync.RWMutex
}

// NewStructuredPatternManager creates a new structured pattern manager
func NewStructuredPatternManager(patternsDir string) (*StructuredPatternManager, error) {
	spm := &StructuredPatternManager{
		structuredPatterns: make(map[string][]StructuredPattern),
		fallbackPatterns:   make(map[string][]Pattern),
		jsonParser:         NewLogParser(),
	}

	if err := spm.loadPatterns(patternsDir); err != nil {
		return nil, fmt.Errorf("failed to load patterns: %w", err)
	}

	return spm, nil
}

// loadPatterns loads both structured and fallback patterns
func (spm *StructuredPatternManager) loadPatterns(patternsDir string) error {
	spm.mutex.Lock()
	defer spm.mutex.Unlock()

	// Load structured patterns from YAML/JSON files
	if err := spm.loadStructuredPatterns(patternsDir); err != nil {
		log.Warnf("Failed to load structured patterns: %v", err)
	}

	// Load fallback regex patterns
	pm := &PatternManager{patterns: make(map[string][]Pattern)}
	if err := pm.loadPatterns(patternsDir); err != nil {
		return fmt.Errorf("failed to load fallback patterns: %w", err)
	}

	spm.fallbackPatterns = pm.patterns

	// Update Prometheus metrics for loaded patterns
	spm.updatePatternsLoadedMetrics()
	
	return nil
}

// loadStructuredPatterns loads structured patterns (this could be extended to read from YAML)
func (spm *StructuredPatternManager) loadStructuredPatterns(patternsDir string) error {
	// For now, define some common structured patterns programmatically
	// This could later be extended to read from YAML configuration files
	
	spm.structuredPatterns["application"] = []StructuredPattern{
		{
			Name: "HTTP_ERROR_RESPONSE",
			FieldRules: map[string]*regexp.Regexp{
				"status_code": regexp.MustCompile(`^[45]\d{2}$`),
				"level":       regexp.MustCompile(`(?i)error|warn`),
			},
			Score:       2.0,
			Description: "HTTP error response detected via status code",
			RequiredFields: []string{"status_code"},
		},
		{
			Name: "SLOW_REQUEST",
			FieldRules: map[string]*regexp.Regexp{
				"duration": regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*(ms|milliseconds|s|seconds)`),
			},
			Score:       1.5,
			Description: "Slow request detected",
		},
		{
			Name: "AUTHENTICATION_FAILURE",
			FieldRules: map[string]*regexp.Regexp{
				"level":   regexp.MustCompile(`(?i)error|warn`),
				"message": regexp.MustCompile(`(?i)(auth|login|password|credential).*fail`),
			},
			Score:       3.0,
			Description: "Authentication failure in structured log",
		},
		{
			Name: "EXCEPTION_WITH_STACK",
			FieldRules: map[string]*regexp.Regexp{
				"error": regexp.MustCompile(`.+`),
				"stack": regexp.MustCompile(`.+`),
			},
			Score:       2.5,
			Description: "Exception with stack trace",
			RequiredFields: []string{"error", "stack"},
		},
	}

	spm.structuredPatterns["system"] = []StructuredPattern{
		{
			Name: "SYSTEM_ERROR_STRUCTURED",
			FieldRules: map[string]*regexp.Regexp{
				"level":   regexp.MustCompile(`(?i)error|critical|alert|emergency`),
				"service": regexp.MustCompile(`(?i)systemd|kernel|init`),
			},
			Score:       4.0,
			Description: "System-level error in structured format",
		},
		{
			Name: "SERVICE_RESTART",
			FieldRules: map[string]*regexp.Regexp{
				"message": regexp.MustCompile(`(?i)(restart|reboot|reload)`),
				"service": regexp.MustCompile(`.+`),
			},
			Score:       2.0,
			Description: "Service restart detected",
		},
	}

	spm.structuredPatterns["security"] = []StructuredPattern{
		{
			Name: "FAILED_LOGIN_STRUCTURED",
			FieldRules: map[string]*regexp.Regexp{
				"level":   regexp.MustCompile(`(?i)warn|error`),
				"message": regexp.MustCompile(`(?i)(failed|invalid).*(login|auth)`),
			},
			Score:       2.5,
			Description: "Failed login attempt in structured log",
		},
		{
			Name: "SUSPICIOUS_IP_ACCESS",
			FieldRules: map[string]*regexp.Regexp{
				"ip":      regexp.MustCompile(`\d+\.\d+\.\d+\.\d+`),
				"level":   regexp.MustCompile(`(?i)warn|error`),
			},
			Score:       1.8,
			Description: "Suspicious IP access pattern",
		},
	}

	log.Infof("Loaded structured patterns for %d categories", len(spm.structuredPatterns))
	return nil
}

// MatchPatterns matches against both structured and fallback patterns
func (spm *StructuredPatternManager) MatchPatterns(message string, labels map[string]string) []PatternMatch {
	spm.mutex.RLock()
	defer spm.mutex.RUnlock()

	var matches []PatternMatch

	// Parse the log entry first
	originalEntry := LogEntry{
		Message: message,
		Labels:  labels,
		Host:    getLabel(labels, "hostname", "unknown"),
		Service: getLabel(labels, "unit", getLabel(labels, "container", "unknown")),
		Level:   getLabel(labels, "level", "info"),
	}

	normalizedEntry := spm.jsonParser.ParseLogEntry(originalEntry)

	// If it's structured JSON, try structured patterns first
	if normalizedEntry.IsStructured {
		structuredMatches := spm.matchStructuredPatterns(normalizedEntry)
		matches = append(matches, structuredMatches...)
	}

	// Always try fallback patterns for additional matches
	fallbackMatches := spm.matchFallbackPatterns(message, labels)
	matches = append(matches, fallbackMatches...)

	return matches
}

// matchStructuredPatterns matches against structured field patterns
func (spm *StructuredPatternManager) matchStructuredPatterns(entry NormalizedLogEntry) []PatternMatch {
	var matches []PatternMatch
	
	categories := spm.getRelevantStructuredCategories(entry)
	structuredFields := entry.GetStructuredFields()

	for _, category := range categories {
		patterns, exists := spm.structuredPatterns[category]
		if !exists {
			continue
		}

		for _, pattern := range patterns {
			if spm.matchesStructuredPattern(pattern, structuredFields) {
				matches = append(matches, PatternMatch{
					PatternName:   pattern.Name,
					PatternRegex:  spm.structuredPatternToRegex(pattern),
					AnomalyScore:  pattern.Score,
					Description:   pattern.Description,
					MatchedText:   spm.getMatchedText(pattern, structuredFields),
				})
			}
		}
	}

	return matches
}

// matchesStructuredPattern checks if a structured pattern matches the fields
func (spm *StructuredPatternManager) matchesStructuredPattern(pattern StructuredPattern, fields map[string]string) bool {
	// Check required fields first
	for _, requiredField := range pattern.RequiredFields {
		if _, exists := fields[requiredField]; !exists {
			return false
		}
	}

	// Check field rules
	matchedRules := 0
	for fieldName, regex := range pattern.FieldRules {
		if fieldValue, exists := fields[fieldName]; exists {
			if regex.MatchString(fieldValue) {
				matchedRules++
			}
		}
	}

	// Need to match at least one rule, or all rules if it's a strict pattern
	if len(pattern.RequiredFields) > 0 {
		// Strict pattern: must match all field rules that have corresponding fields
		expectedMatches := 0
		for fieldName := range pattern.FieldRules {
			if _, exists := fields[fieldName]; exists {
				expectedMatches++
			}
		}
		return matchedRules == expectedMatches && expectedMatches > 0
	} else {
		// Flexible pattern: match at least one rule
		return matchedRules > 0
	}
}

// getMatchedText generates a representative matched text for structured patterns
func (spm *StructuredPatternManager) getMatchedText(pattern StructuredPattern, fields map[string]string) string {
	var parts []string
	
	for fieldName, regex := range pattern.FieldRules {
		if fieldValue, exists := fields[fieldName]; exists {
			if regex.MatchString(fieldValue) {
				parts = append(parts, fmt.Sprintf("%s=%s", fieldName, fieldValue))
			}
		}
	}
	
	if len(parts) == 0 {
		return "structured_match"
	}
	
	return strings.Join(parts, ", ")
}

// structuredPatternToRegex converts a structured pattern to a regex string for display
func (spm *StructuredPatternManager) structuredPatternToRegex(pattern StructuredPattern) string {
	var parts []string
	for fieldName, regex := range pattern.FieldRules {
		parts = append(parts, fmt.Sprintf("field:%s~%s", fieldName, regex.String()))
	}
	return strings.Join(parts, " AND ")
}

// matchFallbackPatterns uses original regex patterns for non-JSON logs
func (spm *StructuredPatternManager) matchFallbackPatterns(message string, labels map[string]string) []PatternMatch {
	var matches []PatternMatch
	categories := spm.getRelevantCategories(labels)

	for _, category := range categories {
		patterns, exists := spm.fallbackPatterns[category]
		if !exists {
			continue
		}

		for _, pattern := range patterns {
			if match := pattern.Regex.FindString(message); match != "" {
				matches = append(matches, PatternMatch{
					PatternName:   pattern.Name,
					PatternRegex:  pattern.Regex.String(),
					AnomalyScore:  pattern.Score,
					Description:   pattern.Description,
					MatchedText:   match,
				})
			}
		}
	}

	return matches
}

// getRelevantStructuredCategories determines relevant categories for structured logs
func (spm *StructuredPatternManager) getRelevantStructuredCategories(entry NormalizedLogEntry) []string {
	categories := []string{"system", "application"} // Always check these

	// Add categories based on structured fields
	if entry.StatusCode != "" {
		categories = append(categories, "network")
	}

	if entry.Error != "" || entry.Level == "error" || entry.Level == "critical" {
		categories = append(categories, "system")
	}

	if entry.IP != "" || entry.UserID != "" || strings.Contains(entry.Message, "auth") {
		categories = append(categories, "security")
	}

	if strings.Contains(entry.Service, "docker") || strings.Contains(entry.Service, "container") {
		categories = append(categories, "docker")
	}

	// Media services
	mediaServices := []string{"plex", "sonarr", "radarr", "bazarr", "nzbget", "prowlarr", "tautulli", "overseerr", "tdarr", "audible"}
	serviceLower := strings.ToLower(entry.Service)
	for _, service := range mediaServices {
		if strings.Contains(serviceLower, service) {
			categories = append(categories, "media")
			break
		}
	}

	// Remove duplicates
	seen := make(map[string]bool)
	var unique []string
	for _, cat := range categories {
		if !seen[cat] {
			unique = append(unique, cat)
			seen[cat] = true
		}
	}

	return unique
}

// getRelevantCategories determines relevant categories for fallback patterns
func (spm *StructuredPatternManager) getRelevantCategories(labels map[string]string) []string {
	categories := []string{"system", "application"} // Always check these

	container := labels["container"]
	unit := labels["unit"]

	// Docker containers
	if container != "" {
		categories = append(categories, "docker")

		// Media stack containers
		mediaServices := []string{"plex", "sonarr", "radarr", "bazarr", "nzbget", "prowlarr", "tautulli", "overseerr", "tdarr", "audible"}
		for _, service := range mediaServices {
			if strings.Contains(strings.ToLower(container), service) {
				categories = append(categories, "media")
				break
			}
		}
	}

	// Network-related services
	if unit != "" {
		unitLower := strings.ToLower(unit)
		networkServices := []string{"nginx", "ssh", "dns", "dhcp", "firewall", "iptables"}
		for _, service := range networkServices {
			if strings.Contains(unitLower, service) {
				categories = append(categories, "network")
				break
			}
		}

		// Security-related services
		securityServices := []string{"auth", "sudo", "pam", "ssh", "login"}
		for _, service := range securityServices {
			if strings.Contains(unitLower, service) {
				categories = append(categories, "security")
				break
			}
		}
	}

	// Remove duplicates
	seen := make(map[string]bool)
	var unique []string
	for _, cat := range categories {
		if !seen[cat] {
			unique = append(unique, cat)
			seen[cat] = true
		}
	}

	return unique
}

// GetPatterns returns pattern information for both structured and fallback patterns
func (spm *StructuredPatternManager) GetPatterns() map[string]interface{} {
	spm.mutex.RLock()
	defer spm.mutex.RUnlock()

	result := make(map[string]interface{})

	// Add structured patterns
	structuredInfo := make(map[string]interface{})
	for category, patterns := range spm.structuredPatterns {
		categoryPatterns := make([]map[string]interface{}, len(patterns))
		for i, pattern := range patterns {
			categoryPatterns[i] = map[string]interface{}{
				"name":           pattern.Name,
				"score":          pattern.Score,
				"description":    pattern.Description,
				"type":           "structured",
				"field_rules":    spm.fieldRulesToStrings(pattern.FieldRules),
				"required_fields": pattern.RequiredFields,
			}
		}
		structuredInfo[category] = categoryPatterns
	}
	result["structured"] = structuredInfo

	// Add fallback patterns
	fallbackInfo := make(map[string]interface{})
	for category, patterns := range spm.fallbackPatterns {
		categoryPatterns := make([]map[string]interface{}, len(patterns))
		for i, pattern := range patterns {
			categoryPatterns[i] = map[string]interface{}{
				"name":        pattern.Name,
				"score":       pattern.Score,
				"description": pattern.Description,
				"type":        "regex",
			}
		}
		fallbackInfo[category] = categoryPatterns
	}
	result["fallback"] = fallbackInfo

	return result
}

// fieldRulesToStrings converts field rules to string representation
func (spm *StructuredPatternManager) fieldRulesToStrings(rules map[string]*regexp.Regexp) map[string]string {
	result := make(map[string]string)
	for field, regex := range rules {
		result[field] = regex.String()
	}
	return result
}

// GetPatternsCount returns total count of loaded patterns
func (spm *StructuredPatternManager) GetPatternsCount() int {
	spm.mutex.RLock()
	defer spm.mutex.RUnlock()

	count := 0
	for _, patterns := range spm.structuredPatterns {
		count += len(patterns)
	}
	for _, patterns := range spm.fallbackPatterns {
		count += len(patterns)
	}
	return count
}

// ReloadPatterns reloads all patterns
func (spm *StructuredPatternManager) ReloadPatterns(patternsDir string) error {
	log.Info("Reloading structured and fallback patterns")
	return spm.loadPatterns(patternsDir)
}

// updatePatternsLoadedMetrics updates the Prometheus metrics for loaded patterns
func (spm *StructuredPatternManager) updatePatternsLoadedMetrics() {
	// Reset metrics first
	patternsLoadedTotal.Reset()

	// Count structured patterns by category
	for category, patterns := range spm.structuredPatterns {
		patternsLoadedTotal.WithLabelValues(category, "structured").Set(float64(len(patterns)))
	}

	// Count fallback patterns by category
	for category, patterns := range spm.fallbackPatterns {
		patternsLoadedTotal.WithLabelValues(category, "fallback").Set(float64(len(patterns)))
	}

	log.Infof("Updated pattern metrics: %d structured categories, %d fallback categories", 
		len(spm.structuredPatterns), len(spm.fallbackPatterns))
}
