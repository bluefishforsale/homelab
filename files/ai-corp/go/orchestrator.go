package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

// Orchestrator manages workflow execution
type Orchestrator struct {
	config    *Config
	db        *Database
	redis     *RedisClient
	providers *ProviderManager
	storage   *StorageManager
	templates map[string]*WorkflowTemplate
	mu        sync.RWMutex
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
}

// NewOrchestrator creates a new workflow orchestrator
func NewOrchestrator(config *Config, db *Database, redis *RedisClient, providers *ProviderManager, storage *StorageManager) *Orchestrator {
	ctx, cancel := context.WithCancel(context.Background())
	return &Orchestrator{
		config:    config,
		db:        db,
		redis:     redis,
		providers: providers,
		storage:   storage,
		templates: make(map[string]*WorkflowTemplate),
		ctx:       ctx,
		cancel:    cancel,
	}
}

// Start begins the orchestrator background processing
func (o *Orchestrator) Start() error {
	// Recover any jobs from unclean shutdown
	if o.redis != nil {
		recovered, err := o.redis.RecoverProcessingJobs()
		if err != nil {
			log.Warnf("Failed to recover processing jobs: %v", err)
		} else if recovered > 0 {
			log.Infof("Recovered %d jobs from previous session", recovered)
		}
	}

	// Load workflow templates
	if err := o.LoadTemplates(); err != nil {
		return fmt.Errorf("failed to load templates: %w", err)
	}

	// Start job processor
	o.wg.Add(1)
	go o.processJobs()

	log.Info("Orchestrator started")
	return nil
}

// Stop gracefully stops the orchestrator
func (o *Orchestrator) Stop() {
	o.cancel()
	o.wg.Wait()
	log.Info("Orchestrator stopped")
}

// LoadTemplates loads workflow templates from the templates directory
func (o *Orchestrator) LoadTemplates() error {
	o.mu.Lock()
	defer o.mu.Unlock()

	dir := o.config.WorkflowsDir
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		log.Warnf("Workflows directory not found: %s", dir)
		return nil
	}

	files, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return err
	}

	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			log.Warnf("Failed to read workflow file %s: %v", file, err)
			continue
		}

		var wf WorkflowTemplate
		if err := yaml.Unmarshal(data, &wf); err != nil {
			log.Warnf("Failed to parse workflow file %s: %v", file, err)
			continue
		}

		// Generate ID from filename
		wf.ID = strings.TrimSuffix(filepath.Base(file), ".yaml")
		wf.CreatedAt = time.Now()
		wf.UpdatedAt = time.Now()

		o.templates[wf.ID] = &wf

		// Save to database
		if err := o.db.SaveWorkflowTemplate(&wf); err != nil {
			log.Warnf("Failed to save workflow template %s: %v", wf.ID, err)
		}

		log.Infof("Loaded workflow template: %s (%s)", wf.ID, wf.Name)
	}

	log.Infof("Loaded %d workflow templates", len(o.templates))
	return nil
}

// GetTemplates returns all loaded templates
func (o *Orchestrator) GetTemplates() []*WorkflowTemplate {
	o.mu.RLock()
	defer o.mu.RUnlock()

	templates := make([]*WorkflowTemplate, 0, len(o.templates))
	for _, t := range o.templates {
		templates = append(templates, t)
	}
	return templates
}

// GetTemplate returns a specific template
func (o *Orchestrator) GetTemplate(id string) (*WorkflowTemplate, bool) {
	o.mu.RLock()
	defer o.mu.RUnlock()
	t, ok := o.templates[id]
	return t, ok
}

