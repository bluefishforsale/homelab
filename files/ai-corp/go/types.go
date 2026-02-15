package main

import (
	"time"

	"github.com/google/uuid"
)

// WorkflowStatus represents the status of a workflow run
type WorkflowStatus string

const (
	StatusPending   WorkflowStatus = "pending"
	StatusRunning   WorkflowStatus = "running"
	StatusCompleted WorkflowStatus = "completed"
	StatusFailed    WorkflowStatus = "failed"
	StatusCancelled WorkflowStatus = "cancelled"
)

// StepStatus represents the status of a workflow step
type StepStatus string

const (
	StepPending   StepStatus = "pending"
	StepRunning   StepStatus = "running"
	StepCompleted StepStatus = "completed"
	StepFailed    StepStatus = "failed"
	StepSkipped   StepStatus = "skipped"
)

// RoleName represents a virtual employee role
type RoleName string

const (
	RoleBoard     RoleName = "board"
	RoleCEO       RoleName = "ceo"
	RoleCTO       RoleName = "cto"
	RoleMarketing RoleName = "marketing"
	RoleArtist    RoleName = "artist"
	RoleWorker    RoleName = "worker"
)

// WorkflowTemplate represents a workflow definition loaded from YAML
type WorkflowTemplate struct {
	ID          string                 `json:"id" db:"id"`
	Name        string                 `json:"name" yaml:"name"`
	Description string                 `json:"description" yaml:"description"`
	Version     string                 `json:"version" yaml:"version"`
	Inputs      []WorkflowInput        `json:"inputs" yaml:"inputs"`
	Steps       []WorkflowStep         `json:"steps" yaml:"steps"`
	OnSuccess   []WorkflowAction       `json:"on_success,omitempty" yaml:"on_success"`
	OnFailure   []WorkflowAction       `json:"on_failure,omitempty" yaml:"on_failure"`
	Definition  map[string]interface{} `json:"-" db:"definition"`
	CreatedAt   time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at" db:"updated_at"`
}

// WorkflowInput defines an input parameter for a workflow
type WorkflowInput struct {
	Name        string `json:"name" yaml:"name"`
	Type        string `json:"type" yaml:"type"`
	Description string `json:"description" yaml:"description"`
	Required    bool   `json:"required" yaml:"required"`
	Default     string `json:"default,omitempty" yaml:"default"`
}

// WorkflowStep defines a single step in a workflow
type WorkflowStep struct {
	ID             string           `json:"id" yaml:"id"`
	Name           string           `json:"name" yaml:"name"`
	Role           RoleName         `json:"role" yaml:"role"`
	Action         string           `json:"action" yaml:"action"`
	DependsOn      []string         `json:"depends_on,omitempty" yaml:"depends_on"`
	Condition      string           `json:"condition,omitempty" yaml:"condition"`
	PromptTemplate string           `json:"prompt_template" yaml:"prompt_template"`
	Outputs        []WorkflowOutput `json:"outputs,omitempty" yaml:"outputs"`
	Timeout        int              `json:"timeout,omitempty" yaml:"timeout"` // seconds
	Retries        int              `json:"retries,omitempty" yaml:"retries"`
}

// WorkflowOutput defines an output from a step
type WorkflowOutput struct {
	Name   string   `json:"name" yaml:"name"`
	Type   string   `json:"type" yaml:"type"`
	Values []string `json:"values,omitempty" yaml:"values"` // for enum types
}

// WorkflowAction defines an action to take on workflow completion
type WorkflowAction struct {
	Action      string `json:"action" yaml:"action"`
	Channel     string `json:"channel,omitempty" yaml:"channel"`
	MaxAttempts int    `json:"max_attempts,omitempty" yaml:"max_attempts"`
	Backoff     string `json:"backoff,omitempty" yaml:"backoff"`
}

// WorkflowRun represents a single execution of a workflow
type WorkflowRun struct {
	ID          uuid.UUID              `json:"id" db:"id"`
	TemplateID  string                 `json:"template_id" db:"template_id"`
	Status      WorkflowStatus         `json:"status" db:"status"`
	Inputs      map[string]interface{} `json:"inputs" db:"-"`
	Outputs     map[string]interface{} `json:"outputs" db:"-"`
	Error       string                 `json:"error,omitempty" db:"error"`
	StartedAt   *time.Time             `json:"started_at,omitempty" db:"started_at"`
	CompletedAt *time.Time             `json:"completed_at,omitempty" db:"completed_at"`
	CreatedAt   time.Time              `json:"created_at" db:"created_at"`
	Steps       []StepExecution        `json:"steps,omitempty" db:"-"`
}

