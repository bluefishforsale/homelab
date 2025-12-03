package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// NormalizedLogEntry represents a log entry with normalized field names
type NormalizedLogEntry struct {
	Timestamp   time.Time         `json:"timestamp"`
	Message     string            `json:"message"`
	Host        string            `json:"host"`
	Service     string            `json:"service"`
	Level       string            `json:"level"`
	Component   string            `json:"component"`
	Module      string            `json:"module"`
	RequestID   string            `json:"request_id,omitempty"`
	UserID      string            `json:"user_id,omitempty"`
	IP          string            `json:"ip,omitempty"`
	StatusCode  string            `json:"status_code,omitempty"`
	Duration    string            `json:"duration,omitempty"`
	Error       string            `json:"error,omitempty"`
	Stack       string            `json:"stack,omitempty"`
	RawJSON     map[string]interface{} `json:"raw_json,omitempty"`
	IsStructured bool              `json:"is_structured"`
}

// LogParser handles parsing and normalization of log entries
type LogParser struct {
	// Common field mappings for normalization
	hostFields     []string
	serviceFields  []string
	levelFields    []string
	messageFields  []string
	componentFields []string
	timestampFields []string
}

// NewLogParser creates a new log parser with field mappings
func NewLogParser() *LogParser {
	return &LogParser{
		// Host field variations
		hostFields: []string{
			"host", "hostname", "server", "node", "machine", "host_name", 
			"server_name", "instance", "pod", "container_host",
		},
		
		// Service/application field variations
		serviceFields: []string{
			"service", "app", "application", "program", "daemon", "process",
			"service_name", "app_name", "application_name", "process_name",
			"container", "container_name", "image", "unit", "systemd_unit",
		},
		
		// Log level variations
		levelFields: []string{
			"level", "severity", "priority", "log_level", "loglevel", 
			"syslog_severity", "priority_keyword", "levelname",
		},
		
		// Message field variations
		messageFields: []string{
			"message", "msg", "text", "content", "log", "event", 
			"description", "summary", "details",
		},
		
		// Component/module field variations
		componentFields: []string{
			"component", "module", "logger", "class", "category",
			"logger_name", "class_name", "source", "facility",
		},
		
		// Timestamp field variations
		timestampFields: []string{
			"timestamp", "time", "ts", "@timestamp", "datetime", 
			"created", "logged_at", "event_time",
		},
	}
}

// ParseLogEntry parses a log entry and normalizes field names
func (lp *LogParser) ParseLogEntry(originalEntry LogEntry) NormalizedLogEntry {
	normalized := NormalizedLogEntry{
		Timestamp:    originalEntry.Timestamp,
		Message:      originalEntry.Message,
		Host:         originalEntry.Host,
		Service:      originalEntry.Service,
		Level:        originalEntry.Level,
		IsStructured: false,
	}
	
	// Try to parse the message as JSON
	if lp.isJSON(originalEntry.Message) {
		if jsonData, err := lp.parseJSON(originalEntry.Message); err == nil {
			normalized = lp.normalizeJSONFields(jsonData, originalEntry)
			normalized.IsStructured = true
			normalized.RawJSON = jsonData
		}
	}
	
	// Also extract fields from Promtail/Loki labels if available
	normalized = lp.enhanceFromLabels(normalized, originalEntry.Labels)
	
	// Normalize values
	normalized = lp.normalizeValues(normalized)
	
	return normalized
}

// isJSON checks if a string looks like JSON
func (lp *LogParser) isJSON(message string) bool {
	message = strings.TrimSpace(message)
	return (strings.HasPrefix(message, "{") && strings.HasSuffix(message, "}")) ||
		   (strings.HasPrefix(message, "[") && strings.HasSuffix(message, "]"))
}

// parseJSON parses a JSON string into a map
func (lp *LogParser) parseJSON(message string) (map[string]interface{}, error) {
	var jsonData map[string]interface{}
	if err := json.Unmarshal([]byte(message), &jsonData); err != nil {
		return nil, err
	}
	return jsonData, nil
}

