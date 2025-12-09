package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOrchestratorRenderTemplate(t *testing.T) {
	cfg, _ := LoadConfig("/nonexistent/config.ini")
	o := &Orchestrator{config: cfg}

	tests := []struct {
		name     string
		template string
		context  map[string]interface{}
		expected string
		hasError bool
	}{
		{
			name:     "simple variable",
			template: "Hello {{.name}}",
			context:  map[string]interface{}{"name": "World"},
			expected: "Hello World",
			hasError: false,
		},
		{
			name:     "multiple variables",
			template: "{{.greeting}} {{.name}}!",
			context:  map[string]interface{}{"greeting": "Hi", "name": "User"},
			expected: "Hi User!",
			hasError: false,
		},
		{
			name:     "nested access",
			template: "Domain: {{.inputs.domain}}",
			context: map[string]interface{}{
				"inputs": map[string]interface{}{"domain": "fintech"},
			},
			expected: "Domain: fintech",
			hasError: false,
		},
		{
			name:     "missing variable",
			template: "Hello {{.missing}}",
			context:  map[string]interface{}{},
			expected: "Hello <no value>",
			hasError: false,
		},
		{
			name:     "invalid template",
			template: "Hello {{.name",
			context:  map[string]interface{}{},
			expected: "",
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := o.renderTemplate(tt.template, tt.context)

			if tt.hasError {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestOrchestratorEvaluateCondition(t *testing.T) {
	cfg, _ := LoadConfig("/nonexistent/config.ini")
	o := &Orchestrator{config: cfg}

	tests := []struct {
		name      string
		condition string
		context   map[string]interface{}
		expected  bool
	}{
		{
			name:      "approve decision",
			condition: "{{.decision}}",
			context:   map[string]interface{}{"decision": "approve"},
			expected:  true,
		},
		{
			name:      "reject decision",
			condition: "{{.decision}}",
			context:   map[string]interface{}{"decision": "reject"},
			expected:  false,
		},
		{
			name:      "empty value",
			condition: "{{.decision}}",
			context:   map[string]interface{}{"decision": ""},
			expected:  false,
		},
		{
			name:      "false string",
			condition: "{{.flag}}",
			context:   map[string]interface{}{"flag": "false"},
			expected:  false,
		},
		{
			name:      "zero value",
			condition: "{{.count}}",
			context:   map[string]interface{}{"count": "0"},
			expected:  false,
		},
		{
			name:      "truthy value",
			condition: "{{.flag}}",
			context:   map[string]interface{}{"flag": "yes"},
			expected:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := o.evaluateCondition(tt.condition, tt.context)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestOrchestratorLoadTemplates(t *testing.T) {
	// Create a temporary directory with test workflows
	tmpDir, err := os.MkdirTemp("", "workflows-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a test workflow file
	workflowContent := `
name: Test Workflow
description: A test workflow
version: "1.0"

inputs:
  - name: domain
    type: string
    required: true

steps:
  - id: step1
    name: First Step
    role: board
    action: generate
    prompt_template: |
      Generate ideas for {{.domain}}
`
	workflowPath := filepath.Join(tmpDir, "test_workflow.yaml")
	if err := os.WriteFile(workflowPath, []byte(workflowContent), 0644); err != nil {
		t.Fatalf("Failed to write workflow file: %v", err)
	}

	// Create orchestrator with temp directory
	cfg, _ := LoadConfig("/nonexistent/config.ini")
	cfg.WorkflowsDir = tmpDir

	o := &Orchestrator{
		config:    cfg,
		templates: make(map[string]*WorkflowTemplate),
	}

	if err := o.LoadTemplates(); err != nil {
		t.Fatalf("LoadTemplates failed: %v", err)
	}

	// Check that template was loaded
	templates := o.GetTemplates()
	if len(templates) != 1 {
		t.Errorf("Expected 1 template, got %d", len(templates))
	}

	// Check template by ID
	template, ok := o.GetTemplate("test_workflow")
	if !ok {
		t.Fatal("Expected test_workflow template to exist")
	}

	if template.Name != "Test Workflow" {
		t.Errorf("Expected name 'Test Workflow', got %s", template.Name)
	}
	if template.Version != "1.0" {
		t.Errorf("Expected version '1.0', got %s", template.Version)
	}
	if len(template.Inputs) != 1 {
		t.Errorf("Expected 1 input, got %d", len(template.Inputs))
	}
	if len(template.Steps) != 1 {
		t.Errorf("Expected 1 step, got %d", len(template.Steps))
	}
	if template.Steps[0].Role != RoleBoard {
		t.Errorf("Expected step role 'board', got %s", template.Steps[0].Role)
	}
}

func TestOrchestratorLoadTemplatesEmptyDir(t *testing.T) {
	// Create an empty temporary directory
	tmpDir, err := os.MkdirTemp("", "workflows-empty-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg, _ := LoadConfig("/nonexistent/config.ini")
	cfg.WorkflowsDir = tmpDir

	o := &Orchestrator{
		config:    cfg,
		templates: make(map[string]*WorkflowTemplate),
	}

	if err := o.LoadTemplates(); err != nil {
		t.Fatalf("LoadTemplates failed: %v", err)
	}

	if len(o.GetTemplates()) != 0 {
		t.Error("Expected 0 templates from empty directory")
	}
}

func TestOrchestratorLoadTemplatesNonexistentDir(t *testing.T) {
	cfg, _ := LoadConfig("/nonexistent/config.ini")
	cfg.WorkflowsDir = "/nonexistent/workflows"

	o := &Orchestrator{
		config:    cfg,
		templates: make(map[string]*WorkflowTemplate),
	}

	// Should not error, just log warning
	if err := o.LoadTemplates(); err != nil {
		t.Fatalf("LoadTemplates should not error on nonexistent dir: %v", err)
	}
}

func TestOrchestratorLoadTemplatesInvalidYAML(t *testing.T) {
	// Create a temporary directory with invalid workflow
	tmpDir, err := os.MkdirTemp("", "workflows-invalid-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create an invalid workflow file
	invalidContent := `
name: Invalid
description: [this is not valid yaml
version: {{{{
`
	workflowPath := filepath.Join(tmpDir, "invalid.yaml")
	if err := os.WriteFile(workflowPath, []byte(invalidContent), 0644); err != nil {
		t.Fatalf("Failed to write workflow file: %v", err)
	}

	cfg, _ := LoadConfig("/nonexistent/config.ini")
	cfg.WorkflowsDir = tmpDir

	o := &Orchestrator{
		config:    cfg,
		templates: make(map[string]*WorkflowTemplate),
	}

	// Should not error, just skip invalid files
	if err := o.LoadTemplates(); err != nil {
		t.Fatalf("LoadTemplates should not error on invalid files: %v", err)
	}

	// No templates should be loaded
	if len(o.GetTemplates()) != 0 {
		t.Error("Expected 0 templates when all are invalid")
	}
}

func TestOrchestratorGetTemplateNotFound(t *testing.T) {
	cfg, _ := LoadConfig("/nonexistent/config.ini")
	o := &Orchestrator{
		config:    cfg,
		templates: make(map[string]*WorkflowTemplate),
	}

	_, ok := o.GetTemplate("nonexistent")
	if ok {
		t.Error("Expected GetTemplate to return false for nonexistent template")
	}
}

func TestWorkflowStepDependencies(t *testing.T) {
	workflowContent := `
name: Multi-step Workflow
description: Workflow with dependencies
version: "1.0"

steps:
  - id: step1
    name: First Step
    role: board
    action: generate
    prompt_template: Generate ideas

  - id: step2
    name: Second Step
    role: ceo
    action: decide
    depends_on: [step1]
    prompt_template: Review {{.step1.response}}

  - id: step3
    name: Third Step
    role: cto
    action: analyze
    depends_on: [step1, step2]
    prompt_template: Analyze {{.step2.response}}
`
	tmpDir, err := os.MkdirTemp("", "workflows-deps-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	workflowPath := filepath.Join(tmpDir, "deps.yaml")
	if err := os.WriteFile(workflowPath, []byte(workflowContent), 0644); err != nil {
		t.Fatalf("Failed to write workflow file: %v", err)
	}

	cfg, _ := LoadConfig("/nonexistent/config.ini")
	cfg.WorkflowsDir = tmpDir

	o := &Orchestrator{
		config:    cfg,
		templates: make(map[string]*WorkflowTemplate),
	}

	if err := o.LoadTemplates(); err != nil {
		t.Fatalf("LoadTemplates failed: %v", err)
	}

	template, ok := o.GetTemplate("deps")
	if !ok {
		t.Fatal("Expected deps template to exist")
	}

	if len(template.Steps) != 3 {
		t.Errorf("Expected 3 steps, got %d", len(template.Steps))
	}

	// Check dependencies
	if len(template.Steps[1].DependsOn) != 1 {
		t.Errorf("Expected step2 to have 1 dependency, got %d", len(template.Steps[1].DependsOn))
	}
	if template.Steps[1].DependsOn[0] != "step1" {
		t.Errorf("Expected step2 dependency 'step1', got %s", template.Steps[1].DependsOn[0])
	}

	if len(template.Steps[2].DependsOn) != 2 {
		t.Errorf("Expected step3 to have 2 dependencies, got %d", len(template.Steps[2].DependsOn))
	}
}
