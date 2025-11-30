package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "github.com/lib/pq"
	log "github.com/sirupsen/logrus"
)

// Database handles PostgreSQL operations
type Database struct {
	db *sql.DB
}

// NewDatabase creates a new database connection
func NewDatabase(config Config) (*Database, error) {
	connStr := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		config.PostgresHost,
		config.PostgresPort,
		config.PostgresUser,
		config.PostgresPassword,
		config.PostgresDB,
	)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Test connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	d := &Database{db: db}

	// Initialize schema
	if err := d.initSchema(); err != nil {
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	log.Info("Database connection established")
	return d, nil
}

// initSchema creates the required tables
func (d *Database) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS problems (
		id TEXT PRIMARY KEY,
		fingerprint TEXT NOT NULL,
		title TEXT NOT NULL,
		severity TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'active',
		first_seen TIMESTAMP NOT NULL,
		last_seen TIMESTAMP NOT NULL,
		resolved_at TIMESTAMP,
		occurrence_count INTEGER DEFAULT 1,
		affected_hosts TEXT,
		affected_services TEXT,
		sample_anomalies TEXT,
		llm_analysis TEXT,
		suppress_reason TEXT,
		created_at TIMESTAMP NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMP NOT NULL DEFAULT NOW()
	);

	CREATE INDEX IF NOT EXISTS idx_problems_status ON problems(status);
	CREATE INDEX IF NOT EXISTS idx_problems_fingerprint ON problems(fingerprint);
	CREATE INDEX IF NOT EXISTS idx_problems_first_seen ON problems(first_seen);
	CREATE INDEX IF NOT EXISTS idx_problems_severity ON problems(severity, status);
	`

	_, err := d.db.Exec(schema)
	return err
}

// Close closes the database connection
func (d *Database) Close() error {
	return d.db.Close()
}

// Ping checks database connectivity
func (d *Database) Ping() error {
	return d.db.Ping()
}

// GetProblemByFingerprint finds a problem by fingerprint
func (d *Database) GetProblemByFingerprint(fingerprint string) (*Problem, error) {
	query := `
		SELECT id, fingerprint, title, severity, status, first_seen, last_seen, 
		       resolved_at, occurrence_count, affected_hosts, affected_services,
		       sample_anomalies, llm_analysis, suppress_reason, created_at, updated_at
		FROM problems 
		WHERE fingerprint = $1 AND status = 'active'
		LIMIT 1
	`

	var p Problem
	var affectedHosts, affectedServices, sampleAnomalies sql.NullString
	var llmAnalysis, suppressReason sql.NullString
	var resolvedAt sql.NullTime

	err := d.db.QueryRow(query, fingerprint).Scan(
		&p.ID, &p.Fingerprint, &p.Title, &p.Severity, &p.Status,
		&p.FirstSeen, &p.LastSeen, &resolvedAt, &p.OccurrenceCount,
		&affectedHosts, &affectedServices, &sampleAnomalies,
		&llmAnalysis, &suppressReason, &p.CreatedAt, &p.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// Parse nullable fields
	if resolvedAt.Valid {
		t := resolvedAt.Time; p.ResolvedAt = &t
	}
	if llmAnalysis.Valid {
		p.LLMAnalysis = llmAnalysis.String
	}
	if suppressReason.Valid {
		p.SuppressReason = suppressReason.String
	}
	if affectedHosts.Valid {
		json.Unmarshal([]byte(affectedHosts.String), &p.AffectedHosts)
	}
	if affectedServices.Valid {
		json.Unmarshal([]byte(affectedServices.String), &p.AffectedServices)
	}
	if sampleAnomalies.Valid {
		json.Unmarshal([]byte(sampleAnomalies.String), &p.SampleAnomalies)
	}

	return &p, nil
}

// CreateProblem creates a new problem
func (d *Database) CreateProblem(p *Problem) error {
	hostsJSON, _ := json.Marshal(p.AffectedHosts)
	servicesJSON, _ := json.Marshal(p.AffectedServices)
	samplesJSON, _ := json.Marshal(p.SampleAnomalies)

	query := `
		INSERT INTO problems (
			id, fingerprint, title, severity, status, first_seen, last_seen,
			occurrence_count, affected_hosts, affected_services, sample_anomalies,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`

	_, err := d.db.Exec(query,
		p.ID, p.Fingerprint, p.Title, p.Severity, p.Status,
		p.FirstSeen, p.LastSeen, p.OccurrenceCount,
		string(hostsJSON), string(servicesJSON), string(samplesJSON),
		p.CreatedAt, p.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to create problem: %w", err)
	}

	problemsCreated.Inc()
	return nil
}

// UpdateProblem updates an existing problem
func (d *Database) UpdateProblem(p *Problem) error {
	hostsJSON, _ := json.Marshal(p.AffectedHosts)
	servicesJSON, _ := json.Marshal(p.AffectedServices)
	samplesJSON, _ := json.Marshal(p.SampleAnomalies)

	query := `
		UPDATE problems SET
			title = $1,
			severity = $2,
			last_seen = $3,
			occurrence_count = $4,
			affected_hosts = $5,
			affected_services = $6,
			sample_anomalies = $7,
			updated_at = $8
		WHERE id = $9
	`

	_, err := d.db.Exec(query,
		p.Title, p.Severity, p.LastSeen, p.OccurrenceCount,
		string(hostsJSON), string(servicesJSON), string(samplesJSON),
		time.Now(), p.ID,
	)

	return err
}

// GetProblem retrieves a problem by ID
func (d *Database) GetProblem(id string) (*Problem, error) {
	query := `
		SELECT id, fingerprint, title, severity, status, first_seen, last_seen, 
		       resolved_at, occurrence_count, affected_hosts, affected_services,
		       sample_anomalies, llm_analysis, suppress_reason, created_at, updated_at
		FROM problems 
		WHERE id = $1
	`

	var p Problem
	var affectedHosts, affectedServices, sampleAnomalies sql.NullString
	var llmAnalysis, suppressReason sql.NullString
	var resolvedAt sql.NullTime

	err := d.db.QueryRow(query, id).Scan(
		&p.ID, &p.Fingerprint, &p.Title, &p.Severity, &p.Status,
		&p.FirstSeen, &p.LastSeen, &resolvedAt, &p.OccurrenceCount,
		&affectedHosts, &affectedServices, &sampleAnomalies,
		&llmAnalysis, &suppressReason, &p.CreatedAt, &p.UpdatedAt,
	)

	if err != nil {
		return nil, err
	}

	if resolvedAt.Valid {
		t := resolvedAt.Time; p.ResolvedAt = &t
	}
	if llmAnalysis.Valid {
		p.LLMAnalysis = llmAnalysis.String
	}
	if suppressReason.Valid {
		p.SuppressReason = suppressReason.String
	}
	if affectedHosts.Valid {
		json.Unmarshal([]byte(affectedHosts.String), &p.AffectedHosts)
	}
	if affectedServices.Valid {
		json.Unmarshal([]byte(affectedServices.String), &p.AffectedServices)
	}
	if sampleAnomalies.Valid {
		json.Unmarshal([]byte(sampleAnomalies.String), &p.SampleAnomalies)
	}

	return &p, nil
}

// GetProblems retrieves problems by status
func (d *Database) GetProblems(status string) ([]Problem, error) {
	query := `
		SELECT id, fingerprint, title, severity, status, first_seen, last_seen, 
		       resolved_at, occurrence_count, affected_hosts, affected_services,
		       sample_anomalies, llm_analysis, suppress_reason, created_at, updated_at
		FROM problems 
		WHERE status = $1
		ORDER BY 
			CASE severity 
				WHEN 'critical' THEN 1 
				WHEN 'high' THEN 2 
				WHEN 'medium' THEN 3 
				ELSE 4 
			END,
			first_seen DESC
		LIMIT 100
	`

	rows, err := d.db.Query(query, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var problems []Problem
	for rows.Next() {
		var p Problem
		var affectedHosts, affectedServices, sampleAnomalies sql.NullString
		var llmAnalysis, suppressReason sql.NullString
		var resolvedAt sql.NullTime

		err := rows.Scan(
			&p.ID, &p.Fingerprint, &p.Title, &p.Severity, &p.Status,
			&p.FirstSeen, &p.LastSeen, &resolvedAt, &p.OccurrenceCount,
			&affectedHosts, &affectedServices, &sampleAnomalies,
			&llmAnalysis, &suppressReason, &p.CreatedAt, &p.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}

		if resolvedAt.Valid {
			t := resolvedAt.Time; p.ResolvedAt = &t
		}
		if llmAnalysis.Valid {
			p.LLMAnalysis = llmAnalysis.String
		}
		if suppressReason.Valid {
			p.SuppressReason = suppressReason.String
		}
		if affectedHosts.Valid {
			json.Unmarshal([]byte(affectedHosts.String), &p.AffectedHosts)
		}
		if affectedServices.Valid {
			json.Unmarshal([]byte(affectedServices.String), &p.AffectedServices)
		}
		if sampleAnomalies.Valid {
			json.Unmarshal([]byte(sampleAnomalies.String), &p.SampleAnomalies)
		}

		problems = append(problems, p)
	}

	return problems, nil
}

// GetActiveProblemsCount returns count of active problems
func (d *Database) GetActiveProblemsCount() (int64, error) {
	var count int64
	err := d.db.QueryRow("SELECT COUNT(*) FROM problems WHERE status = 'active'").Scan(&count)
	return count, err
}

// ResolveProblem marks a problem as resolved
func (d *Database) ResolveProblem(id string) error {
	query := `
		UPDATE problems SET 
			status = 'resolved',
			resolved_at = NOW(),
			updated_at = NOW()
		WHERE id = $1 AND status = 'active'
	`
	_, err := d.db.Exec(query, id)
	return err
}

// SuppressProblem marks a problem as suppressed
func (d *Database) SuppressProblem(id, reason string) error {
	query := `
		UPDATE problems SET 
			status = 'suppressed',
			suppress_reason = $1,
			updated_at = NOW()
		WHERE id = $2
	`
	_, err := d.db.Exec(query, reason, id)
	return err
}

// AutoResolveStaleProblems resolves problems with no recent activity
func (d *Database) AutoResolveStaleProblems(timeout time.Duration) (int64, error) {
	query := `
		UPDATE problems SET 
			status = 'resolved',
			resolved_at = NOW(),
			updated_at = NOW()
		WHERE status = 'active' 
		AND last_seen < $1
	`
	result, err := d.db.Exec(query, time.Now().Add(-timeout))
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// PurgeOldProblems deletes old resolved/suppressed problems
func (d *Database) PurgeOldProblems(resolvedRetention, suppressedRetention time.Duration) error {
	// Delete old resolved problems
	_, err := d.db.Exec(`
		DELETE FROM problems 
		WHERE status = 'resolved' 
		AND resolved_at < $1
	`, time.Now().Add(-resolvedRetention))
	if err != nil {
		return err
	}

	// Delete old suppressed problems
	_, err = d.db.Exec(`
		DELETE FROM problems 
		WHERE status = 'suppressed' 
		AND updated_at < $1
	`, time.Now().Add(-suppressedRetention))

	return err
}

// GetStats returns problem statistics
func (d *Database) GetStats() (*ProblemStats, error) {
	stats := &ProblemStats{
		BySeverity: make(map[string]int),
	}

	// Active count
	d.db.QueryRow("SELECT COUNT(*) FROM problems WHERE status = 'active'").Scan(&stats.ActiveCount)

	// Resolved today
	d.db.QueryRow(`
		SELECT COUNT(*) FROM problems 
		WHERE status = 'resolved' 
		AND resolved_at >= CURRENT_DATE
	`).Scan(&stats.ResolvedToday)

	// New today
	d.db.QueryRow(`
		SELECT COUNT(*) FROM problems 
		WHERE created_at >= CURRENT_DATE
	`).Scan(&stats.NewToday)

	// By severity
	rows, err := d.db.Query(`
		SELECT severity, COUNT(*) 
		FROM problems 
		WHERE status = 'active' 
		GROUP BY severity
	`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var severity string
			var count int
			rows.Scan(&severity, &count)
			stats.BySeverity[severity] = count
		}
	}

	// Average duration
	d.db.QueryRow(`
		SELECT COALESCE(AVG(EXTRACT(EPOCH FROM (COALESCE(resolved_at, NOW()) - first_seen)) / 60), 0)
		FROM problems
		WHERE status IN ('active', 'resolved')
		AND created_at >= CURRENT_DATE - INTERVAL '7 days'
	`).Scan(&stats.AvgDurationMin)

	return stats, nil
}

// GetActiveProblemsForDigest returns active problems sorted by duration for digest
func (d *Database) GetActiveProblemsForDigest() ([]Problem, error) {
	query := `
		SELECT id, fingerprint, title, severity, status, first_seen, last_seen, 
		       resolved_at, occurrence_count, affected_hosts, affected_services,
		       sample_anomalies, llm_analysis, suppress_reason, created_at, updated_at
		FROM problems 
		WHERE status = 'active'
		ORDER BY first_seen ASC
		LIMIT 50
	`

	rows, err := d.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var problems []Problem
	for rows.Next() {
		var p Problem
		var affectedHosts, affectedServices, sampleAnomalies sql.NullString
		var llmAnalysis, suppressReason sql.NullString
		var resolvedAt sql.NullTime

		err := rows.Scan(
			&p.ID, &p.Fingerprint, &p.Title, &p.Severity, &p.Status,
			&p.FirstSeen, &p.LastSeen, &resolvedAt, &p.OccurrenceCount,
			&affectedHosts, &affectedServices, &sampleAnomalies,
			&llmAnalysis, &suppressReason, &p.CreatedAt, &p.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}

		if resolvedAt.Valid {
			t := resolvedAt.Time; p.ResolvedAt = &t
		}
		if llmAnalysis.Valid {
			p.LLMAnalysis = llmAnalysis.String
		}
		if suppressReason.Valid {
			p.SuppressReason = suppressReason.String
		}
		if affectedHosts.Valid {
			json.Unmarshal([]byte(affectedHosts.String), &p.AffectedHosts)
		}
		if affectedServices.Valid {
			json.Unmarshal([]byte(affectedServices.String), &p.AffectedServices)
		}
		if sampleAnomalies.Valid {
			json.Unmarshal([]byte(sampleAnomalies.String), &p.SampleAnomalies)
		}

		problems = append(problems, p)
	}

	return problems, nil
}

// GetResolvedTodayForDigest returns problems resolved today
func (d *Database) GetResolvedTodayForDigest() ([]Problem, error) {
	query := `
		SELECT id, fingerprint, title, severity, status, first_seen, last_seen, 
		       resolved_at, occurrence_count, llm_analysis
		FROM problems 
		WHERE status = 'resolved' 
		AND resolved_at >= CURRENT_DATE
		ORDER BY resolved_at DESC
		LIMIT 20
	`

	rows, err := d.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var problems []Problem
	for rows.Next() {
		var p Problem
		var llmAnalysis sql.NullString
		var resolvedAt sql.NullTime
		err := rows.Scan(
			&p.ID, &p.Fingerprint, &p.Title, &p.Severity, &p.Status,
			&p.FirstSeen, &p.LastSeen, &resolvedAt, &p.OccurrenceCount,
			&llmAnalysis,
		)
		if err != nil {
			return nil, err
		}
		if resolvedAt.Valid {
			t := resolvedAt.Time; p.ResolvedAt = &t
		}
		if llmAnalysis.Valid {
			p.LLMAnalysis = llmAnalysis.String
		}
		problems = append(problems, p)
	}

	return problems, nil
}

// FlushAll deletes all problems from the database
func (d *Database) FlushAll() error {
	_, err := d.db.Exec("TRUNCATE TABLE problems RESTART IDENTITY CASCADE")
	if err != nil {
		return fmt.Errorf("failed to truncate problems table: %w", err)
	}
	log.Info("Database flushed: all problems deleted")
	return nil
}