// normalizeJSONFields extracts and normalizes fields from parsed JSON
func (lp *LogParser) normalizeJSONFields(jsonData map[string]interface{}, originalEntry LogEntry) NormalizedLogEntry {
	normalized := NormalizedLogEntry{
		Timestamp:    originalEntry.Timestamp,
		Host:         originalEntry.Host,
		Service:      originalEntry.Service,
		Level:        originalEntry.Level,
		IsStructured: true,
		RawJSON:      jsonData,
	}
	
	// Extract normalized fields
	normalized.Host = lp.extractField(jsonData, lp.hostFields, normalized.Host)
	normalized.Service = lp.extractField(jsonData, lp.serviceFields, normalized.Service)
	normalized.Level = lp.extractField(jsonData, lp.levelFields, normalized.Level)
	normalized.Message = lp.extractField(jsonData, lp.messageFields, originalEntry.Message)
	normalized.Component = lp.extractField(jsonData, lp.componentFields, "")
	
	// Extract additional structured fields
	normalized.RequestID = lp.extractField(jsonData, []string{
		"request_id", "requestId", "req_id", "correlation_id", "trace_id",
	}, "")
	
	normalized.UserID = lp.extractField(jsonData, []string{
		"user_id", "userId", "uid", "user", "username", "account_id",
	}, "")
	
	normalized.IP = lp.extractField(jsonData, []string{
		"ip", "client_ip", "remote_ip", "source_ip", "x_forwarded_for", "remote_addr",
	}, "")
	
	normalized.StatusCode = lp.extractField(jsonData, []string{
		"status_code", "status", "http_status", "response_code", "code",
	}, "")
	
	normalized.Duration = lp.extractField(jsonData, []string{
		"duration", "elapsed", "response_time", "processing_time", "latency",
	}, "")
	
	normalized.Error = lp.extractField(jsonData, []string{
		"error", "err", "exception", "error_message", "failure", "problem",
	}, "")
	
	normalized.Stack = lp.extractField(jsonData, []string{
		"stack", "stacktrace", "stack_trace", "backtrace", "trace",
	}, "")
	
	// Parse timestamp from JSON if available
	if timestampStr := lp.extractField(jsonData, lp.timestampFields, ""); timestampStr != "" {
		if parsedTime, err := lp.parseTimestamp(timestampStr); err == nil {
			normalized.Timestamp = parsedTime
		}
	}
	
	return normalized
}

// extractField extracts a field value using multiple possible field names
func (lp *LogParser) extractField(data map[string]interface{}, fieldNames []string, defaultValue string) string {
	for _, fieldName := range fieldNames {
		if value, exists := data[fieldName]; exists {
			if strValue := lp.interfaceToString(value); strValue != "" {
				return strValue
			}
		}
		
		// Also check with common prefixes/suffixes
		for key, value := range data {
			keyLower := strings.ToLower(key)
			for _, fieldName := range fieldNames {
				if strings.Contains(keyLower, strings.ToLower(fieldName)) {
					if strValue := lp.interfaceToString(value); strValue != "" {
						return strValue
					}
				}
			}
		}
	}
	return defaultValue
}

// interfaceToString converts interface{} to string safely
func (lp *LogParser) interfaceToString(value interface{}) string {
	if value == nil {
		return ""
	}
	
	switch v := value.(type) {
	case string:
		return v
	case int, int8, int16, int32, int64:
		if jsonBytes, err := json.Marshal(v); err == nil {
			return strings.Trim(strings.Replace(strings.Replace(string(jsonBytes), "\"", "", -1), "\n", "", -1), " ")
		}
	case float32, float64:
		if jsonBytes, err := json.Marshal(v); err == nil {
			return string(jsonBytes)
		}
	case bool:
		if v {
			return "true"
		}
		return "false"
	default:
		// Try to JSON marshal complex types
		if jsonBytes, err := json.Marshal(v); err == nil {
			return string(jsonBytes)
		}
	}
	return ""
}

// enhanceFromLabels enhances normalized entry with Promtail/Loki labels
func (lp *LogParser) enhanceFromLabels(normalized NormalizedLogEntry, labels map[string]string) NormalizedLogEntry {
	// Override with more specific Promtail labels if available
	if hostname := labels["hostname"]; hostname != "" {
		normalized.Host = hostname
	}
	
	if unit := labels["unit"]; unit != "" && normalized.Service == "" {
		normalized.Service = unit
	}
	
	if container := labels["container"]; container != "" {
		if normalized.Service == "" || normalized.Service == "unknown" {
			normalized.Service = container
		}
	}
	
	if level := labels["level"]; level != "" {
		normalized.Level = level
	}
	
	if transport := labels["transport"]; transport != "" && normalized.Component == "" {
		normalized.Component = transport
	}
	
	return normalized
}

