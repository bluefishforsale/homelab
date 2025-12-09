package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	log "github.com/sirupsen/logrus"
)

// Database handles PostgreSQL operations
type Database struct {
	db *sql.DB
}

// NewDatabase creates a new database connection
func NewDatabase(config *Config) (*Database, error) {
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
	CREATE TABLE IF NOT EXISTS workflow_templates (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT,
		version TEXT,
		definition JSONB NOT NULL,
		created_at TIMESTAMP NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMP NOT NULL DEFAULT NOW()
	);

	CREATE TABLE IF NOT EXISTS workflow_runs (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		template_id TEXT REFERENCES workflow_templates(id),
		status TEXT NOT NULL DEFAULT 'pending',
		inputs JSONB,
		outputs JSONB,
		error TEXT,
		started_at TIMESTAMP,
		completed_at TIMESTAMP,
		created_at TIMESTAMP NOT NULL DEFAULT NOW()
	);

	CREATE TABLE IF NOT EXISTS step_executions (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		run_id UUID REFERENCES workflow_runs(id) ON DELETE CASCADE,
		step_id TEXT NOT NULL,
		role TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'pending',
		prompt TEXT,
		response TEXT,
		tokens_used INTEGER DEFAULT 0,
		cost_usd NUMERIC(10,6) DEFAULT 0,
		latency_ms INTEGER DEFAULT 0,
		error TEXT,
		started_at TIMESTAMP,
		completed_at TIMESTAMP,
		created_at TIMESTAMP NOT NULL DEFAULT NOW()
	);

	CREATE TABLE IF NOT EXISTS artifacts (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		run_id UUID REFERENCES workflow_runs(id) ON DELETE CASCADE,
		step_id TEXT,
		type TEXT NOT NULL,
		name TEXT NOT NULL,
		path TEXT,
		url TEXT,
		metadata JSONB,
		created_at TIMESTAMP NOT NULL DEFAULT NOW()
	);

	CREATE INDEX IF NOT EXISTS idx_runs_status ON workflow_runs(status);
	CREATE INDEX IF NOT EXISTS idx_runs_template ON workflow_runs(template_id);
	CREATE INDEX IF NOT EXISTS idx_runs_created ON workflow_runs(created_at DESC);
	CREATE INDEX IF NOT EXISTS idx_steps_run ON step_executions(run_id);
	CREATE INDEX IF NOT EXISTS idx_artifacts_run ON artifacts(run_id);

	CREATE TABLE IF NOT EXISTS biographies (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		person_id UUID NOT NULL UNIQUE,
		person_type TEXT NOT NULL,
		name TEXT NOT NULL,
		bio TEXT,
		background TEXT,
		personality TEXT,
		goals JSONB,
		values JSONB,
		quirks JSONB,
		created_at TIMESTAMP NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMP NOT NULL DEFAULT NOW()
	);

	CREATE INDEX IF NOT EXISTS idx_biographies_person ON biographies(person_id);
	CREATE INDEX IF NOT EXISTS idx_biographies_type ON biographies(person_type);

	CREATE TABLE IF NOT EXISTS company_seeds (
		id UUID PRIMARY KEY,
		sector TEXT NOT NULL,
		custom_sector TEXT,
		company_name TEXT NOT NULL,
		mission TEXT,
		vision TEXT,
		target_market TEXT,
		initial_budget NUMERIC(15,2) DEFAULT 0,
		active BOOLEAN DEFAULT true,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_company_seeds_active ON company_seeds(active);
	CREATE INDEX IF NOT EXISTS idx_company_seeds_created ON company_seeds(created_at DESC);
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

// SaveWorkflowTemplate saves or updates a workflow template
func (d *Database) SaveWorkflowTemplate(template *WorkflowTemplate) error {
	defJSON, err := json.Marshal(template)
	if err != nil {
		return err
	}

	query := `
		INSERT INTO workflow_templates (id, name, description, version, definition, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			description = EXCLUDED.description,
			version = EXCLUDED.version,
			definition = EXCLUDED.definition,
			updated_at = NOW()
	`
	_, err = d.db.Exec(query, template.ID, template.Name, template.Description, template.Version, defJSON)
	return err
}

// GetWorkflowTemplates returns all workflow templates
func (d *Database) GetWorkflowTemplates() ([]WorkflowTemplate, error) {
	query := `SELECT id, name, description, version, created_at, updated_at FROM workflow_templates ORDER BY name`
	rows, err := d.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var templates []WorkflowTemplate
	for rows.Next() {
		var t WorkflowTemplate
		if err := rows.Scan(&t.ID, &t.Name, &t.Description, &t.Version, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		templates = append(templates, t)
	}
	return templates, rows.Err()
}

// GetWorkflowTemplate returns a single workflow template by ID
func (d *Database) GetWorkflowTemplate(id string) (*WorkflowTemplate, error) {
	query := `SELECT id, name, description, version, definition, created_at, updated_at FROM workflow_templates WHERE id = $1`
	var t WorkflowTemplate
	var defJSON []byte

	err := d.db.QueryRow(query, id).Scan(&t.ID, &t.Name, &t.Description, &t.Version, &defJSON, &t.CreatedAt, &t.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(defJSON, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

// CreateWorkflowRun creates a new workflow run
func (d *Database) CreateWorkflowRun(run *WorkflowRun) error {
	inputsJSON, _ := json.Marshal(run.Inputs)

	query := `
		INSERT INTO workflow_runs (id, template_id, status, inputs, created_at)
		VALUES ($1, $2, $3, $4, NOW())
	`
	_, err := d.db.Exec(query, run.ID, run.TemplateID, run.Status, inputsJSON)
	return err
}

// UpdateWorkflowRun updates an existing workflow run
func (d *Database) UpdateWorkflowRun(run *WorkflowRun) error {
	outputsJSON, _ := json.Marshal(run.Outputs)

	query := `
		UPDATE workflow_runs SET
			status = $2,
			outputs = $3,
			error = $4,
			started_at = $5,
			completed_at = $6
		WHERE id = $1
	`
	_, err := d.db.Exec(query, run.ID, run.Status, outputsJSON, run.Error, run.StartedAt, run.CompletedAt)
	return err
}

// GetWorkflowRuns returns workflow runs with optional status filter
func (d *Database) GetWorkflowRuns(status string, limit int) ([]WorkflowRun, error) {
	query := `
		SELECT id, template_id, status, inputs, outputs, error, started_at, completed_at, created_at
		FROM workflow_runs
	`
	args := []interface{}{}

	if status != "" {
		query += " WHERE status = $1"
		args = append(args, status)
	}
	query += " ORDER BY created_at DESC LIMIT $" + fmt.Sprintf("%d", len(args)+1)
	args = append(args, limit)

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []WorkflowRun
	for rows.Next() {
		var r WorkflowRun
		var inputsJSON, outputsJSON sql.NullString

		if err := rows.Scan(&r.ID, &r.TemplateID, &r.Status, &inputsJSON, &outputsJSON, &r.Error, &r.StartedAt, &r.CompletedAt, &r.CreatedAt); err != nil {
			return nil, err
		}

		if inputsJSON.Valid {
			json.Unmarshal([]byte(inputsJSON.String), &r.Inputs)
		}
		if outputsJSON.Valid {
			json.Unmarshal([]byte(outputsJSON.String), &r.Outputs)
		}
		runs = append(runs, r)
	}
	return runs, rows.Err()
}

// GetWorkflowRun returns a single workflow run by ID
func (d *Database) GetWorkflowRun(id uuid.UUID) (*WorkflowRun, error) {
	query := `
		SELECT id, template_id, status, inputs, outputs, error, started_at, completed_at, created_at
		FROM workflow_runs WHERE id = $1
	`
	var r WorkflowRun
	var inputsJSON, outputsJSON sql.NullString

	err := d.db.QueryRow(query, id).Scan(&r.ID, &r.TemplateID, &r.Status, &inputsJSON, &outputsJSON, &r.Error, &r.StartedAt, &r.CompletedAt, &r.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if inputsJSON.Valid {
		json.Unmarshal([]byte(inputsJSON.String), &r.Inputs)
	}
	if outputsJSON.Valid {
		json.Unmarshal([]byte(outputsJSON.String), &r.Outputs)
	}

	// Load steps
	r.Steps, _ = d.GetStepExecutions(id)

	return &r, nil
}

// GetActiveRunsCount returns the count of running workflows
func (d *Database) GetActiveRunsCount() (int, error) {
	var count int
	err := d.db.QueryRow("SELECT COUNT(*) FROM workflow_runs WHERE status = 'running'").Scan(&count)
	return count, err
}

// CreateStepExecution creates a new step execution record
func (d *Database) CreateStepExecution(step *StepExecution) error {
	query := `
		INSERT INTO step_executions (id, run_id, step_id, role, status, created_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
	`
	_, err := d.db.Exec(query, step.ID, step.RunID, step.StepID, step.Role, step.Status)
	return err
}

// UpdateStepExecution updates an existing step execution
func (d *Database) UpdateStepExecution(step *StepExecution) error {
	query := `
		UPDATE step_executions SET
			status = $2,
			prompt = $3,
			response = $4,
			tokens_used = $5,
			cost_usd = $6,
			latency_ms = $7,
			error = $8,
			started_at = $9,
			completed_at = $10
		WHERE id = $1
	`
	_, err := d.db.Exec(query, step.ID, step.Status, step.Prompt, step.Response,
		step.TokensUsed, step.CostUSD, step.LatencyMs, step.Error, step.StartedAt, step.CompletedAt)
	return err
}

// GetStepExecutions returns all step executions for a run
func (d *Database) GetStepExecutions(runID uuid.UUID) ([]StepExecution, error) {
	query := `
		SELECT id, run_id, step_id, role, status, prompt, response, tokens_used, cost_usd, latency_ms, error, started_at, completed_at, created_at
		FROM step_executions WHERE run_id = $1 ORDER BY created_at
	`
	rows, err := d.db.Query(query, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var steps []StepExecution
	for rows.Next() {
		var s StepExecution
		var prompt, response, stepError sql.NullString

		if err := rows.Scan(&s.ID, &s.RunID, &s.StepID, &s.Role, &s.Status, &prompt, &response,
			&s.TokensUsed, &s.CostUSD, &s.LatencyMs, &stepError, &s.StartedAt, &s.CompletedAt, &s.CreatedAt); err != nil {
			return nil, err
		}
		if prompt.Valid {
			s.Prompt = prompt.String
		}
		if response.Valid {
			s.Response = response.String
		}
		if stepError.Valid {
			s.Error = stepError.String
		}
		steps = append(steps, s)
	}
	return steps, rows.Err()
}

// CreateArtifact creates a new artifact record
func (d *Database) CreateArtifact(artifact *Artifact) error {
	metadataJSON, _ := json.Marshal(artifact.Metadata)

	query := `
		INSERT INTO artifacts (id, run_id, step_id, type, name, path, url, metadata, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
	`
	_, err := d.db.Exec(query, artifact.ID, artifact.RunID, artifact.StepID, artifact.Type,
		artifact.Name, artifact.Path, artifact.URL, metadataJSON)
	return err
}

// GetArtifacts returns all artifacts for a run
func (d *Database) GetArtifacts(runID uuid.UUID) ([]Artifact, error) {
	query := `SELECT id, run_id, step_id, type, name, path, url, metadata, created_at FROM artifacts WHERE run_id = $1`
	rows, err := d.db.Query(query, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var artifacts []Artifact
	for rows.Next() {
		var a Artifact
		var metadataJSON sql.NullString

		if err := rows.Scan(&a.ID, &a.RunID, &a.StepID, &a.Type, &a.Name, &a.Path, &a.URL, &metadataJSON, &a.CreatedAt); err != nil {
			return nil, err
		}
		if metadataJSON.Valid {
			json.Unmarshal([]byte(metadataJSON.String), &a.Metadata)
		}
		artifacts = append(artifacts, a)
	}
	return artifacts, rows.Err()
}
