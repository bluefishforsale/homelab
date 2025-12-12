package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

// Handlers holds HTTP handlers
type Handlers struct {
	app *App
}

// NewHandlers creates a new handlers instance
func NewHandlers(app *App) *Handlers {
	return &Handlers{app: app}
}

// HealthHandler handles health check requests
func (h *Handlers) HealthHandler(w http.ResponseWriter, r *http.Request) {
	checks := make(map[string]ComponentHealth)
	downCount := 0

	// Check PostgreSQL
	pgStart := time.Now()
	if err := h.app.db.Ping(); err != nil {
		checks["postgresql"] = ComponentHealth{Status: "down", Error: err.Error()}
		healthStatus.WithLabelValues("postgresql").Set(0)
		downCount++
	} else {
		checks["postgresql"] = ComponentHealth{Status: "up", LatencyMs: time.Since(pgStart).Milliseconds()}
		healthStatus.WithLabelValues("postgresql").Set(1)
	}

	// Check Redis
	redisStart := time.Now()
	if err := h.app.redis.Ping(); err != nil {
		checks["redis"] = ComponentHealth{Status: "down", Error: err.Error()}
		healthStatus.WithLabelValues("redis").Set(0)
		downCount++
	} else {
		checks["redis"] = ComponentHealth{Status: "up", LatencyMs: time.Since(redisStart).Milliseconds()}
		healthStatus.WithLabelValues("redis").Set(1)
	}

	// Check default LLM provider
	provider, err := h.app.providers.GetProvider(h.app.config.DefaultProvider)
	if err != nil || !provider.IsAvailable() {
		checks["llm"] = ComponentHealth{Status: "down", Error: "provider not available"}
		healthStatus.WithLabelValues("llm").Set(0)
	} else {
		checks["llm"] = ComponentHealth{Status: "up"}
		healthStatus.WithLabelValues("llm").Set(1)
	}

	// Get queue depth
	queueSize, _ := h.app.redis.GetQueueDepth()
	queueDepth.Set(float64(queueSize))

	// Get active runs
	activeCount, _ := h.app.db.GetActiveRunsCount()
	activeWorkflows.Set(float64(activeCount))

	// Determine status
	status := "healthy"
	mode := "full"
	if downCount > 0 {
		status = "degraded"
		mode = "limited"
	}
	if downCount >= 2 {
		status = "unhealthy"
		mode = "minimal"
	}

	response := HealthResponse{
		Status:        status,
		Mode:          mode,
		Service:       "ai-corp",
		Version:       "0.1.0",
		UptimeSeconds: int64(time.Since(h.app.startTime).Seconds()),
		Checks:        checks,
		ActiveRuns:    activeCount,
		QueueDepth:    int(queueSize),
	}

	w.Header().Set("Content-Type", "application/json")
	if status == "unhealthy" {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	json.NewEncoder(w).Encode(response)
}

// ListWorkflowsHandler returns available workflow templates
func (h *Handlers) ListWorkflowsHandler(w http.ResponseWriter, r *http.Request) {
	templates := h.app.orchestrator.GetTemplates()

	response := map[string]interface{}{
		"workflows": templates,
		"total":     len(templates),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GetWorkflowHandler returns a specific workflow template
func (h *Handlers) GetWorkflowHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	template, ok := h.app.orchestrator.GetTemplate(id)
	if !ok {
		http.Error(w, "Workflow not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(template)
}

// StartRunHandler starts a new workflow run
func (h *Handlers) StartRunHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	templateID := vars["id"]

	var req struct {
		Inputs map[string]interface{} `json:"inputs"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	run, err := h.app.orchestrator.StartRun(templateID, req.Inputs)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(run)
}

// ListRunsHandler returns workflow runs
func (h *Handlers) ListRunsHandler(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	limitStr := r.URL.Query().Get("limit")

	limit := 50
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 200 {
			limit = l
		}
	}

	runs, err := h.app.db.GetWorkflowRuns(status, limit)
	if err != nil {
		writeError(w, "Database error", http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"runs":  runs,
		"total": len(runs),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GetRunHandler returns a specific workflow run
func (h *Handlers) GetRunHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]

	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, "Invalid run ID", http.StatusBadRequest)
		return
	}

	run, err := h.app.db.GetWorkflowRun(id)
	if err != nil {
		writeError(w, "Database error", http.StatusInternalServerError)
		return
	}
	if run == nil {
		writeError(w, "Run not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(run)
}

// CancelRunHandler cancels a running workflow
func (h *Handlers) CancelRunHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]

	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, "Invalid run ID", http.StatusBadRequest)
		return
	}

	if err := h.app.orchestrator.CancelRun(id); err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "cancelled",
	})
}

// GetArtifactsHandler returns artifacts for a run
func (h *Handlers) GetArtifactsHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]

	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, "Invalid run ID", http.StatusBadRequest)
		return
	}

	artifacts, err := h.app.db.GetArtifacts(id)
	if err != nil {
		writeError(w, "Database error", http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"artifacts": artifacts,
		"total":     len(artifacts),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// ListRolesHandler returns configured roles
func (h *Handlers) ListRolesHandler(w http.ResponseWriter, r *http.Request) {
	roles := make([]Role, 0)
	for _, role := range h.app.config.Roles {
		roles = append(roles, role)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"roles": roles,
		"total": len(roles),
	})
}

// ListProvidersHandler returns configured providers
func (h *Handlers) ListProvidersHandler(w http.ResponseWriter, r *http.Request) {
	providers := h.app.providers.ListProviders()

	// Check availability
	result := make([]map[string]interface{}, 0)
	for _, p := range providers {
		provider, _ := h.app.providers.GetProvider(p.Name)
		available := false
		if provider != nil {
			available = provider.IsAvailable()
		}

		result = append(result, map[string]interface{}{
			"name":      p.Name,
			"type":      p.Type,
			"model":     p.Model,
			"available": available,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"providers": result,
		"total":     len(result),
	})
}

// TestProviderHandler tests a provider connection
func (h *Handlers) TestProviderHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["id"]

	err := h.app.providers.TestProvider(name)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok",
	})
}

// ReloadConfigHandler reloads configuration
func (h *Handlers) ReloadConfigHandler(w http.ResponseWriter, r *http.Request) {
	// Reload workflow templates
	if err := h.app.orchestrator.LoadTemplates(); err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"message": "Configuration reloaded",
	})
}

// MidjourneyWebhookHandler handles Midjourney callbacks
func (h *Handlers) MidjourneyWebhookHandler(w http.ResponseWriter, r *http.Request) {
	var payload map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	log.WithField("payload", payload).Info("Received Midjourney webhook")

	// TODO: Process Midjourney callback, update artifacts

	w.WriteHeader(http.StatusOK)
}

// DownloadArtifactHandler serves artifact file data
func (h *Handlers) DownloadArtifactHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	runIDStr := vars["run_id"]
	artifactIDStr := vars["artifact_id"]

	runID, err := uuid.Parse(runIDStr)
	if err != nil {
		writeError(w, "Invalid run ID", http.StatusBadRequest)
		return
	}

	artifactID, err := uuid.Parse(artifactIDStr)
	if err != nil {
		writeError(w, "Invalid artifact ID", http.StatusBadRequest)
		return
	}

	// Get artifacts for run
	artifacts, err := h.app.db.GetArtifacts(runID)
	if err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Find specific artifact
	var artifact *Artifact
	for _, a := range artifacts {
		if a.ID == artifactID {
			artifact = &a
			break
		}
	}

	if artifact == nil {
		writeError(w, "Artifact not found", http.StatusNotFound)
		return
	}

	// Get artifact data from storage
	data, err := h.app.orchestrator.GetArtifactData(artifact)
	if err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Set content type
	contentType := "application/octet-stream"
	switch artifact.Type {
	case "image":
		contentType = "image/png"
	case "text":
		contentType = "text/plain"
	case "json":
		contentType = "application/json"
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", artifact.Name))
	w.Write(data)
}

// StorageStatusHandler returns storage backend status
func (h *Handlers) StorageStatusHandler(w http.ResponseWriter, r *http.Request) {
	if h.app.storage == nil {
		writeError(w, "Storage not configured", http.StatusServiceUnavailable)
		return
	}

	backends := h.app.storage.List()
	result := make([]map[string]interface{}, 0)

	for _, b := range backends {
		result = append(result, map[string]interface{}{
			"name":      b.Name(),
			"type":      string(b.Type()),
			"available": b.IsAvailable(),
		})
	}

	primary := h.app.storage.Primary()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"primary":  primary.Name(),
		"backends": result,
	})
}

// ListBoardMembersHandler returns all board members
func (h *Handlers) ListBoardMembersHandler(w http.ResponseWriter, r *http.Request) {
	if h.app.board == nil {
		writeError(w, "Board not initialized", http.StatusServiceUnavailable)
		return
	}

	members := h.app.board.GetAllMembers()

	result := make([]map[string]interface{}, 0, len(members))
	for _, m := range members {
		result = append(result, map[string]interface{}{
			"id":          m.ID,
			"name":        m.Name,
			"title":       m.Title,
			"background":  m.Background,
			"expertise":   m.Expertise,
			"concerns":    m.Concerns,
			"priorities":  m.Priorities,
			"voting_style": m.VotingStyle,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"members":        result,
		"total":          len(result),
		"votes_required": VotesRequiredToPass,
		"majority_pct":   VoteMajorityPct,
	})
}

// BoardVoteRequest represents a vote request
type BoardVoteRequest struct {
	Type        string `json:"type"`        // project_approval, ceo_veto, project_cancel
	Subject     string `json:"subject"`
	Description string `json:"description"`
	ProposedBy  string `json:"proposed_by"`
}

// BoardVoteHandler conducts a board vote
func (h *Handlers) BoardVoteHandler(w http.ResponseWriter, r *http.Request) {
	if h.app.board == nil {
		writeError(w, "Board not initialized", http.StatusServiceUnavailable)
		return
	}

	var req BoardVoteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Subject == "" || req.Description == "" {
		writeError(w, "Subject and description are required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	var decision *BoardDecision
	var err error

	switch req.Type {
	case "project_approval":
		decision, err = h.app.board.ApproveProject(ctx, req.Subject, req.Description, req.ProposedBy)
	case "ceo_veto":
		decision, err = h.app.board.VetoCEODecision(ctx, req.Subject, req.Description)
	case "project_cancel":
		decision, err = h.app.board.CancelProject(ctx, req.Subject, req.Description)
	default:
		writeError(w, "Invalid vote type", http.StatusBadRequest)
		return
	}

	if err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(decision)
}

// ListScheduledMeetingsHandler returns scheduled meetings from scheduler
func (h *Handlers) ListScheduledMeetingsHandler(w http.ResponseWriter, r *http.Request) {
	// Return scheduled board meetings from scheduler
	if h.app.scheduler == nil {
		writeError(w, "Scheduler not initialized", http.StatusServiceUnavailable)
		return
	}

	tasks := h.app.scheduler.GetTasks()
	meetings := make([]map[string]interface{}, 0)

	for _, t := range tasks {
		if t.Type == TaskBoardMeeting {
			meetings = append(meetings, map[string]interface{}{
				"id":       t.ID,
				"name":     t.Name,
				"schedule": t.Schedule,
				"next_run": t.NextRun,
				"last_run": t.LastRun,
				"enabled":  t.Enabled,
				"config":   t.Config,
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"meetings": meetings,
		"total":    len(meetings),
	})
}

// TriggerMeetingHandler manually triggers a board meeting
func (h *Handlers) TriggerMeetingHandler(w http.ResponseWriter, r *http.Request) {
	if h.app.scheduler == nil {
		writeError(w, "Scheduler not initialized", http.StatusServiceUnavailable)
		return
	}

	// Find a board meeting task and trigger it
	tasks := h.app.scheduler.GetTasks()
	for _, t := range tasks {
		if t.Type == TaskBoardMeeting {
			if err := h.app.scheduler.TriggerTask(t.ID); err != nil {
				writeError(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status":  "triggered",
				"task_id": t.ID,
				"message": "Board meeting triggered",
			})
			return
		}
	}

	writeError(w, "No board meeting task found", http.StatusNotFound)
}

// ListScheduledTasksHandler returns all scheduled tasks
func (h *Handlers) ListScheduledTasksHandler(w http.ResponseWriter, r *http.Request) {
	if h.app.scheduler == nil {
		writeError(w, "Scheduler not initialized", http.StatusServiceUnavailable)
		return
	}

	tasks := h.app.scheduler.GetTasks()
	result := make([]map[string]interface{}, 0, len(tasks))

	for _, t := range tasks {
		result = append(result, map[string]interface{}{
			"id":        t.ID,
			"name":      t.Name,
			"type":      t.Type,
			"schedule":  t.Schedule,
			"next_run":  t.NextRun,
			"last_run":  t.LastRun,
			"enabled":   t.Enabled,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"tasks": result,
		"total": len(result),
	})
}

// TriggerTaskHandler manually triggers a scheduled task
func (h *Handlers) TriggerTaskHandler(w http.ResponseWriter, r *http.Request) {
	if h.app.scheduler == nil {
		writeError(w, "Scheduler not initialized", http.StatusServiceUnavailable)
		return
	}

	vars := mux.Vars(r)
	idStr := vars["id"]

	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, "Invalid task ID", http.StatusBadRequest)
		return
	}

	if err := h.app.scheduler.TriggerTask(id); err != nil {
		writeError(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "triggered",
		"task_id": id,
	})
}

// OrgStatsHandler returns organization statistics
func (h *Handlers) OrgStatsHandler(w http.ResponseWriter, r *http.Request) {
	if h.app.org == nil {
		writeError(w, "Organization not initialized", http.StatusServiceUnavailable)
		return
	}

	stats := h.app.org.GetStats()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// ListDivisionsHandler returns all divisions
func (h *Handlers) ListDivisionsHandler(w http.ResponseWriter, r *http.Request) {
	if h.app.org == nil {
		writeError(w, "Organization not initialized", http.StatusServiceUnavailable)
		return
	}

	h.app.org.mu.RLock()
	divisions := make([]map[string]interface{}, 0, len(h.app.org.Divisions))
	for _, div := range h.app.org.Divisions {
		divisions = append(divisions, map[string]interface{}{
			"id":          div.ID,
			"name":        div.Name,
			"description": div.Description,
			"departments": len(div.Heads),
			"created_at":  div.CreatedAt,
		})
	}
	h.app.org.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"divisions": divisions,
		"total":     len(divisions),
	})
}

// CreateDivisionRequest represents a request to create a division
type CreateDivisionRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// CreateDivisionHandler creates a new division
func (h *Handlers) CreateDivisionHandler(w http.ResponseWriter, r *http.Request) {
	if h.app.org == nil {
		writeError(w, "Organization not initialized", http.StatusServiceUnavailable)
		return
	}

	var req CreateDivisionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		writeError(w, "Name is required", http.StatusBadRequest)
		return
	}

	div := h.app.org.CreateDivision(req.Name, req.Description)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":          div.ID,
		"name":        div.Name,
		"description": div.Description,
		"created_at":  div.CreatedAt,
	})
}

// ListEmployeesHandler returns all employees
// Uses TryRLock to avoid blocking on employee locks
func (h *Handlers) ListEmployeesHandler(w http.ResponseWriter, r *http.Request) {
	if h.app.org == nil {
		writeError(w, "Organization not initialized", http.StatusServiceUnavailable)
		return
	}

	// Filter by skill if provided
	skillFilter := r.URL.Query().Get("skill")
	statusFilter := r.URL.Query().Get("status")

	// Build employee list while holding org lock, using TryRLock to avoid blocking
	h.app.org.mu.RLock()
	employees := make([]map[string]interface{}, 0, len(h.app.org.AllEmployees))
	for _, emp := range h.app.org.AllEmployees {
		// Use TryRLock to avoid indefinite blocking
		if !emp.mu.TryRLock() {
			// Skip employees with held locks - they're likely being modified
			continue
		}
		
		// Apply filters
		if skillFilter != "" && string(emp.Skill) != skillFilter {
			emp.mu.RUnlock()
			continue
		}
		if statusFilter != "" && string(emp.Status) != statusFilter {
			emp.mu.RUnlock()
			continue
		}

		empData := map[string]interface{}{
			"id":         emp.ID,
			"name":       emp.Name,
			"skill":      emp.Skill,
			"status":     emp.Status,
			"manager_id": emp.ManagerID,
			"work_count": emp.workCount,
		}
		// Include current work title if working
		if emp.CurrentWork != nil {
			empData["current_work"] = emp.CurrentWork.Title
		}
		employees = append(employees, empData)
		emp.mu.RUnlock()
	}
	h.app.org.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"employees": employees,
		"total":     len(employees),
	})
}

// DownloadPipelineHandler returns HTML for a pipeline (printable to PDF)
func (h *Handlers) DownloadPipelineHandler(w http.ResponseWriter, r *http.Request) {
	if h.app.org == nil || h.app.org.pipeline == nil {
		writeError(w, "Organization not initialized", http.StatusServiceUnavailable)
		return
	}

	vars := mux.Vars(r)
	idStr := vars["id"]
	
	pipelineID, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, "Invalid pipeline ID", http.StatusBadRequest)
		return
	}
	
	html, err := h.app.org.pipeline.GenerateHTML(pipelineID)
	if err != nil {
		writeError(w, err.Error(), http.StatusNotFound)
		return
	}
	
	// Check if download is requested, otherwise preview in browser
	if r.URL.Query().Get("download") == "true" {
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s-execution-plan.html\"", idStr[:8]))
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

// ListPipelinesHandler returns all product pipelines
func (h *Handlers) ListPipelinesHandler(w http.ResponseWriter, r *http.Request) {
	if h.app.org == nil || h.app.org.pipeline == nil {
		writeError(w, "Organization not initialized", http.StatusServiceUnavailable)
		return
	}

	pipelines := h.app.org.pipeline.ListPipelines()
	
	// Count by stage
	stageCounts := map[string]int{
		"ideation":       0,
		"work_packet":    0,
		"csuite_review":  0,
		"board_vote":     0,
		"execution_plan": 0,
		"production":     0,
		"final_review":   0,
		"launched":       0,
		"rejected":       0,
	}
	
	result := make([]map[string]interface{}, 0, len(pipelines))
	for _, p := range pipelines {
		stageCounts[string(p.Stage)]++
		
		item := map[string]interface{}{
			"id":             p.ID,
			"name":           p.Name,
			"description":    p.Description,
			"category":       p.Category,
			"stage":          p.Stage,
			"target_market":  p.TargetMarket,
			"revision_count": p.RevisionCount,
			"created_at":     p.CreatedAt,
			"updated_at":     p.UpdatedAt,
		}
		
		if p.Idea != nil {
			item["idea"] = p.Idea
		}
		if p.WorkPacket != nil {
			item["work_packet"] = p.WorkPacket
		}
		if p.CsuiteReview != nil {
			item["csuite_review"] = p.CsuiteReview
		}
		if p.BoardDecision != nil {
			item["board_decision"] = p.BoardDecision
		}
		if p.ExecutionPlan != nil {
			item["execution_plan"] = p.ExecutionPlan
		}
		
		result = append(result, item)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"pipelines": result,
		"total":     len(result),
		"by_stage":  stageCounts,
	})
}

// ListProductsHandler returns all product/service ideas
func (h *Handlers) ListProductsHandler(w http.ResponseWriter, r *http.Request) {
	if h.app.org == nil {
		writeError(w, "Organization not initialized", http.StatusServiceUnavailable)
		return
	}

	statusFilter := r.URL.Query().Get("status")

	h.app.org.mu.RLock()
	products := make([]map[string]interface{}, 0)
	
	// Count by status
	statusCounts := map[string]int{
		"ideation":    0,
		"planning":    0,
		"development": 0,
		"review":      0,
		"approved":    0,
		"launched":    0,
		"rejected":    0,
	}
	
	for _, p := range h.app.org.Products {
		statusCounts[string(p.Status)]++
		
		if statusFilter != "" && string(p.Status) != statusFilter {
			continue
		}

		products = append(products, map[string]interface{}{
			"id":              p.ID,
			"name":            p.Name,
			"description":     p.Description,
			"category":        p.Category,
			"status":          p.Status,
			"target_market":   p.TargetMarket,
			"value_prop":      p.ValueProp,
			"features":        p.Features,
			"created_at":      p.CreatedAt,
			"updated_at":      p.UpdatedAt,
			"deliverables":    p.Deliverables,
		})
	}
	h.app.org.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"products":  products,
		"total":     len(products),
		"by_status": statusCounts,
	})
}

// ListDeliverablesHandler returns all deliverables/work products
func (h *Handlers) ListDeliverablesHandler(w http.ResponseWriter, r *http.Request) {
	if h.app.org == nil {
		writeError(w, "Organization not initialized", http.StatusServiceUnavailable)
		return
	}

	// Filter by status if provided
	statusFilter := r.URL.Query().Get("status")
	skillFilter := r.URL.Query().Get("skill")

	h.app.org.deliverablesMu.RLock()
	deliverables := make([]map[string]interface{}, 0)
	
	// Count by status for summary
	statusCounts := map[string]int{
		"completed":   0,
		"in_progress": 0,
		"in_review":   0,
		"approved":    0,
		"rejected":    0,
	}
	
	for _, d := range h.app.org.Deliverables {
		statusCounts[string(d.Status)]++
		
		// Apply filters
		if statusFilter != "" && string(d.Status) != statusFilter {
			continue
		}
		if skillFilter != "" && d.Skill != skillFilter {
			continue
		}

		deliverable := map[string]interface{}{
			"id":            d.ID,
			"title":         d.Title,
			"type":          d.Type,
			"description":   d.Description,
			"output":        d.Output,
			"status":        d.Status,
			"employee_id":   d.EmployeeID,
			"employee_name": d.EmployeeName,
			"skill":         d.Skill,
			"created_at":    d.CreatedAt,
			"completed_at":  d.CompletedAt,
			"duration_ms":   d.Duration.Milliseconds(),
		}
		if d.ReviewerID != nil {
			deliverable["reviewer_id"] = d.ReviewerID
		}
		if d.ReviewNotes != "" {
			deliverable["review_notes"] = d.ReviewNotes
		}
		deliverables = append(deliverables, deliverable)
	}
	h.app.org.deliverablesMu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"deliverables": deliverables,
		"total":        len(deliverables),
		"by_status":    statusCounts,
	})
}

// GetEmployeeHandler returns a specific employee
func (h *Handlers) GetEmployeeHandler(w http.ResponseWriter, r *http.Request) {
	if h.app.org == nil {
		writeError(w, "Organization not initialized", http.StatusServiceUnavailable)
		return
	}

	vars := mux.Vars(r)
	idStr := vars["id"]

	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, "Invalid employee ID", http.StatusBadRequest)
		return
	}

	h.app.org.mu.RLock()
	emp, ok := h.app.org.AllEmployees[id]
	h.app.org.mu.RUnlock()

	if !ok {
		writeError(w, "Employee not found", http.StatusNotFound)
		return
	}

	emp.mu.RLock()
	result := map[string]interface{}{
		"id":           emp.ID,
		"name":         emp.Name,
		"skill":        emp.Skill,
		"status":       emp.Status,
		"manager_id":   emp.ManagerID,
		"work_count":   emp.workCount,
		"current_work": emp.CurrentWork,
	}
	emp.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// ListManagersHandler returns all managers
func (h *Handlers) ListManagersHandler(w http.ResponseWriter, r *http.Request) {
	if h.app.org == nil {
		writeError(w, "Organization not initialized", http.StatusServiceUnavailable)
		return
	}

	// Copy manager list while holding org lock
	h.app.org.mu.RLock()
	mgrList := make([]*Manager, 0, len(h.app.org.AllManagers))
	for _, mgr := range h.app.org.AllManagers {
		mgrList = append(mgrList, mgr)
	}
	h.app.org.mu.RUnlock()

	// Now iterate without holding org lock (prevents deadlock)
	managers := make([]map[string]interface{}, 0, len(mgrList))
	for _, mgr := range mgrList {
		mgr.mu.RLock()
		managers = append(managers, map[string]interface{}{
			"id":             mgr.ID,
			"name":           mgr.Name,
			"specialty":      mgr.Specialty,
			"employee_count": len(mgr.Employees),
			"max_reports":    mgr.MaxReports,
		})
		mgr.mu.RUnlock()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"managers": managers,
		"total":    len(managers),
	})
}

// AssignWorkRequest represents a work assignment request
type AssignWorkRequest struct {
	Skill       string                 `json:"skill"`
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	Objectives  []string               `json:"objectives"`
	Priority    int                    `json:"priority"`
	Inputs      map[string]interface{} `json:"inputs"`
}

// AssignWorkHandler assigns work to an available employee
func (h *Handlers) AssignWorkHandler(w http.ResponseWriter, r *http.Request) {
	if h.app.org == nil {
		writeError(w, "Organization not initialized", http.StatusServiceUnavailable)
		return
	}

	var req AssignWorkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Skill == "" || req.Title == "" {
		writeError(w, "Skill and title are required", http.StatusBadRequest)
		return
	}

	work := &WorkItem{
		ID:          uuid.New(),
		Type:        "assigned",
		Title:       req.Title,
		Description: req.Description,
		Objectives:  req.Objectives,
		Priority:    req.Priority,
		Inputs:      req.Inputs,
		CreatedAt:   time.Now(),
	}

	if err := h.app.org.AssignWork(EmployeeSkill(req.Skill), work); err != nil {
		writeError(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":      "assigned",
		"work_id":     work.ID,
		"assigned_to": work.AssignedTo,
	})
}

// CompanyStatusHandler returns the current company status (cached for instant response)
func (h *Handlers) CompanyStatusHandler(w http.ResponseWriter, r *http.Request) {
	if h.app.org == nil {
		writeError(w, "Organization not initialized", http.StatusServiceUnavailable)
		return
	}

	// Try cache first for instant response
	cacheKey := "company_status"
	if cached, found := h.app.cache.Get(cacheKey); found {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Cache", "HIT")
		w.Header().Set("Cache-Control", "public, max-age=2")
		json.NewEncoder(w).Encode(cached)
		return
	}

	// Cache miss - get fresh data
	status := h.app.org.GetStatus()
	stats := h.app.org.GetStats()
	
	response := map[string]interface{}{
		"status": status,
		"stats":  stats,
	}
	
	// Cache for 2 seconds
	h.app.cache.Set(cacheKey, response, 2*time.Second)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Cache", "MISS")
	w.Header().Set("Cache-Control", "public, max-age=2")
	json.NewEncoder(w).Encode(response)
}

// PauseCompanyHandler pauses the company
func (h *Handlers) PauseCompanyHandler(w http.ResponseWriter, r *http.Request) {
	if h.app.org == nil {
		writeError(w, "Organization not initialized", http.StatusServiceUnavailable)
		return
	}

	h.app.org.Pause()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "paused",
		"message": "Company operations paused",
	})
}

// ResumeCompanyHandler resumes the company
func (h *Handlers) ResumeCompanyHandler(w http.ResponseWriter, r *http.Request) {
	if h.app.org == nil {
		writeError(w, "Organization not initialized", http.StatusServiceUnavailable)
		return
	}

	h.app.org.Resume()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "running",
		"message": "Company operations resumed",
	})
}

// ListBiographiesHandler returns all biographies
func (h *Handlers) ListBiographiesHandler(w http.ResponseWriter, r *http.Request) {
	if h.app.org == nil {
		writeError(w, "Organization not initialized", http.StatusServiceUnavailable)
		return
	}

	bios := h.app.org.GetAllBiographies()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"biographies": bios,
		"total":       len(bios),
	})
}

// GetBiographyHandler returns a specific biography
func (h *Handlers) GetBiographyHandler(w http.ResponseWriter, r *http.Request) {
	if h.app.org == nil {
		writeError(w, "Organization not initialized", http.StatusServiceUnavailable)
		return
	}

	vars := mux.Vars(r)
	idStr := vars["person_id"]

	personID, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, "Invalid person ID", http.StatusBadRequest)
		return
	}

	bio, ok := h.app.org.GetBiography(personID)
	if !ok {
		// Return empty biography template
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"person_id":   personID,
			"exists":      false,
			"name":        "",
			"bio":         "",
			"background":  "",
			"personality": "",
			"goals":       []string{},
			"values":      []string{},
			"quirks":      []string{},
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(bio)
}

// BiographyRequest represents a biography update request
type BiographyRequest struct {
	PersonID    string   `json:"person_id"`
	PersonType  string   `json:"person_type"`
	Name        string   `json:"name"`
	Bio         string   `json:"bio"`
	Background  string   `json:"background"`
	Personality string   `json:"personality"`
	Goals       []string `json:"goals"`
	Values      []string `json:"values"`
	Quirks      []string `json:"quirks"`
}

// UpdateBiographyHandler creates or updates a biography
func (h *Handlers) UpdateBiographyHandler(w http.ResponseWriter, r *http.Request) {
	if h.app.org == nil {
		writeError(w, "Organization not initialized", http.StatusServiceUnavailable)
		return
	}

	var req BiographyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	personID, err := uuid.Parse(req.PersonID)
	if err != nil {
		writeError(w, "Invalid person ID", http.StatusBadRequest)
		return
	}

	bio := &Biography{
		PersonID:    personID,
		PersonType:  req.PersonType,
		Name:        req.Name,
		Bio:         req.Bio,
		Background:  req.Background,
		Personality: req.Personality,
		Goals:       req.Goals,
		Values:      req.Values,
		Quirks:      req.Quirks,
	}

	if err := h.app.org.SetBiography(bio); err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "updated",
		"person_id": personID,
		"message":   "Biography updated and persona refreshed",
	})
}

// DeleteBiographyHandler deletes a biography
func (h *Handlers) DeleteBiographyHandler(w http.ResponseWriter, r *http.Request) {
	if h.app.org == nil {
		writeError(w, "Organization not initialized", http.StatusServiceUnavailable)
		return
	}

	vars := mux.Vars(r)
	idStr := vars["person_id"]

	personID, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, "Invalid person ID", http.StatusBadRequest)
		return
	}

	h.app.org.DeleteBiography(personID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "deleted",
		"person_id": personID,
	})
}

// GetSectorsHandler returns all available business sectors
func (h *Handlers) GetSectorsHandler(w http.ResponseWriter, r *http.Request) {
	sectors := GetAvailableSectors()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"sectors": sectors,
		"total":   len(sectors),
	})
}

// GetSeedHandler returns the current company seed
func (h *Handlers) GetSeedHandler(w http.ResponseWriter, r *http.Request) {
	if h.app.org == nil {
		writeError(w, "Organization not initialized", http.StatusServiceUnavailable)
		return
	}

	seed := h.app.org.GetSeed()
	if seed == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"seeded": false,
			"seed":   nil,
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"seeded": true,
		"seed":   seed,
	})
}

// SetSeedRequest represents a request to seed the company
type SetSeedRequest struct {
	Sector       string   `json:"sector"`
	CustomSector string   `json:"custom_sector,omitempty"`
	CompanyName  string   `json:"company_name"`
	TargetMarket string   `json:"target_market"`
	Mission      string   `json:"mission,omitempty"`
	Vision       string   `json:"vision,omitempty"`
	Goals        []string `json:"goals,omitempty"`
	Constraints  []string `json:"constraints,omitempty"`
	InitialBudget float64 `json:"initial_budget,omitempty"`
}

// SetSeedHandler sets the company seed
func (h *Handlers) SetSeedHandler(w http.ResponseWriter, r *http.Request) {
	if h.app.org == nil {
		writeError(w, "Organization not initialized", http.StatusServiceUnavailable)
		return
	}

	var req SetSeedRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Sector == "" {
		writeError(w, "Sector is required", http.StatusBadRequest)
		return
	}

	if req.CompanyName == "" {
		writeError(w, "Company name is required", http.StatusBadRequest)
		return
	}

	seed := &CompanySeed{
		Sector:        BusinessSector(req.Sector),
		CustomSector:  req.CustomSector,
		CompanyName:   req.CompanyName,
		TargetMarket:  req.TargetMarket,
		Mission:       req.Mission,
		Vision:        req.Vision,
		Goals:         req.Goals,
		Constraints:   req.Constraints,
		InitialBudget: req.InitialBudget,
	}

	if err := h.app.org.SetSeed(seed); err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "seeded",
		"seed":    h.app.org.GetSeed(),
		"message": "Company successfully bootstrapped with business sector",
	})
}

// GetRestructuringHistoryHandler returns restructuring proposals
func (h *Handlers) GetRestructuringHistoryHandler(w http.ResponseWriter, r *http.Request) {
	if h.app.org == nil {
		writeError(w, "Organization not initialized", http.StatusServiceUnavailable)
		return
	}

	history := h.app.org.GetRestructuringHistory()
	if history == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"proposals": []interface{}{},
			"total":     0,
		})
		return
	}

	// Get limit from query params
	limitStr := r.URL.Query().Get("limit")
	limit := 20
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	proposals := history.GetRecent(limit)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"proposals": proposals,
		"total":     len(history.Proposals),
	})
}

// GetPendingRestructuringHandler returns pending restructuring proposals
func (h *Handlers) GetPendingRestructuringHandler(w http.ResponseWriter, r *http.Request) {
	if h.app.org == nil {
		writeError(w, "Organization not initialized", http.StatusServiceUnavailable)
		return
	}

	history := h.app.org.GetRestructuringHistory()
	pending := history.GetPending()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"proposals": pending,
		"total":     len(pending),
	})
}

// ProposeRestructuringRequest represents a restructuring proposal request
type ProposeRestructuringRequest struct {
	Type          string                 `json:"type"`
	Title         string                 `json:"title"`
	Rationale     string                 `json:"rationale"`
	ProposerName  string                 `json:"proposer_name"`
	ProposerRole  string                 `json:"proposer_role"`
	Parameters    map[string]interface{} `json:"parameters"`
}

// ProposeRestructuringHandler creates a new restructuring proposal
func (h *Handlers) ProposeRestructuringHandler(w http.ResponseWriter, r *http.Request) {
	if h.app.org == nil {
		writeError(w, "Organization not initialized", http.StatusServiceUnavailable)
		return
	}

	var req ProposeRestructuringRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Type == "" || req.Title == "" {
		writeError(w, "Type and title are required", http.StatusBadRequest)
		return
	}

	proposerID := uuid.New() // In production, this would come from auth
	if req.ProposerName == "" {
		req.ProposerName = "Executive"
	}
	if req.ProposerRole == "" {
		req.ProposerRole = "CEO"
	}

	proposal, err := h.app.org.ProposeRestructuring(
		proposerID,
		req.ProposerName,
		req.ProposerRole,
		RestructuringType(req.Type),
		req.Title,
		req.Rationale,
		req.Parameters,
	)

	if err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(proposal)
}

// VoteRestructuringRequest represents a vote on a restructuring proposal
type VoteRestructuringRequest struct {
	Approve bool   `json:"approve"`
	Comment string `json:"comment,omitempty"`
}

// VoteRestructuringHandler allows voting on a restructuring proposal
func (h *Handlers) VoteRestructuringHandler(w http.ResponseWriter, r *http.Request) {
	if h.app.org == nil {
		writeError(w, "Organization not initialized", http.StatusServiceUnavailable)
		return
	}

	vars := mux.Vars(r)
	proposalIDStr := vars["id"]
	proposalID, err := uuid.Parse(proposalIDStr)
	if err != nil {
		writeError(w, "Invalid proposal ID", http.StatusBadRequest)
		return
	}

	var req VoteRestructuringRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	voterID := uuid.New() // In production, this would come from auth

	if err := h.app.org.VoteOnRestructuring(proposalID, voterID, req.Approve, req.Comment); err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Get updated proposal
	proposal := h.app.org.GetRestructuringHistory().Get(proposalID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(proposal)
}

// ExecuteRestructuringHandler executes an approved restructuring proposal
func (h *Handlers) ExecuteRestructuringHandler(w http.ResponseWriter, r *http.Request) {
	if h.app.org == nil {
		writeError(w, "Organization not initialized", http.StatusServiceUnavailable)
		return
	}

	vars := mux.Vars(r)
	proposalIDStr := vars["id"]
	proposalID, err := uuid.Parse(proposalIDStr)
	if err != nil {
		writeError(w, "Invalid proposal ID", http.StatusBadRequest)
		return
	}

	if err := h.app.org.ExecuteRestructuring(proposalID); err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Get updated proposal
	proposal := h.app.org.GetRestructuringHistory().Get(proposalID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":   "executed",
		"proposal": proposal,
	})
}

// ResetOrganizationHandler destroys and recreates the organization
func (h *Handlers) ResetOrganizationHandler(w http.ResponseWriter, r *http.Request) {
	if h.app.org == nil {
		writeError(w, "Organization not initialized", http.StatusServiceUnavailable)
		return
	}

	// Stop the organization first
	h.app.org.Pause()

	// Create a new organization (this replaces the old one)
	h.app.org = NewOrganization(h.app.config, h.app.providers, h.app.storage, h.app.db)

	log.Info("Organization has been reset to initial state")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "reset",
		"message": "Organization has been destroyed and recreated",
		"stats":   h.app.org.GetStats(),
	})
}

// GetSystemStatusHandler returns overall system status for admin panel
func (h *Handlers) GetSystemStatusHandler(w http.ResponseWriter, r *http.Request) {
	status := map[string]interface{}{
		"database":    h.app.db != nil,
		"redis":       h.app.redis != nil,
		"storage":     h.app.storage != nil,
		"providers":   h.app.providers != nil,
		"organization": h.app.org != nil,
	}

	// Get provider info
	if h.app.providers != nil {
		providers := h.app.providers.ListProviders()
		status["provider_count"] = len(providers)
		status["default_provider"] = h.app.config.DefaultProvider
	}

	// Get org status
	if h.app.org != nil {
		status["org_status"] = h.app.org.GetStatus()
		status["org_stats"] = h.app.org.GetStats()
		status["seeded"] = h.app.org.IsSeeded()
		if h.app.org.GetSeed() != nil {
			status["seed"] = h.app.org.GetSeed()
		}
	}

	// Config info
	status["config"] = map[string]interface{}{
		"server_port":     h.app.config.Port,
		"workflow_dir":    h.app.config.WorkflowsDir,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// AnalyzeOrgHealthHandler analyzes organization health and restructuring needs
func (h *Handlers) AnalyzeOrgHealthHandler(w http.ResponseWriter, r *http.Request) {
	if h.app.org == nil {
		writeError(w, "Organization not initialized", http.StatusServiceUnavailable)
		return
	}

	analysis, err := h.app.org.AnalyzeOrganizationHealth()
	if err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(analysis)
}

// GetEmployeeDetailHandler returns comprehensive employee info with work history and activity log
func (h *Handlers) GetEmployeeDetailHandler(w http.ResponseWriter, r *http.Request) {
	if h.app.org == nil {
		writeError(w, "Organization not initialized", http.StatusServiceUnavailable)
		return
	}

	vars := mux.Vars(r)
	employeeID, err := uuid.Parse(vars["id"])
	if err != nil {
		writeError(w, "Invalid employee ID", http.StatusBadRequest)
		return
	}

	h.app.org.mu.RLock()
	emp, exists := h.app.org.AllEmployees[employeeID]
	h.app.org.mu.RUnlock()

	if !exists {
		writeError(w, "Employee not found", http.StatusNotFound)
		return
	}

	emp.mu.RLock()
	defer emp.mu.RUnlock()

	// Calculate time on current task
	var timeOnTask *string
	if emp.CurrentWorkStarted != nil && emp.CurrentWork != nil {
		duration := time.Since(*emp.CurrentWorkStarted)
		durationStr := formatDuration(duration)
		timeOnTask = &durationStr
	}

	// Find manager info
	var managerInfo map[string]interface{}
	if manager, ok := h.app.org.AllManagers[emp.ManagerID]; ok {
		managerInfo = map[string]interface{}{
			"id":   manager.ID.String(),
			"name": manager.Name,
		}
	}

	// Build response
	result := map[string]interface{}{
		"id":             emp.ID.String(),
		"name":           emp.Name,
		"skill":          string(emp.Skill),
		"status":         string(emp.Status),
		"hired_at":       emp.HiredAt,
		"persona":        emp.Persona,
		"manager":        managerInfo,
		"work_count":     atomic.LoadInt64(&emp.workCount),
		"current_work":   emp.CurrentWork,
		"time_on_task":   timeOnTask,
		"activity_log":   emp.ActivityLog,
		"work_history":   emp.WorkHistory,
	}

	// Calculate stats
	completedCount := len(emp.WorkHistory)
	var totalDuration time.Duration
	for _, wr := range emp.WorkHistory {
		totalDuration += wr.Duration
	}
	avgDuration := time.Duration(0)
	if completedCount > 0 {
		avgDuration = totalDuration / time.Duration(completedCount)
	}

	result["stats"] = map[string]interface{}{
		"completed_tasks":    completedCount,
		"total_work_time":    formatDuration(totalDuration),
		"average_task_time":  formatDuration(avgDuration),
		"activity_count":     len(emp.ActivityLog),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// formatDuration formats a duration in human-readable format
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	} else if d < time.Hour {
		return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
	} else if d < 24*time.Hour {
		return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	return fmt.Sprintf("%dd %dh", days, hours)
}

// GetPersonDetailHandler returns detailed info about a person including boss, reports, expectations
func (h *Handlers) GetPersonDetailHandler(w http.ResponseWriter, r *http.Request) {
	if h.app.org == nil {
		writeError(w, "Organization not initialized", http.StatusServiceUnavailable)
		return
	}

	vars := mux.Vars(r)
	id := vars["id"]
	
	// Check if it's a board member first (they use string IDs like "marketing", not UUIDs)
	if h.app.board != nil {
		for _, member := range h.app.board.GetAllMembers() {
			if string(member.ID) == id {
				result := map[string]interface{}{
					"id":             id,
					"name":           member.Name,
					"title":          member.Title,
					"type":           "board_member",
					"expectations":   []string{
						"Provide strategic guidance to the company",
						"Review and vote on major decisions",
						"Ensure company alignment with mission and vision",
						"Represent shareholder interests",
					},
					"boss":           nil, // Board members don't report to anyone
					"direct_reports": []map[string]string{}, // Board has no direct reports
					"background":     member.Background,
					"expertise":      member.Expertise,
					"voting_style":   member.VotingStyle,
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(result)
				return
			}
		}
	}
	
	// For employees/managers/executives, parse as UUID
	personID, err := uuid.Parse(id)
	if err != nil {
		writeError(w, "Invalid person ID", http.StatusBadRequest)
		return
	}

	h.app.org.mu.RLock()
	defer h.app.org.mu.RUnlock()

	result := map[string]interface{}{
		"id":             personID.String(),
		"name":           "",
		"title":          "",
		"type":           "",
		"boss":           nil,
		"direct_reports": []map[string]string{},
		"expectations":   []string{},
	}

	// Check if it's the CEO
	if h.app.org.CEO != nil && h.app.org.CEO.ID == personID {
		result["name"] = h.app.org.CEO.Name
		result["title"] = h.app.org.CEO.Title
		result["type"] = "department_head"
		result["expectations"] = h.app.org.CEO.Objectives
		// CEO has no boss, but has division heads as reports
		reports := []map[string]string{}
		for _, div := range h.app.org.Divisions {
			for _, head := range div.Heads {
				reports = append(reports, map[string]string{
					"id":    head.ID.String(),
					"name":  head.Name,
					"title": head.Title,
				})
			}
		}
		result["direct_reports"] = reports
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
		return
	}

	// Check department heads
	for _, div := range h.app.org.Divisions {
		for _, head := range div.Heads {
			if head.ID == personID {
				result["name"] = head.Name
				result["title"] = head.Title
				result["type"] = "department_head"
				result["expectations"] = head.Objectives
				if h.app.org.CEO != nil {
					result["boss"] = map[string]string{
						"id":    h.app.org.CEO.ID.String(),
						"name":  h.app.org.CEO.Name,
						"title": h.app.org.CEO.Title,
					}
				}
				// Managers as reports
				reports := []map[string]string{}
				for _, mgr := range head.Managers {
					reports = append(reports, map[string]string{
						"id":    mgr.ID.String(),
						"name":  mgr.Name,
						"title": "Manager",
					})
				}
				result["direct_reports"] = reports
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(result)
				return
			}
		}
	}

	// Check managers
	for _, manager := range h.app.org.AllManagers {
		if manager.ID == personID {
			result["name"] = manager.Name
			result["title"] = "Manager"
			result["type"] = "manager"
			result["specialty"] = string(manager.Specialty)
			result["expectations"] = []string{
				"Lead and motivate team members",
				"Ensure project deadlines are met",
				"Maintain quality standards",
				"Report progress to leadership",
			}
			// Find boss (department head)
			for _, div := range h.app.org.Divisions {
				for _, head := range div.Heads {
					for _, mgr := range head.Managers {
						if mgr.ID == personID {
							result["boss"] = map[string]string{
								"id":    head.ID.String(),
								"name":  head.Name,
								"title": head.Title,
							}
							break
						}
					}
				}
			}
			// Employees as reports
			reports := []map[string]string{}
			for _, emp := range manager.Employees {
				reports = append(reports, map[string]string{
					"id":    emp.ID.String(),
					"name":  emp.Name,
					"title": string(emp.Skill),
				})
			}
			result["direct_reports"] = reports
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(result)
			return
		}
	}

	// Check employees
	for _, emp := range h.app.org.AllEmployees {
		if emp.ID == personID {
			// Load biography if exists
			bioData := make(map[string]interface{})
			if bio, exists := h.app.org.Biographies[emp.ID]; exists {
				bioData = map[string]interface{}{
					"bio":         bio.Bio,
					"background":  bio.Background,
					"personality": bio.Personality,
					"goals":       bio.Goals,
					"values":      bio.Values,
					"quirks":      bio.Quirks,
				}
			}
			
			result["name"] = emp.Name
			result["title"] = string(emp.Skill)
			result["type"] = "employee"
			result["status"] = string(emp.Status)
			result["biography"] = bioData
			result["expectations"] = []string{
				"Complete assigned tasks on time",
				"Maintain quality of work",
				"Collaborate with team members",
				"Communicate progress regularly",
			}
			// Find manager using ManagerID field
			if manager, exists := h.app.org.AllManagers[emp.ManagerID]; exists {
				result["boss"] = map[string]string{
					"id":    manager.ID.String(),
					"name":  manager.Name,
					"title": "Manager",
				}
			}
			// Employees have no direct reports
			result["direct_reports"] = []map[string]string{}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(result)
			return
		}
	}

	writeError(w, "Person not found", http.StatusNotFound)
}

// ListAllPeopleHandler returns all people in the organization (cached for instant response)
func (h *Handlers) ListAllPeopleHandler(w http.ResponseWriter, r *http.Request) {
	if h.app.org == nil {
		writeError(w, "Organization not initialized", http.StatusServiceUnavailable)
		return
	}

	// Try cache first for instant response
	cacheKey := "all_people"
	if cached, found := h.app.cache.Get(cacheKey); found {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Cache", "HIT")
		json.NewEncoder(w).Encode(cached)
		return
	}

	// Copy all lists while holding org lock to prevent deadlock
	h.app.org.mu.RLock()
	var ceo *DepartmentHead
	if h.app.org.CEO != nil {
		ceo = h.app.org.CEO
	}
	divisions := make([]*Division, 0, len(h.app.org.Divisions))
	for _, div := range h.app.org.Divisions {
		divisions = append(divisions, div)
	}
	employees := make([]*Employee, 0, len(h.app.org.AllEmployees))
	for _, emp := range h.app.org.AllEmployees {
		employees = append(employees, emp)
	}
	managers := make([]*Manager, 0, len(h.app.org.AllManagers))
	for _, mgr := range h.app.org.AllManagers {
		managers = append(managers, mgr)
	}
	h.app.org.mu.RUnlock()

	// Now iterate without holding org lock (prevents deadlock)
	people := make([]map[string]interface{}, 0)

	// Add CEO
	if ceo != nil {
		ceo.mu.RLock()
		people = append(people, map[string]interface{}{
			"id":   ceo.ID,
			"type": "ceo",
			"name": ceo.Name,
			"role": ceo.Title,
		})
		ceo.mu.RUnlock()
	}

	// Add department heads (executives)
	for _, division := range divisions {
		division.mu.RLock()
		heads := make([]*DepartmentHead, len(division.Heads))
		copy(heads, division.Heads)
		division.mu.RUnlock()
		
		for _, head := range heads {
			head.mu.RLock()
			people = append(people, map[string]interface{}{
				"id":   head.ID,
				"type": "executive",
				"name": head.Name,
				"role": head.Title,
			})
			head.mu.RUnlock()
		}
	}

	// Add employees
	for _, emp := range employees {
		emp.mu.RLock()
		people = append(people, map[string]interface{}{
			"id":     emp.ID,
			"type":   "employee",
			"name":   emp.Name,
			"role":   string(emp.Skill),
			"status": emp.Status,
		})
		emp.mu.RUnlock()
	}

	// Add managers
	for _, mgr := range managers {
		mgr.mu.RLock()
		people = append(people, map[string]interface{}{
			"id":   mgr.ID,
			"type": "manager",
			"name": mgr.Name,
			"role": string(mgr.Specialty) + " Manager",
		})
		mgr.mu.RUnlock()
	}

	// Add board members
	if h.app.board != nil {
		for _, member := range h.app.board.GetAllMembers() {
			people = append(people, map[string]interface{}{
				"id":    member.ID,
				"type":  "board_member",
				"name":  member.Name,
				"role":  member.Title,
				"style": member.VotingStyle,
			})
		}
	}

	response := map[string]interface{}{
		"people": people,
		"total":  len(people),
	}
	
	// Cache for 2 seconds
	h.app.cache.Set(cacheKey, response, 2*time.Second)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Cache", "MISS")
	w.Header().Set("Cache-Control", "public, max-age=2")
	w.Header().Set("ETag", fmt.Sprintf(`"%d"`, len(people)))
	json.NewEncoder(w).Encode(response)
}

// ListMeetingsHandler returns all meetings
func (h *Handlers) ListMeetingsHandler(w http.ResponseWriter, r *http.Request) {
	if h.app.board == nil {
		writeError(w, "Board not initialized", http.StatusServiceUnavailable)
		return
	}

	meetings := h.app.board.ListMeetings()

	// Build response with summary info
	result := make([]map[string]interface{}, 0, len(meetings))
	for _, m := range meetings {
		item := map[string]interface{}{
			"id":            m.ID.String(),
			"type":          m.Type,
			"title":         m.Title,
			"scheduled_at":  m.ScheduledAt,
			"status":        m.Status,
			"decision_count": len(m.Decisions),
			"dialog_count":  len(m.Dialog),
			"attendee_count": len(m.Attendees),
		}
		if m.StartedAt != nil {
			item["started_at"] = m.StartedAt
		}
		if m.EndedAt != nil {
			item["ended_at"] = m.EndedAt
		}
		if m.Summary != "" {
			item["summary"] = m.Summary
		}
		result = append(result, item)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"meetings": result,
		"total":    len(result),
	})
}

// GetMeetingHandler returns detailed meeting info with dialog and decisions
func (h *Handlers) GetMeetingHandler(w http.ResponseWriter, r *http.Request) {
	if h.app.board == nil {
		writeError(w, "Board not initialized", http.StatusServiceUnavailable)
		return
	}

	vars := mux.Vars(r)
	meetingID, err := uuid.Parse(vars["id"])
	if err != nil {
		writeError(w, "Invalid meeting ID", http.StatusBadRequest)
		return
	}

	meeting := h.app.board.GetMeeting(meetingID)
	if meeting == nil {
		writeError(w, "Meeting not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(meeting)
}

// LoggingMiddleware logs HTTP requests
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap response writer to capture status code
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(wrapped, r)

		duration := time.Since(start)

		// Update metrics
		httpRequests.WithLabelValues(r.Method, r.URL.Path, strconv.Itoa(wrapped.statusCode)).Inc()
		httpDuration.WithLabelValues(r.Method, r.URL.Path).Observe(duration.Seconds())

		// Log request
		log.WithFields(log.Fields{
			"method":   r.Method,
			"path":     r.URL.Path,
			"status":   wrapped.statusCode,
			"duration": duration,
		}).Debug("HTTP request")
	})
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Hijack implements http.Hijacker for WebSocket support
func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hijacker, ok := rw.ResponseWriter.(http.Hijacker); ok {
		return hijacker.Hijack()
	}
	return nil, nil, fmt.Errorf("response writer does not implement http.Hijacker")
}

// writeError writes a JSON error response
func writeError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(ErrorResponse{Error: message})
}

// CORSMiddleware adds CORS headers
func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}