// normalizeValues normalizes field values to consistent formats
func (lp *LogParser) normalizeValues(entry NormalizedLogEntry) NormalizedLogEntry {
	// Normalize log levels to standard values
	entry.Level = lp.normalizeLogLevel(entry.Level)
	
	// Normalize service names (remove common suffixes/prefixes)
	entry.Service = lp.normalizeServiceName(entry.Service)
	
	// Normalize host names (remove domain suffixes)
	entry.Host = lp.normalizeHostName(entry.Host)
	
	return entry
}

// normalizeLogLevel normalizes log level to standard values
func (lp *LogParser) normalizeLogLevel(level string) string {
	if level == "" {
		return "info"
	}
	
	levelLower := strings.ToLower(strings.TrimSpace(level))
	
	// Map various level formats to standard levels
	switch {
	case strings.Contains(levelLower, "emerg") || strings.Contains(levelLower, "panic"):
		return "emergency"
	case strings.Contains(levelLower, "alert"):
		return "alert" 
	case strings.Contains(levelLower, "crit") || strings.Contains(levelLower, "fatal"):
		return "critical"
	case strings.Contains(levelLower, "err") || strings.Contains(levelLower, "error"):
		return "error"
	case strings.Contains(levelLower, "warn"):
		return "warning"
	case strings.Contains(levelLower, "notice"):
		return "notice"
	case strings.Contains(levelLower, "info"):
		return "info"
	case strings.Contains(levelLower, "debug") || strings.Contains(levelLower, "trace"):
		return "debug"
	default:
		return levelLower
	}
}

// normalizeServiceName cleans up service names
func (lp *LogParser) normalizeServiceName(service string) string {
	if service == "" {
		return "unknown"
	}
	
	service = strings.TrimSpace(service)
	
	// Remove common suffixes
	suffixes := []string{".service", ".timer", ".socket", ".target", "-service"}
	for _, suffix := range suffixes {
		if strings.HasSuffix(service, suffix) {
			service = strings.TrimSuffix(service, suffix)
		}
	}
	
	// Remove container prefixes if present
	if strings.Contains(service, "/") {
		parts := strings.Split(service, "/")
		service = parts[len(parts)-1]
	}
	
	return strings.ToLower(service)
}

// normalizeHostName cleans up host names
func (lp *LogParser) normalizeHostName(host string) string {
	if host == "" {
		return "unknown"
	}
	
	// Remove domain suffix if present
	if strings.Contains(host, ".") {
		parts := strings.Split(host, ".")
		host = parts[0]
	}
	
	return strings.ToLower(strings.TrimSpace(host))
}

// parseTimestamp attempts to parse various timestamp formats
func (lp *LogParser) parseTimestamp(timestampStr string) (time.Time, error) {
	// Common timestamp formats
	formats := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02T15:04:05.000Z",
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04:05.000",
		"Jan 02 15:04:05",
		"Jan 02, 2006 15:04:05",
	}
	
	for _, format := range formats {
		if t, err := time.Parse(format, timestampStr); err == nil {
			return t, nil
		}
	}
	
	return time.Time{}, fmt.Errorf("Unable to parse timestamp: %s", timestampStr)
}

// GetStructuredFields returns a map of normalized structured fields for pattern matching
func (entry *NormalizedLogEntry) GetStructuredFields() map[string]string {
	fields := make(map[string]string)
	
	fields["host"] = entry.Host
	fields["service"] = entry.Service  
	fields["level"] = entry.Level
	fields["message"] = entry.Message
	
	if entry.Component != "" {
		fields["component"] = entry.Component
	}
	if entry.RequestID != "" {
		fields["request_id"] = entry.RequestID
	}
	if entry.UserID != "" {
		fields["user_id"] = entry.UserID
	}
	if entry.IP != "" {
		fields["ip"] = entry.IP
	}
	if entry.StatusCode != "" {
		fields["status_code"] = entry.StatusCode
	}
	if entry.Error != "" {
		fields["error"] = entry.Error
	}
	
	return fields
}