// StepExecution represents the execution of a single workflow step
type StepExecution struct {
	ID          uuid.UUID  `json:"id" db:"id"`
	RunID       uuid.UUID  `json:"run_id" db:"run_id"`
	StepID      string     `json:"step_id" db:"step_id"`
	Role        RoleName   `json:"role" db:"role"`
	Status      StepStatus `json:"status" db:"status"`
	Prompt      string     `json:"prompt,omitempty" db:"prompt"`
	Response    string     `json:"response,omitempty" db:"response"`
	TokensUsed  int        `json:"tokens_used" db:"tokens_used"`
	CostUSD     float64    `json:"cost_usd" db:"cost_usd"`
	LatencyMs   int        `json:"latency_ms" db:"latency_ms"`
	Error       string     `json:"error,omitempty" db:"error"`
	StartedAt   *time.Time `json:"started_at,omitempty" db:"started_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty" db:"completed_at"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
}

// Artifact represents a generated artifact (image, text, file)
type Artifact struct {
	ID        uuid.UUID              `json:"id" db:"id"`
	RunID     uuid.UUID              `json:"run_id" db:"run_id"`
	StepID    string                 `json:"step_id" db:"step_id"`
	Type      string                 `json:"type" db:"type"` // image, text, json, file
	Name      string                 `json:"name" db:"name"`
	Path      string                 `json:"path,omitempty" db:"path"`
	URL       string                 `json:"url,omitempty" db:"url"`
	Metadata  map[string]interface{} `json:"metadata,omitempty" db:"-"`
	CreatedAt time.Time              `json:"created_at" db:"created_at"`
}

// Role represents a configured AI role/employee
type Role struct {
	Name     RoleName `json:"name"`
	Label    string   `json:"label"`
	Provider string   `json:"provider"`
	Persona  string   `json:"persona"`
	Model    string   `json:"model,omitempty"`
}

// ProviderConfig represents an LLM provider configuration
type ProviderConfig struct {
	Name    string `json:"name"`
	Type    string `json:"type"` // openai, anthropic, google, openai_compatible
	URL     string `json:"url,omitempty"`
	Model   string `json:"model"`
	APIKey  string `json:"api_key,omitempty"`
	Enabled bool   `json:"enabled"`
}

// ComponentHealth represents health of a single component
type ComponentHealth struct {
	Status    string `json:"status"`
	LatencyMs int64  `json:"latency_ms,omitempty"`
	Error     string `json:"error,omitempty"`
}

// HealthResponse represents the health check response
type HealthResponse struct {
	Status        string                     `json:"status"`
	Mode          string                     `json:"mode"`
	Service       string                     `json:"service"`
	Version       string                     `json:"version"`
	UptimeSeconds int64                      `json:"uptime_seconds"`
	Checks        map[string]ComponentHealth `json:"checks"`
	ActiveRuns    int                        `json:"active_runs"`
	QueueDepth    int                        `json:"queue_depth"`
}

// LLMRequest represents a request to an LLM provider
type LLMRequest struct {
	Messages    []LLMMessage `json:"messages"`
	MaxTokens   int          `json:"max_tokens,omitempty"`
	Temperature float64      `json:"temperature,omitempty"`
	Model       string       `json:"model,omitempty"`
}

// LLMMessage represents a chat message
type LLMMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// LLMResponse represents a response from an LLM provider
type LLMResponse struct {
	Content      string `json:"content"`
	Model        string `json:"model"`
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
	LatencyMs    int    `json:"latency_ms"`
}

// WebSocketMessage represents a message sent over WebSocket
type WebSocketMessage struct {
	Type    string      `json:"type"` // run_update, step_update, error
	RunID   string      `json:"run_id,omitempty"`
	StepID  string      `json:"step_id,omitempty"`
	Payload interface{} `json:"payload"`
}

// RunCreateRequest represents a request to start a workflow
type RunCreateRequest struct {
	TemplateID string                 `json:"template_id"`
	Inputs     map[string]interface{} `json:"inputs"`
}

// ErrorResponse represents an API error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Code    string `json:"code,omitempty"`
	Details string `json:"details,omitempty"`
}