// StartRun initiates a new workflow run
func (o *Orchestrator) StartRun(templateID string, inputs map[string]interface{}) (*WorkflowRun, error) {
	template, ok := o.GetTemplate(templateID)
	if !ok {
		return nil, fmt.Errorf("template not found: %s", templateID)
	}

	// Check concurrent run limit
	activeCount, _ := o.db.GetActiveRunsCount()
	if activeCount >= o.config.MaxConcurrentWorkflows {
		return nil, fmt.Errorf("max concurrent workflows reached (%d)", o.config.MaxConcurrentWorkflows)
	}

	// Validate required inputs
	for _, input := range template.Inputs {
		if input.Required {
			if _, ok := inputs[input.Name]; !ok {
				if input.Default != "" {
					inputs[input.Name] = input.Default
				} else {
					return nil, fmt.Errorf("missing required input: %s", input.Name)
				}
			}
		}
	}

	// Create run record
	run := &WorkflowRun{
		ID:         uuid.New(),
		TemplateID: templateID,
		Status:     StatusPending,
		Inputs:     inputs,
		Outputs:    make(map[string]interface{}),
		CreatedAt:  time.Now(),
	}

	if err := o.db.CreateWorkflowRun(run); err != nil {
		return nil, fmt.Errorf("failed to create run: %w", err)
	}

	// Enqueue job
	job := JobPayload{
		RunID:      run.ID,
		TemplateID: templateID,
		Priority:   0,
		CreatedAt:  time.Now(),
	}

	if err := o.redis.EnqueueJob(job); err != nil {
		return nil, fmt.Errorf("failed to enqueue job: %w", err)
	}

	workflowsStarted.WithLabelValues(templateID).Inc()
	log.WithFields(log.Fields{
		"run_id":   run.ID,
		"template": templateID,
	}).Info("Workflow run started")

	return run, nil
}

// CancelRun cancels a running workflow
func (o *Orchestrator) CancelRun(runID uuid.UUID) error {
	run, err := o.db.GetWorkflowRun(runID)
	if err != nil {
		return err
	}
	if run == nil {
		return fmt.Errorf("run not found: %s", runID)
	}

	if run.Status != StatusRunning && run.Status != StatusPending {
		return fmt.Errorf("cannot cancel run with status: %s", run.Status)
	}

	now := time.Now()
	run.Status = StatusCancelled
	run.CompletedAt = &now
	run.Error = "Cancelled by user"

	if err := o.db.UpdateWorkflowRun(run); err != nil {
		return err
	}

	workflowsCompleted.WithLabelValues(run.TemplateID, string(StatusCancelled)).Inc()
	activeWorkflows.Dec()

	log.WithField("run_id", runID).Info("Workflow run cancelled")
	return nil
}

// processJobs is the background job processor
func (o *Orchestrator) processJobs() {
	defer o.wg.Done()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-o.ctx.Done():
			return
		case <-ticker.C:
			job, err := o.redis.DequeueJob()
			if err != nil {
				log.Warnf("Failed to dequeue job: %v", err)
				continue
			}
			if job == nil {
				continue
			}

			// Process job
			o.wg.Add(1)
			go func(j JobPayload) {
				defer o.wg.Done()
				o.executeWorkflow(j)
			}(*job)
		}
	}
}

