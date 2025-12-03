package main

import "time"

// PatternMatch represents a pattern match from Tier 1
type PatternMatch struct {
	PatternName  string  `json:"pattern_name"`
	PatternRegex string  `json:"pattern_regex"`
	AnomalyScore float64 `json:"anomaly_score"`
	Category     string  `json:"category"`
	Description  string  `json:"description"`
}

// Anomaly represents an incoming anomaly from Tier 1
type Anomaly struct {
	Timestamp         string             `json:"timestamp"`
	Host              string             `json:"host"`
	Service           string             `json:"service"`
	Severity          string             `json:"severity"`
	AnomalyType       string             `json:"anomaly_type"`
	Score             float64            `json:"score"`
	Description       string             `json:"description"`
	LogMessage        string             `json:"log_message"`
	PatternMatches    []PatternMatch     `json:"pattern_matches"`
	StatisticalScores map[string]float64 `json:"statistical_scores"`
}

// Problem represents an ongoing issue grouping related anomalies
type Problem struct {
	ID               string    `json:"id" db:"id"`
	Fingerprint      string    `json:"fingerprint" db:"fingerprint"`
	Title            string    `json:"title" db:"title"`
	Severity         string    `json:"severity" db:"severity"`
	Status           string    `json:"status" db:"status"`
	FirstSeen        time.Time `json:"first_seen" db:"first_seen"`
	LastSeen         time.Time `json:"last_seen" db:"last_seen"`
	ResolvedAt       *time.Time `json:"resolved_at,omitempty" db:"resolved_at"`
	OccurrenceCount  int       `json:"occurrence_count" db:"occurrence_count"`
	AffectedHosts    []string  `json:"affected_hosts" db:"-"`
	AffectedServices []string  `json:"affected_services" db:"-"`
	SampleAnomalies  []string  `json:"sample_anomalies" db:"-"`
	LLMAnalysis      string    `json:"llm_analysis,omitempty" db:"llm_analysis"`
	SuppressReason   string    `json:"suppress_reason,omitempty" db:"suppress_reason"`
	CreatedAt        time.Time `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time `json:"updated_at" db:"updated_at"`
}

// Duration returns the problem duration
func (p *Problem) Duration() time.Duration {
	if p.Status == "resolved" && p.ResolvedAt != nil {
		return p.ResolvedAt.Sub(p.FirstSeen)
	}
	return time.Since(p.FirstSeen)
}

// DurationString returns a human-readable duration
func (p *Problem) DurationString() string {
	d := p.Duration()
	if d < time.Hour {
		return formatMinutes(int(d.Minutes()))
	} else if d < 24*time.Hour {
		hours := int(d.Hours())
		minutes := int(d.Minutes()) % 60
		return formatHoursMinutes(hours, minutes)
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	return formatDaysHours(days, hours)
}

func formatMinutes(m int) string {
	return pluralize(m, "minute")
}

func formatHoursMinutes(h, m int) string {
	if m == 0 {
		return pluralize(h, "hour")
	}
	return pluralize(h, "hour") + " " + pluralize(m, "minute")
}

func formatDaysHours(d, h int) string {
	if h == 0 {
		return pluralize(d, "day")
	}
	return pluralize(d, "day") + " " + pluralize(h, "hour")
}

func pluralize(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return string(rune('0'+n/10)) + string(rune('0'+n%10)) + " " + word + "s"
}

// ProcessResult represents the result of processing an anomaly
type ProcessResult struct {
	Status       string `json:"status"`
	ProblemID    string `json:"problem_id"`
	IsNewProblem bool   `json:"is_new_problem"`
}

// ComponentHealth represents health of a single component
type ComponentHealth struct {
	Status    string `json:"status"`
	LatencyMs int64  `json:"latency_ms,omitempty"`
	Error     string `json:"error,omitempty"`
}

// HealthResponse represents the health check response
type HealthResponse struct {
	Status         string                     `json:"status"`
	Mode           string                     `json:"mode"`
	Service        string                     `json:"service"`
	UptimeSeconds  int64                      `json:"uptime_seconds"`
	Checks         map[string]ComponentHealth `json:"checks"`
	ProblemsActive int                        `json:"problems_active"`
	QueueDepth     int                        `json:"queue_depth"`
}

// ProblemStats represents problem statistics
type ProblemStats struct {
	ActiveCount    int            `json:"active_count"`
	ResolvedToday  int            `json:"resolved_today"`
	NewToday       int            `json:"new_today"`
	BySeverity     map[string]int `json:"by_severity"`
	AvgDurationMin float64        `json:"avg_duration_minutes"`
}

// AlertManagerAlert represents an alert to send to AlertManager
type AlertManagerAlert struct {
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	StartsAt    string            `json:"startsAt"`
	EndsAt      string            `json:"endsAt,omitempty"`
	GeneratorURL string           `json:"generatorURL,omitempty"`
}
