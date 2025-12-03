package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

// Processor handles anomaly processing
type Processor struct {
	db       *Database
	redis    *RedisClient
	alertMgr *AlertManagerClient
	digest   *DigestSender
	llm      *LLMClient
	config   Config
}

// NewProcessor creates a new processor
func NewProcessor(db *Database, redis *RedisClient, alertMgr *AlertManagerClient, config Config) *Processor {
	var llm *LLMClient
	if config.LLMUrl != "" {
		llm = NewLLMClient(config.LLMUrl)
		if llm.IsAvailable() {
			log.Infof("LLM client initialized: %s", config.LLMUrl)
			healthStatus.WithLabelValues("llm").Set(1)
		} else {
			log.Warnf("LLM server not available at %s - analysis disabled", config.LLMUrl)
			healthStatus.WithLabelValues("llm").Set(0)
		}
	}

	return &Processor{
		db:       db,
		redis:    redis,
		alertMgr: alertMgr,
		digest:   NewDigestSender(config),
		llm:      llm,
		config:   config,
	}
}

// ProcessAnomaly processes an incoming anomaly
func (p *Processor) ProcessAnomaly(anomaly Anomaly) (*ProcessResult, error) {
	start := time.Now()
	defer func() {
		processingDuration.WithLabelValues("process_anomaly").Observe(time.Since(start).Seconds())
	}()

	// Generate fingerprint for deduplication and problem grouping
	fingerprint := p.generateFingerprint(anomaly)

	// Check for duplicate
	hash := p.generateAnomalyHash(anomaly)
	isDupe, err := p.redis.IsDuplicate(hash)
	if err != nil {
		log.Warnf("Failed to check duplicate: %v", err)
	} else if isDupe {
		anomaliesDuplicate.Inc()
		return &ProcessResult{Status: "duplicate", ProblemID: "", IsNewProblem: false}, nil
	}

	// Mark as seen
	p.redis.MarkSeen(hash)

	// Check rate limit
	withinLimit, err := p.redis.CheckRateLimit(anomaly.Host, anomaly.Service, 100)
	if err != nil {
		log.Warnf("Failed to check rate limit: %v", err)
	} else if !withinLimit {
		anomaliesRateLimited.Inc()
		return &ProcessResult{Status: "rate_limited", ProblemID: "", IsNewProblem: false}, nil
	}

	// Find or create problem
	problem, isNew, err := p.findOrCreateProblem(fingerprint, anomaly)
	if err != nil {
		return nil, fmt.Errorf("failed to process problem: %w", err)
	}

	// Update metrics
	anomaliesProcessed.WithLabelValues(anomaly.Severity, anomaly.Host).Inc()

	// Trigger LLM analysis for new high/critical problems
	if isNew && p.llm != nil && (anomaly.Severity == "high" || anomaly.Severity == "critical") {
		p.llm.AnalyzeProblemAsync(problem, p.db)
	}

	// Route alert based on severity
	if err := p.routeAlert(problem, isNew); err != nil {
		log.Warnf("Failed to route alert: %v", err)
	}

	return &ProcessResult{
		Status:       "accepted",
		ProblemID:    problem.ID,
		IsNewProblem: isNew,
	}, nil
}

// generateFingerprint creates a stable identifier for grouping related anomalies
func (p *Processor) generateFingerprint(a Anomaly) string {
	// Fingerprint: host:service:anomaly_type
	parts := []string{
		a.Host,
		a.Service,
		a.AnomalyType,
	}

	// Include first pattern match if available
	if len(a.PatternMatches) > 0 {
		parts = append(parts, a.PatternMatches[0].PatternName)
	}

	return strings.Join(parts, ":")
}

// generateAnomalyHash creates a hash for exact deduplication
func (p *Processor) generateAnomalyHash(a Anomaly) string {
	data := fmt.Sprintf("%s:%s:%s:%s:%s",
		a.Timestamp,
		a.Host,
		a.Service,
		a.AnomalyType,
		a.LogMessage,
	)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:16])
}

