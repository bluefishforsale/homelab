package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	log "github.com/sirupsen/logrus"
)

// Pattern represents a compiled regex pattern
type Pattern struct {
	Name        string
	Regex       *regexp.Regexp
	Score       float64
	Description string
}

// PatternManager manages loading and matching of log patterns
type PatternManager struct {
	patterns map[string][]Pattern
	mutex    sync.RWMutex
}

// NewPatternManager creates a new pattern manager
func NewPatternManager(patternsDir string) (*PatternManager, error) {
	pm := &PatternManager{
		patterns: make(map[string][]Pattern),
	}

	if err := pm.loadPatterns(patternsDir); err != nil {
		return nil, fmt.Errorf("failed to load patterns: %w", err)
	}

	return pm, nil
}

// loadPatterns loads all pattern files from the patterns directory
func (pm *PatternManager) loadPatterns(patternsDir string) error {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	patternFiles := []string{
		"system.patterns",
		"security.patterns",
		"network.patterns",
		"docker.patterns",
		"application.patterns",
		"media.patterns",
	}

	totalPatterns := 0
	for _, patternFile := range patternFiles {
		filePath := filepath.Join(patternsDir, patternFile)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			log.Warnf("Pattern file %s does not exist, skipping", filePath)
			continue
		}

		category := strings.TrimSuffix(patternFile, ".patterns")
		patterns, err := pm.loadPatternFile(filePath)
		if err != nil {
			log.Errorf("Failed to load pattern file %s: %v", filePath, err)
			continue
		}

		pm.patterns[category] = patterns
		totalPatterns += len(patterns)
		log.Infof("Loaded %d patterns for %s", len(patterns), category)
	}

	log.Infof("Loaded %d total patterns across all categories", totalPatterns)
	return nil
}

// loadPatternFile loads patterns from a single file
func (pm *PatternManager) loadPatternFile(filePath string) ([]Pattern, error) {
	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var patterns []Pattern
	lines := strings.Split(string(content), "\n")

	for lineNum, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, " ", 4)
		if len(parts) < 3 {
			log.Warnf("Invalid pattern line %d in %s: %s", lineNum+1, filePath, line)
			continue
		}

		name := parts[0]
		regexStr := parts[1]
		scoreStr := parts[2]
		description := ""
		if len(parts) > 3 {
			description = parts[3]
		}

		score, err := strconv.ParseFloat(scoreStr, 64)
		if err != nil {
			log.Warnf("Invalid score in pattern %s: %s", name, scoreStr)
			score = 1.0
		}

		// Compile regex with case-insensitive and multiline flags
		regex, err := regexp.Compile("(?i)" + regexStr)
		if err != nil {
			log.Warnf("Failed to compile regex for pattern %s: %v", name, err)
			continue
		}

		patterns = append(patterns, Pattern{
			Name:        name,
			Regex:       regex,
			Score:       score,
			Description: description,
		})
	}

	return patterns, nil
}

// MatchPatterns matches log message against all loaded patterns
func (pm *PatternManager) MatchPatterns(message string, labels map[string]string) []PatternMatch {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()

	var matches []PatternMatch
	categories := pm.getRelevantCategories(labels)

	for _, category := range categories {
		patterns, exists := pm.patterns[category]
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
				
				// For performance, only match first occurrence of each pattern
				// This prevents multiple matches of the same pattern in one message
				break
			}
		}
	}

	return matches
}

// getRelevantCategories determines which pattern categories are relevant
func (pm *PatternManager) getRelevantCategories(labels map[string]string) []string {
	categories := []string{"system", "application"} // Always check these

	container := labels["container"]
	unit := labels["unit"]

	// Docker containers
	if container != "" {
		categories = append(categories, "docker")

		// Media stack containers
		mediaServices := []string{"plex", "sonarr", "radarr", "bazarr", "nzbget", "prowlarr", "tautulli", "overseerr", "tdarr", "audible"}
		containerLower := strings.ToLower(container)
		for _, service := range mediaServices {
			if strings.Contains(containerLower, service) {
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

// GetPatterns returns all loaded patterns
func (pm *PatternManager) GetPatterns() map[string][]Pattern {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()
	
	// Create a copy to avoid race conditions
	result := make(map[string][]Pattern)
	for k, v := range pm.patterns {
		result[k] = make([]Pattern, len(v))
		copy(result[k], v)
	}
	
	return result
}

// GetPatternsCount returns the total number of loaded patterns
func (pm *PatternManager) GetPatternsCount() int {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()
	
	count := 0
	for _, patterns := range pm.patterns {
		count += len(patterns)
	}
	
	return count
}

// ReloadPatterns reloads patterns from the patterns directory
func (pm *PatternManager) ReloadPatterns(patternsDir string) error {
	log.Info("Reloading patterns")
	return pm.loadPatterns(patternsDir)
}