// executeWorkflow executes a workflow run
func (o *Orchestrator) executeWorkflow(job JobPayload) {
	start := time.Now()

	run, err := o.db.GetWorkflowRun(job.RunID)
	if err != nil || run == nil {
		log.Errorf("Failed to get run %s: %v", job.RunID, err)
		o.redis.CompleteJob(job)
		return
	}

	template, ok := o.GetTemplate(job.TemplateID)
	if !ok {
		log.Errorf("Template not found: %s", job.TemplateID)
		o.redis.CompleteJob(job)
		return
	}

	// Update status to running
	now := time.Now()
	run.Status = StatusRunning
	run.StartedAt = &now
	o.db.UpdateWorkflowRun(run)
	activeWorkflows.Inc()

	// Execute steps
	context := run.Inputs
	if context == nil {
		context = make(map[string]interface{})
	}

	var lastError error
	completedSteps := make(map[string]bool)

	for _, step := range template.Steps {
		// Check if cancelled
		currentRun, _ := o.db.GetWorkflowRun(run.ID)
		if currentRun != nil && currentRun.Status == StatusCancelled {
			break
		}

		// Check dependencies
		depsOK := true
		for _, dep := range step.DependsOn {
			if !completedSteps[dep] {
				depsOK = false
				break
			}
		}
		if !depsOK {
			continue
		}

		// Check condition
		if step.Condition != "" {
			conditionMet, err := o.evaluateCondition(step.Condition, context)
			if err != nil {
				log.Warnf("Failed to evaluate condition for step %s: %v", step.ID, err)
			}
			if !conditionMet {
				completedSteps[step.ID] = true
				continue
			}
		}

		// Execute step
		result, err := o.executeStep(run.ID, &step, context)
		if err != nil {
			lastError = err
			log.WithFields(log.Fields{
				"run_id":  run.ID,
				"step_id": step.ID,
				"error":   err,
			}).Error("Step execution failed")
			break
		}

		// Update context with outputs
		if result != nil {
			context[step.ID] = result
			for k, v := range result {
				context[k] = v
			}
		}

		completedSteps[step.ID] = true
	}

	// Update run status
	completedAt := time.Now()
	run.CompletedAt = &completedAt
	run.Outputs = context

	if lastError != nil {
		run.Status = StatusFailed
		run.Error = lastError.Error()
	} else {
		run.Status = StatusCompleted
	}

	o.db.UpdateWorkflowRun(run)
	o.redis.CompleteJob(job)

	activeWorkflows.Dec()
	workflowsCompleted.WithLabelValues(template.ID, string(run.Status)).Inc()
	workflowDuration.WithLabelValues(template.ID).Observe(time.Since(start).Seconds())

	log.WithFields(log.Fields{
		"run_id":   run.ID,
		"status":   run.Status,
		"duration": time.Since(start),
	}).Info("Workflow execution completed")
}

// executeStep executes a single workflow step
func (o *Orchestrator) executeStep(runID uuid.UUID, step *WorkflowStep, stepContext map[string]interface{}) (map[string]interface{}, error) {
	start := time.Now()

	// Create step execution record
	stepExec := &StepExecution{
		ID:        uuid.New(),
		RunID:     runID,
		StepID:    step.ID,
		Role:      step.Role,
		Status:    StepRunning,
		StartedAt: &start,
		CreatedAt: start,
	}
	o.db.CreateStepExecution(stepExec)

	// Get role configuration
	roleConfig, ok := o.config.GetRole(step.Role)
	if !ok {
		return nil, fmt.Errorf("role not configured: %s", step.Role)
	}

	// Get provider
	provider, err := o.providers.GetProvider(roleConfig.Provider)
	if err != nil {
		// Try default provider
		provider, err = o.providers.GetProvider(o.config.DefaultProvider)
		if err != nil {
			return nil, fmt.Errorf("no provider available for role %s", step.Role)
		}
	}

	// Render prompt template
	prompt, err := o.renderTemplate(step.PromptTemplate, stepContext)
	if err != nil {
		return nil, fmt.Errorf("failed to render prompt: %w", err)
	}

	stepExec.Prompt = prompt

	// Build messages
	messages := []LLMMessage{
		{Role: "system", Content: roleConfig.Persona},
		{Role: "user", Content: prompt},
	}

	// Make LLM request
	ctx, cancel := context.WithTimeout(o.ctx, time.Duration(step.Timeout)*time.Second)
	if step.Timeout == 0 {
		ctx, cancel = context.WithTimeout(o.ctx, 120*time.Second)
	}
	defer cancel()

	resp, err := provider.Chat(ctx, LLMRequest{
		Messages:    messages,
		MaxTokens:   2000,
		Temperature: 0.7,
	})

	completedAt := time.Now()
	stepExec.CompletedAt = &completedAt
	stepExec.LatencyMs = int(time.Since(start).Milliseconds())

	if err != nil {
		stepExec.Status = StepFailed
		stepExec.Error = err.Error()
		o.db.UpdateStepExecution(stepExec)
		stepsExecuted.WithLabelValues(string(step.Role), "error").Inc()
		return nil, err
	}

	stepExec.Status = StepCompleted
	stepExec.Response = resp.Content
	stepExec.TokensUsed = resp.InputTokens + resp.OutputTokens
	o.db.UpdateStepExecution(stepExec)

	stepsExecuted.WithLabelValues(string(step.Role), "success").Inc()
	stepDuration.WithLabelValues(string(step.Role)).Observe(time.Since(start).Seconds())

	log.WithFields(log.Fields{
		"run_id":   runID,
		"step_id":  step.ID,
		"role":     step.Role,
		"tokens":   stepExec.TokensUsed,
		"latency":  stepExec.LatencyMs,
	}).Debug("Step completed")

	// Return response as output
	return map[string]interface{}{
		"response": resp.Content,
	}, nil
}