// findOrCreateProblem finds an existing problem or creates a new one
func (p *Processor) findOrCreateProblem(fingerprint string, anomaly Anomaly) (*Problem, bool, error) {
	// Check for existing active problem
	existing, err := p.db.GetProblemByFingerprint(fingerprint)
	if err != nil {
		return nil, false, fmt.Errorf("failed to get problem: %w", err)
	}

	now := time.Now()

	if existing != nil {
		// Update existing problem
		existing.LastSeen = now
		existing.OccurrenceCount++
		existing.UpdatedAt = now

		// Update severity if this anomaly is more severe
		if severityRank(anomaly.Severity) > severityRank(existing.Severity) {
			existing.Severity = anomaly.Severity
		}

		// Add host/service if not already present
		existing.AffectedHosts = addUnique(existing.AffectedHosts, anomaly.Host)
		existing.AffectedServices = addUnique(existing.AffectedServices, anomaly.Service)

		// Keep sample anomalies (max 5)
		if len(existing.SampleAnomalies) < 5 {
			existing.SampleAnomalies = append(existing.SampleAnomalies, anomaly.LogMessage)
		}

		if err := p.db.UpdateProblem(existing); err != nil {
			return nil, false, fmt.Errorf("failed to update problem: %w", err)
		}

		return existing, false, nil
	}

	// Create new problem
	problem := &Problem{
		ID:               uuid.New().String(),
		Fingerprint:      fingerprint,
		Title:            generateTitle(anomaly),
		Severity:         anomaly.Severity,
		Status:           "active",
		FirstSeen:        now,
		LastSeen:         now,
		OccurrenceCount:  1,
		AffectedHosts:    []string{anomaly.Host},
		AffectedServices: []string{anomaly.Service},
		SampleAnomalies:  []string{anomaly.LogMessage},
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	if err := p.db.CreateProblem(problem); err != nil {
		return nil, false, fmt.Errorf("failed to create problem: %w", err)
	}

	return problem, true, nil
}

// routeAlert routes the alert based on severity
func (p *Processor) routeAlert(problem *Problem, isNew bool) error {
	// Check if we've already alerted for this problem recently
	alerted, err := p.redis.HasAlerted(problem.ID)
	if err != nil {
		log.Warnf("Failed to check alert status: %v", err)
	}

	// For critical/high: send to AlertManager immediately
	if problem.Severity == "critical" || problem.Severity == "high" {
		if !alerted || isNew {
			if err := p.alertMgr.SendAlert(problem); err != nil {
				return err
			}
			p.redis.MarkAlerted(problem.ID)
		}
	}

	// Medium/low: just track, will be included in daily digest
	return nil
}

// ReplayDeadLetter processes items from the dead letter queue
func (p *Processor) ReplayDeadLetter() int {
	count := 0
	for {
		data, err := p.redis.PopDeadLetter()
		if err != nil {
			break // Queue is empty
		}

		var anomaly Anomaly
		if err := json.Unmarshal([]byte(data), &anomaly); err != nil {
			log.Warnf("Failed to unmarshal dead letter item: %v", err)
			continue
		}

		_, err = p.ProcessAnomaly(anomaly)
		if err != nil {
			log.Warnf("Failed to replay anomaly: %v", err)
			// Push back to dead letter
			p.redis.PushDeadLetter([]byte(data))
			break
		}

		count++
		deadLetterReplayed.Inc()
	}

	return count
}

// StartScheduler starts the background scheduler for digest and cleanup
func (p *Processor) StartScheduler(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	lastDigest := time.Now()

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			// Check if it's time for digest (at configured hour, PT timezone)
			loc, _ := time.LoadLocation("America/Los_Angeles")
			ptNow := now.In(loc)

			if ptNow.Hour() == p.config.DigestHour && ptNow.Minute() == 0 {
				// Only send if we haven't sent in the last 23 hours
				if time.Since(lastDigest) > 23*time.Hour {
					count, err := p.SendDigest()
					if err != nil {
						log.Errorf("Failed to send digest: %v", err)
					} else {
						log.Infof("Sent daily digest with %d problems", count)
						lastDigest = now
					}
				}
			}

			// Run cleanup every hour
			if ptNow.Minute() == 30 {
				p.runCleanup()
			}
		}
	}
}

// StartProblemResolver auto-resolves stale problems
func (p *Processor) StartProblemResolver(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Auto-resolve problems with no activity in 2 hours
			count, err := p.db.AutoResolveStaleProblems(2 * time.Hour)
			if err != nil {
				log.Warnf("Failed to auto-resolve problems: %v", err)
			} else if count > 0 {
				log.Infof("Auto-resolved %d stale problems", count)
				problemsResolved.Add(float64(count))
			}
		}
	}
}

// SendDigest sends the daily digest email
func (p *Processor) SendDigest() (int, error) {
	if p.config.DigestRecipient == "" {
		return 0, fmt.Errorf("no digest recipient configured")
	}

	problems, err := p.db.GetActiveProblemsForDigest()
	if err != nil {
		return 0, err
	}

	resolved, err := p.db.GetResolvedTodayForDigest()
	if err != nil {
		log.Warnf("Failed to get resolved problems: %v", err)
	}

	stats, err := p.db.GetStats()
	if err != nil {
		log.Warnf("Failed to get stats: %v", err)
	}

	if err := p.digest.Send(problems, resolved, stats); err != nil {
		digestsSent.WithLabelValues("error").Inc()
		return 0, err
	}

	digestsSent.WithLabelValues("success").Inc()
	return len(problems), nil
}

// runCleanup performs periodic cleanup tasks
func (p *Processor) runCleanup() {
	// Purge old problems (7 days for resolved, 30 days for suppressed)
	if err := p.db.PurgeOldProblems(7*24*time.Hour, 30*24*time.Hour); err != nil {
		log.Warnf("Failed to purge old problems: %v", err)
	}
}

// Helper functions

func severityRank(s string) int {
	switch s {
	case "critical":
		return 4
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}

func addUnique(slice []string, item string) []string {
	for _, s := range slice {
		if s == item {
			return slice
		}
	}
	return append(slice, item)
}

func generateTitle(a Anomaly) string {
	if a.Description != "" {
		// Truncate description to 100 chars
		desc := a.Description
		if len(desc) > 100 {
			desc = desc[:97] + "..."
		}
		return desc
	}
	return fmt.Sprintf("%s on %s:%s", a.AnomalyType, a.Host, a.Service)
}
