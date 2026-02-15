package main

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestWorkflowStatus(t *testing.T) {
	tests := []struct {
		status   WorkflowStatus
		expected string
	}{
		{StatusPending, "pending"},
		{StatusRunning, "running"},
		{StatusCompleted, "completed"},
		{StatusFailed, "failed"},
		{StatusCancelled, "cancelled"},
	}

	for _, tt := range tests {
		if string(tt.status) != tt.expected {
			t.Errorf("Expected %s, got %s", tt.expected, tt.status)
		}
	}
}

func TestStepStatus(t *testing.T) {
	tests := []struct {
		status   StepStatus
		expected string
	}{
		{StepPending, "pending"},
		{StepRunning, "running"},
		{StepCompleted, "completed"},
		{StepFailed, "failed"},
		{StepSkipped, "skipped"},
	}

	for _, tt := range tests {
		if string(tt.status) != tt.expected {
			t.Errorf("Expected %s, got %s", tt.expected, tt.status)
		}
	}
}

func TestRoleName(t *testing.T) {
	tests := []struct {
		role     RoleName
		expected string
	}{
		{RoleBoard, "board"},
		{RoleCEO, "ceo"},
		{RoleCTO, "cto"},
		{RoleMarketing, "marketing"},
		{RoleArtist, "artist"},
		{RoleWorker, "worker"},
	}

	for _, tt := range tests {
		if string(tt.role) != tt.expected {
			t.Errorf("Expected %s, got %s", tt.expected, tt.role)
		}
	}
}

func TestWorkflowRunJSON(t *testing.T) {
	now := time.Now()
	run := WorkflowRun{
		ID:         uuid.New(),
		TemplateID: "test-template",
		Status:     StatusRunning,
		Inputs:     map[string]interface{}{"domain": "fintech"},
		Outputs:    map[string]interface{}{},
		StartedAt:  &now,
		CreatedAt:  now,
	}

	// Test JSON marshaling
	data, err := json.Marshal(run)
	if err != nil {
		t.Fatalf("Failed to marshal WorkflowRun: %v", err)
	}

	// Test JSON unmarshaling
	var decoded WorkflowRun
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal WorkflowRun: %v", err)
	}

	if decoded.TemplateID != run.TemplateID {
		t.Errorf("Expected template_id %s, got %s", run.TemplateID, decoded.TemplateID)
	}
	if decoded.Status != run.Status {
		t.Errorf("Expected status %s, got %s", run.Status, decoded.Status)
	}
}

func TestStepExecutionJSON(t *testing.T) {
	now := time.Now()
	step := StepExecution{
		ID:         uuid.New(),
		RunID:      uuid.New(),
		StepID:     "ideation",
		Role:       RoleBoard,
		Status:     StepCompleted,
		Prompt:     "Generate ideas",
		Response:   "Here are ideas...",
		TokensUsed: 500,
		CostUSD:    0.01,
		LatencyMs:  1500,
		StartedAt:  &now,
		CreatedAt:  now,
	}

	data, err := json.Marshal(step)
	if err != nil {
		t.Fatalf("Failed to marshal StepExecution: %v", err)
	}

	var decoded StepExecution
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal StepExecution: %v", err)
	}

	if decoded.StepID != step.StepID {
		t.Errorf("Expected step_id %s, got %s", step.StepID, decoded.StepID)
	}
	if decoded.Role != step.Role {
		t.Errorf("Expected role %s, got %s", step.Role, decoded.Role)
	}
	if decoded.TokensUsed != step.TokensUsed {
		t.Errorf("Expected tokens_used %d, got %d", step.TokensUsed, decoded.TokensUsed)
	}
}

func TestLLMRequestJSON(t *testing.T) {
	req := LLMRequest{
		Messages: []LLMMessage{
			{Role: "system", Content: "You are a CEO"},
			{Role: "user", Content: "Make a decision"},
		},
		MaxTokens:   1000,
		Temperature: 0.7,
		Model:       "gpt-4",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal LLMRequest: %v", err)
	}

	var decoded LLMRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal LLMRequest: %v", err)
	}

	if len(decoded.Messages) != 2 {
		t.Errorf("Expected 2 messages, got %d", len(decoded.Messages))
	}
	if decoded.MaxTokens != 1000 {
		t.Errorf("Expected max_tokens 1000, got %d", decoded.MaxTokens)
	}
}

func TestHealthResponseJSON(t *testing.T) {
	resp := HealthResponse{
		Status:        "healthy",
		Mode:          "full",
		Service:       "ai-corp",
		Version:       "0.1.0",
		UptimeSeconds: 3600,
		Checks: map[string]ComponentHealth{
			"postgresql": {Status: "up", LatencyMs: 5},
			"redis":      {Status: "up", LatencyMs: 2},
			"llm":        {Status: "down", Error: "connection refused"},
		},
		ActiveRuns: 2,
		QueueDepth: 5,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Failed to marshal HealthResponse: %v", err)
	}

	var decoded HealthResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal HealthResponse: %v", err)
	}

	if decoded.Status != "healthy" {
		t.Errorf("Expected status 'healthy', got %s", decoded.Status)
	}
	if len(decoded.Checks) != 3 {
		t.Errorf("Expected 3 checks, got %d", len(decoded.Checks))
	}
	if decoded.Checks["llm"].Error != "connection refused" {
		t.Errorf("Expected llm error 'connection refused', got %s", decoded.Checks["llm"].Error)
	}
}

func TestWorkflowTemplateInputs(t *testing.T) {
	template := WorkflowTemplate{
		ID:          "test",
		Name:        "Test Workflow",
		Description: "A test workflow",
		Version:     "1.0",
		Inputs: []WorkflowInput{
			{Name: "domain", Type: "string", Required: true},
			{Name: "budget", Type: "string", Required: false, Default: "bootstrapped"},
		},
		Steps: []WorkflowStep{
			{
				ID:             "step1",
				Name:           "First Step",
				Role:           RoleBoard,
				Action:         "generate",
				PromptTemplate: "Generate ideas for {{.domain}}",
			},
		},
	}

	if len(template.Inputs) != 2 {
		t.Errorf("Expected 2 inputs, got %d", len(template.Inputs))
	}
	if template.Inputs[0].Name != "domain" {
		t.Errorf("Expected first input name 'domain', got %s", template.Inputs[0].Name)
	}
	if !template.Inputs[0].Required {
		t.Error("Expected domain input to be required")
	}
	if template.Inputs[1].Default != "bootstrapped" {
		t.Errorf("Expected budget default 'bootstrapped', got %s", template.Inputs[1].Default)
	}
}

func TestWebSocketMessage(t *testing.T) {
	msg := WebSocketMessage{
		Type:   "run_update",
		RunID:  uuid.New().String(),
		StepID: "ideation",
		Payload: map[string]interface{}{
			"status": "completed",
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal WebSocketMessage: %v", err)
	}

	var decoded WebSocketMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal WebSocketMessage: %v", err)
	}

	if decoded.Type != "run_update" {
		t.Errorf("Expected type 'run_update', got %s", decoded.Type)
	}
}