// renderTemplate renders a Go template with the given context
func (o *Orchestrator) renderTemplate(tmplStr string, ctx map[string]interface{}) (string, error) {
	tmpl, err := template.New("prompt").Parse(tmplStr)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ctx); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// evaluateCondition evaluates a simple condition string
func (o *Orchestrator) evaluateCondition(condition string, ctx map[string]interface{}) (bool, error) {
	// Simple equality check: "{{.decision}} == 'approve'"
	rendered, err := o.renderTemplate(condition, ctx)
	if err != nil {
		return false, err
	}

	// Very basic evaluation - just check if the rendered string looks truthy
	rendered = strings.TrimSpace(rendered)
	if rendered == "" || rendered == "false" || rendered == "0" || rendered == "reject" {
		return false, nil
	}

	return true, nil
}

// SaveArtifact saves an artifact to storage and database
func (o *Orchestrator) SaveArtifact(runID uuid.UUID, stepID, artifactType, name string, data []byte, metadata map[string]interface{}) (*Artifact, error) {
	artifact := &Artifact{
		ID:        uuid.New(),
		RunID:     runID,
		StepID:    stepID,
		Type:      artifactType,
		Name:      name,
		Metadata:  metadata,
		CreatedAt: time.Now(),
	}

	// Determine storage path
	storagePath := fmt.Sprintf("runs/%s/%s/%s", runID.String(), stepID, name)
	artifact.Path = storagePath

	// Store in storage backend
	if o.storage != nil {
		storage := o.storage.Primary()
		if storage != nil && storage.IsAvailable() {
			reader := bytes.NewReader(data)
			contentType := "application/octet-stream"
			switch artifactType {
			case "image":
				contentType = "image/png"
			case "text":
				contentType = "text/plain"
			case "json":
				contentType = "application/json"
			}

			if err := storage.Put(o.ctx, storagePath, reader, contentType); err != nil {
				log.Warnf("Failed to store artifact in storage: %v", err)
			} else {
				// Get URL for artifact
				url, _ := storage.GetURL(o.ctx, storagePath, 24*time.Hour)
				artifact.URL = url
			}
		}
	}

	// Save to database
	if err := o.db.CreateArtifact(artifact); err != nil {
		return nil, fmt.Errorf("failed to save artifact to database: %w", err)
	}

	log.WithFields(log.Fields{
		"artifact_id": artifact.ID,
		"run_id":      runID,
		"step_id":     stepID,
		"type":        artifactType,
		"name":        name,
	}).Debug("Artifact saved")

	return artifact, nil
}

// GetArtifactData retrieves artifact data from storage
func (o *Orchestrator) GetArtifactData(artifact *Artifact) ([]byte, error) {
	if artifact.Path == "" {
		return nil, fmt.Errorf("artifact has no storage path")
	}

	if o.storage == nil {
		return nil, fmt.Errorf("storage not configured")
	}

	storage := o.storage.Primary()
	if storage == nil || !storage.IsAvailable() {
		return nil, fmt.Errorf("storage not available")
	}

	reader, err := storage.Get(o.ctx, artifact.Path)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(reader); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
