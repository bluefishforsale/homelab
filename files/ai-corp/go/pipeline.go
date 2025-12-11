package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

// PipelineStage represents a stage in the product development pipeline
type PipelineStage string

const (
	StageIdeation         PipelineStage = "ideation"
	StageWorkPacket       PipelineStage = "work_packet"
	StageCsuiteReview     PipelineStage = "csuite_review"
	StageBoardVote        PipelineStage = "board_vote"
	StageExecutionPlan    PipelineStage = "execution_plan"
	StageProduction       PipelineStage = "production"
	StageFinalReview      PipelineStage = "final_review"
	StageLaunched         PipelineStage = "launched"
	StageRejected         PipelineStage = "rejected"
)

// ProductPipeline represents a product going through the development pipeline
type ProductPipeline struct {
	ID              uuid.UUID              `json:"id"`
	Name            string                 `json:"name"`
	Description     string                 `json:"description"`
	Category        string                 `json:"category"`
	Stage           PipelineStage          `json:"stage"`
	TargetMarket    string                 `json:"target_market"`
	
	// Content generated at each stage
	Idea            *IdeaContent           `json:"idea,omitempty"`
	WorkPacket      *WorkPacketContent     `json:"work_packet,omitempty"`
	CsuiteReview    *ReviewContent         `json:"csuite_review,omitempty"`
	BoardDecision   *PipelineBoardDecision `json:"board_decision,omitempty"`
	ExecutionPlan   *ExecutionPlanContent  `json:"execution_plan,omitempty"`
	FinalDeliverables []uuid.UUID          `json:"final_deliverables,omitempty"`
	
	// Tracking
	CreatedBy       uuid.UUID              `json:"created_by"`
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
	RevisionCount   int                    `json:"revision_count"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

// IdeaContent represents the initial C-Suite idea
type IdeaContent struct {
	Problem         string   `json:"problem"`
	Solution        string   `json:"solution"`
	ValueProp       string   `json:"value_proposition"`
	TargetCustomer  string   `json:"target_customer"`
	RevenueModel    string   `json:"revenue_model"`
	GeneratedBy     uuid.UUID `json:"generated_by"`
	GeneratedAt     time.Time `json:"generated_at"`
}

// WorkPacketContent represents the assembled work packet from employees
type WorkPacketContent struct {
	MarketResearch      string    `json:"market_research"`
	CompetitiveAnalysis string    `json:"competitive_analysis"`
	BusinessPlan        string    `json:"business_plan"`
	FinancialProjections string   `json:"financial_projections"`
	MarketingStrategy   string    `json:"marketing_strategy"`
	TechnicalOverview   string    `json:"technical_overview"`
	RiskAnalysis        string    `json:"risk_analysis"`
	AssembledAt         time.Time `json:"assembled_at"`
	Contributors        []uuid.UUID `json:"contributors"`
}

// ReviewContent represents a C-Suite review decision
type ReviewContent struct {
	Approved    bool      `json:"approved"`
	Feedback    string    `json:"feedback"`
	Concerns    []string  `json:"concerns,omitempty"`
	Suggestions []string  `json:"suggestions,omitempty"`
	ReviewerID  uuid.UUID `json:"reviewer_id"`
	ReviewedAt  time.Time `json:"reviewed_at"`
}

// PipelineBoardDecision represents the board's vote on a product in the pipeline
type PipelineBoardDecision struct {
	Approved     bool                `json:"approved"`
	VotesFor     int                 `json:"votes_for"`
	VotesAgainst int                 `json:"votes_against"`
	Abstentions  int                 `json:"abstentions"`
	Comments     map[string]string   `json:"comments,omitempty"` // member_id -> comment
	DecidedAt    time.Time           `json:"decided_at"`
}

// ExecutionPlanContent represents the execution plan from C-Suite
type ExecutionPlanContent struct {
	Phases          []ExecutionPhase `json:"phases"`
	Timeline        string           `json:"timeline"`
	Budget          string           `json:"budget"`
	TeamStructure   string           `json:"team_structure"`
	KPIs            []string         `json:"kpis"`
	Milestones      []string         `json:"milestones"`
	RiskMitigation  string           `json:"risk_mitigation"`
	CreatedBy       uuid.UUID        `json:"created_by"`
	CreatedAt       time.Time        `json:"created_at"`
}

// ExecutionPhase represents a phase in the execution plan
type ExecutionPhase struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Duration    string   `json:"duration"`
	Tasks       []string `json:"tasks"`
	Deliverables []string `json:"deliverables"`
}

// PipelineManager manages product pipelines
type PipelineManager struct {
	pipelines    map[uuid.UUID]*ProductPipeline
	org          *Organization
	running      bool
	pipelineCount int
	mu           sync.RWMutex
}

// NewPipelineManager creates a new pipeline manager
func NewPipelineManager(org *Organization) *PipelineManager {
	return &PipelineManager{
		pipelines: make(map[uuid.UUID]*ProductPipeline),
		org:       org,
	}
}

// StartContinuousOperation begins the continuous idea generation loop
func (pm *PipelineManager) StartContinuousOperation() {
	pm.mu.Lock()
	if pm.running {
		pm.mu.Unlock()
		return
	}
	pm.running = true
	pm.mu.Unlock()
	
	log.Info("Starting continuous pipeline operation")
	
	go func() {
		for {
			// Check if we should stop
			pm.mu.RLock()
			running := pm.running
			pm.mu.RUnlock()
			
			if !running {
				log.Info("Pipeline operation stopped")
				return
			}
			
			// Check company status
			if pm.org.GetStatus() != CompanyRunning {
				time.Sleep(5 * time.Second)
				continue
			}
			
			// Check if we have capacity for a new pipeline
			activePipelines := pm.countActivePipelines()
			if activePipelines >= 5 { // Max 5 concurrent pipelines
				time.Sleep(10 * time.Second)
				continue
			}
			
			// Start a new pipeline
			seed := pm.org.GetSeed()
			if seed == nil {
				time.Sleep(5 * time.Second)
				continue
			}
			
			pm.mu.Lock()
			pm.pipelineCount++
			count := pm.pipelineCount
			pm.mu.Unlock()
			
			pipeline := pm.CreatePipeline(
				fmt.Sprintf("%s Product #%d", seed.CompanyName, count),
				fmt.Sprintf("Product idea #%d for %s sector", count, seed.Sector),
				string(seed.Sector),
				seed.TargetMarket,
				uuid.Nil,
			)
			
			log.WithField("pipeline_id", pipeline.ID).Info("Starting new pipeline")
			pm.StartPipeline(pipeline)
			
			// Wait before starting next one
			time.Sleep(15 * time.Second)
		}
	}()
}

// StopContinuousOperation stops the continuous loop
func (pm *PipelineManager) StopContinuousOperation() {
	pm.mu.Lock()
	pm.running = false
	pm.mu.Unlock()
	log.Info("Stopping continuous pipeline operation")
}

// countActivePipelines returns number of pipelines not yet launched/rejected
func (pm *PipelineManager) countActivePipelines() int {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	
	count := 0
	for _, p := range pm.pipelines {
		if p.Stage != StageLaunched && p.Stage != StageRejected {
			count++
		}
	}
	return count
}

// CreatePipeline creates a new product pipeline from a C-Suite idea
func (pm *PipelineManager) CreatePipeline(name, description, category, targetMarket string, createdBy uuid.UUID) *ProductPipeline {
	pipeline := &ProductPipeline{
		ID:           uuid.New(),
		Name:         name,
		Description:  description,
		Category:     category,
		Stage:        StageIdeation,
		TargetMarket: targetMarket,
		CreatedBy:    createdBy,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		Metadata:     make(map[string]interface{}),
	}
	
	pm.mu.Lock()
	pm.pipelines[pipeline.ID] = pipeline
	pm.mu.Unlock()
	
	log.WithFields(log.Fields{
		"pipeline_id": pipeline.ID,
		"name":        name,
	}).Info("Pipeline created")
	
	return pipeline
}

// GetPipeline returns a pipeline by ID
func (pm *PipelineManager) GetPipeline(id uuid.UUID) *ProductPipeline {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.pipelines[id]
}

// ListPipelines returns all pipelines
func (pm *PipelineManager) ListPipelines() []*ProductPipeline {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	
	result := make([]*ProductPipeline, 0, len(pm.pipelines))
	for _, p := range pm.pipelines {
		result = append(result, p)
	}
	return result
}

// getExistingIdeaSummaries returns summaries of existing product ideas
func (pm *PipelineManager) getExistingIdeaSummaries() []string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	
	summaries := []string{}
	for _, p := range pm.pipelines {
		if p.Idea != nil {
			summary := fmt.Sprintf("- %s: %s", p.Name, p.Idea.Solution)
			summaries = append(summaries, summary)
		}
	}
	return summaries
}

// GenerateCSuiteIdea uses LLM to generate an initial product idea
func (pm *PipelineManager) GenerateCSuiteIdea(ctx context.Context, pipeline *ProductPipeline) error {
	provider, err := pm.org.providers.GetProvider(pm.org.config.DefaultProvider)
	if err != nil || provider == nil {
		return fmt.Errorf("no provider available")
	}
	
	seed := pm.org.GetSeed()
	if seed == nil {
		return fmt.Errorf("company not seeded")
	}
	
	// Get existing product ideas to avoid duplicates
	existingIdeas := pm.getExistingIdeaSummaries()
	
	existingContext := ""
	if len(existingIdeas) > 0 {
		existingContext = fmt.Sprintf(`
IMPORTANT: We already have these products in development:
%s

Generate a DIFFERENT product idea that complements but does NOT overlap with existing products.`, strings.Join(existingIdeas, "\n"))
	}
	
	prompt := fmt.Sprintf(`You are the CEO of %s, a company in the %s sector targeting %s.

Company Mission: %s
Company Vision: %s
%s

Generate ONE innovative, bootstrappable product idea. Requirements:
- Solves a real urgent problem people will pay to fix
- Can be built by a small team without massive capital
- NO AI/ML/LLM features, NO blockchain/crypto/quantum
- Focus on B2B SaaS or niche vertical solutions

IMPORTANT: Output ONLY the structured response below. Do NOT include any thinking, reasoning, or explanation. Start directly with PROBLEM:

PROBLEM: [Who has this problem and why it matters - 1-2 sentences]
SOLUTION: [Your product name and what it does - 1-2 sentences]
VALUE_PROP: [Why customers will pay - 1 sentence]
TARGET_CUSTOMER: [Buyer persona: role, company size, industry - 1 sentence]
REVENUE_MODEL: [Pricing strategy - 1 sentence]`, 
		seed.CompanyName, seed.Sector, seed.TargetMarket, seed.Mission, seed.Vision, existingContext)
	
	resp, err := provider.Chat(ctx, LLMRequest{
		Messages: []LLMMessage{{Role: "user", Content: prompt}},
		MaxTokens: 1000,
		Temperature: 0.85,
	})
	if err != nil {
		log.WithError(err).Error("Failed to generate C-Suite idea")
		return err
	}
	
	// Log the actual response for debugging
	log.WithFields(log.Fields{
		"pipeline_id": pipeline.ID,
		"response": resp.Content,
		"length": len(resp.Content),
	}).Info("LLM response for C-Suite idea")
	
	// Parse response - handle multi-line values
	idea := &IdeaContent{
		GeneratedBy: pipeline.CreatedBy,
		GeneratedAt: time.Now(),
	}
	
	// Remove markdown formatting and thinking tags, split into lines
	content := resp.Content
	// Strip <think>...</think> blocks (local LLM reasoning)
	if idx := strings.Index(content, "</think>"); idx != -1 {
		content = content[idx+8:]
	}
	content = strings.ReplaceAll(content, "**", "")
	lines := strings.Split(content, "\n")
	
	var currentField string
	var currentValue strings.Builder
	
	for _, line := range lines {
		line = strings.TrimSpace(line)
		
		// Check if this is a new field header
		if strings.HasPrefix(line, "PROBLEM:") {
			// Save previous field if any
			if currentField != "" {
				assignField(idea, currentField, currentValue.String())
			}
			currentField = "PROBLEM"
			currentValue.Reset()
			currentValue.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "PROBLEM:")))
		} else if strings.HasPrefix(line, "SOLUTION:") {
			if currentField != "" {
				assignField(idea, currentField, currentValue.String())
			}
			currentField = "SOLUTION"
			currentValue.Reset()
			currentValue.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "SOLUTION:")))
		} else if strings.HasPrefix(line, "VALUE_PROP:") {
			if currentField != "" {
				assignField(idea, currentField, currentValue.String())
			}
			currentField = "VALUE_PROP"
			currentValue.Reset()
			currentValue.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "VALUE_PROP:")))
		} else if strings.HasPrefix(line, "TARGET_CUSTOMER:") {
			if currentField != "" {
				assignField(idea, currentField, currentValue.String())
			}
			currentField = "TARGET_CUSTOMER"
			currentValue.Reset()
			currentValue.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "TARGET_CUSTOMER:")))
		} else if strings.HasPrefix(line, "REVENUE_MODEL:") {
			if currentField != "" {
				assignField(idea, currentField, currentValue.String())
			}
			currentField = "REVENUE_MODEL"
			currentValue.Reset()
			currentValue.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "REVENUE_MODEL:")))
		} else if currentField != "" && line != "" && !strings.HasPrefix(line, "#") {
			// Continue accumulating multi-line value (skip empty lines and markdown headers)
			if currentValue.Len() > 0 {
				currentValue.WriteString(" ")
			}
			currentValue.WriteString(line)
		}
	}
	
	// Save the last field
	if currentField != "" {
		assignField(idea, currentField, currentValue.String())
	}
	
	// Log what we parsed
	log.WithFields(log.Fields{
		"pipeline_id": pipeline.ID,
		"problem": idea.Problem,
		"solution": idea.Solution,
		"value_prop": idea.ValueProp,
		"target": idea.TargetCustomer,
		"revenue": idea.RevenueModel,
		"total_lines": len(lines),
	}).Info("Parsed idea fields")
	
	// Validate critical fields
	if idea.Problem == "" || idea.Solution == "" {
		log.WithFields(log.Fields{
			"pipeline_id": pipeline.ID,
			"has_problem": idea.Problem != "",
			"has_solution": idea.Solution != "",
		}).Warn("Generated idea missing critical fields")
	}
	
	pm.mu.Lock()
	pipeline.Idea = idea
	
	// Set name with UTF-8 safe truncation and fallback
	if idea.Solution != "" {
		pipeline.Name = idea.Solution
		if len([]rune(pipeline.Name)) > 50 {
			pipeline.Name = string([]rune(pipeline.Name)[:50])
		}
	} else {
		pipeline.Name = fmt.Sprintf("Product Idea %s", pipeline.ID.String()[:8])
	}
	
	// Set description with fallback
	if idea.Problem != "" {
		pipeline.Description = idea.Problem
	} else {
		pipeline.Description = "Product concept in development"
	}
	
	pipeline.Stage = StageWorkPacket
	pipeline.UpdatedAt = time.Now()
	pm.mu.Unlock()
	
	log.WithField("pipeline_id", pipeline.ID).Info("C-Suite idea generated, moving to work packet stage")
	
	// Broadcast update
	if pm.org.wsHub != nil {
		pm.org.wsHub.Broadcast(WebSocketMessage{
			Type: "pipeline_update",
			Payload: map[string]interface{}{
				"pipeline_id": pipeline.ID.String(),
				"stage":       pipeline.Stage,
				"name":        pipeline.Name,
			},
		})
	}
	
	return nil
}

// AssignWorkPacketTasks assigns work to employees to build the work packet
func (pm *PipelineManager) AssignWorkPacketTasks(pipeline *ProductPipeline) error {
	if pipeline.Idea == nil {
		return fmt.Errorf("pipeline has no idea to work on")
	}
	
	seed := pm.org.GetSeed()
	if seed == nil {
		return fmt.Errorf("company not seeded")
	}
	
	// Define tasks for the work packet
	tasks := []struct {
		Skill       EmployeeSkill
		Title       string
		Description string
		Field       string // Which WorkPacketContent field this populates
	}{
		{
			Skill: SkillResearch,
			Title: fmt.Sprintf("Market Research: %s", pipeline.Name),
			Description: fmt.Sprintf(`Research the market for: %s

Problem being solved: %s
Target customer: %s

Provide comprehensive market research including:
- Market size and growth rate
- Key market trends
- Customer pain points
- Willingness to pay`, pipeline.Name, pipeline.Idea.Problem, pipeline.Idea.TargetCustomer),
			Field: "market_research",
		},
		{
			Skill: SkillAnalysis,
			Title: fmt.Sprintf("Competitive Analysis: %s", pipeline.Name),
			Description: fmt.Sprintf(`Analyze competitors for: %s

Our solution: %s
Our value proposition: %s

Identify:
- Direct competitors
- Indirect competitors
- Their strengths and weaknesses
- Our competitive advantage`, pipeline.Name, pipeline.Idea.Solution, pipeline.Idea.ValueProp),
			Field: "competitive_analysis",
		},
		{
			Skill: SkillWriting,
			Title: fmt.Sprintf("Business Plan: %s", pipeline.Name),
			Description: fmt.Sprintf(`Write a business plan for: %s

Problem: %s
Solution: %s
Revenue Model: %s

Include executive summary, value proposition, go-to-market strategy, and operational plan.`, 
				pipeline.Name, pipeline.Idea.Problem, pipeline.Idea.Solution, pipeline.Idea.RevenueModel),
			Field: "business_plan",
		},
		{
			Skill: SkillAnalysis,
			Title: fmt.Sprintf("Financial Projections: %s", pipeline.Name),
			Description: fmt.Sprintf(`Create financial projections for: %s

Revenue Model: %s
Target Customer: %s

Provide 3-year projections including revenue, costs, and profitability.`,
				pipeline.Name, pipeline.Idea.RevenueModel, pipeline.Idea.TargetCustomer),
			Field: "financial_projections",
		},
		{
			Skill: SkillMarketing,
			Title: fmt.Sprintf("Marketing Strategy: %s", pipeline.Name),
			Description: fmt.Sprintf(`Develop marketing strategy for: %s

Value Proposition: %s
Target Customer: %s

Include positioning, messaging, channels, and launch plan.`,
				pipeline.Name, pipeline.Idea.ValueProp, pipeline.Idea.TargetCustomer),
			Field: "marketing_strategy",
		},
	}
	
	// Store task mapping in pipeline metadata
	taskMap := make(map[string]string) // work_id -> field
	
	for _, task := range tasks {
		work := &WorkItem{
			ID:          uuid.New(),
			Title:       task.Title,
			Description: task.Description,
			Priority:    1,
			CreatedAt:   time.Now(),
			Inputs: map[string]interface{}{
				"pipeline_id": pipeline.ID.String(),
				"field":       task.Field,
			},
		}
		
		taskMap[work.ID.String()] = task.Field
		
		if err := pm.org.AssignWork(task.Skill, work); err != nil {
			log.WithError(err).WithField("task", task.Title).Warn("Failed to assign work packet task")
		} else {
			log.WithField("task", task.Title).Info("Work packet task assigned")
		}
	}
	
	pm.mu.Lock()
	if pipeline.Metadata == nil {
		pipeline.Metadata = make(map[string]interface{})
	}
	pipeline.Metadata["task_map"] = taskMap
	pipeline.Metadata["pending_tasks"] = len(tasks)
	pm.mu.Unlock()
	
	return nil
}

// OnWorkComplete is called when a work item completes, to aggregate into work packet
func (pm *PipelineManager) OnWorkComplete(workID uuid.UUID, output string, pipelineID uuid.UUID, field string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	pipeline, ok := pm.pipelines[pipelineID]
	if !ok {
		return
	}
	
	if pipeline.WorkPacket == nil {
		pipeline.WorkPacket = &WorkPacketContent{
			AssembledAt:  time.Now(),
			Contributors: make([]uuid.UUID, 0),
		}
	}
	
	// Update the appropriate field
	switch field {
	case "market_research":
		pipeline.WorkPacket.MarketResearch = output
	case "competitive_analysis":
		pipeline.WorkPacket.CompetitiveAnalysis = output
	case "business_plan":
		pipeline.WorkPacket.BusinessPlan = output
	case "financial_projections":
		pipeline.WorkPacket.FinancialProjections = output
	case "marketing_strategy":
		pipeline.WorkPacket.MarketingStrategy = output
	case "technical_overview":
		pipeline.WorkPacket.TechnicalOverview = output
	case "risk_analysis":
		pipeline.WorkPacket.RiskAnalysis = output
	}
	
	// Decrement pending tasks
	if pending, ok := pipeline.Metadata["pending_tasks"].(int); ok {
		pipeline.Metadata["pending_tasks"] = pending - 1
		
		// Check if all tasks complete
		if pending-1 <= 0 {
			pipeline.Stage = StageCsuiteReview
			pipeline.UpdatedAt = time.Now()
			log.WithField("pipeline_id", pipeline.ID).Info("Work packet complete, moving to C-Suite review")
			
			// Trigger C-Suite review
			go pm.TriggerCsuiteReview(pipeline)
		}
	}
}

// TriggerCsuiteReview has C-Suite review the work packet
func (pm *PipelineManager) TriggerCsuiteReview(pipeline *ProductPipeline) {
	provider, err := pm.org.providers.GetProvider(pm.org.config.DefaultProvider)
	if err != nil || provider == nil {
		log.Warn("No provider for C-Suite review")
		return
	}
	
	// Acquire semaphore
	select {
	case pm.org.llmSemaphore <- struct{}{}:
		defer func() { <-pm.org.llmSemaphore }()
	case <-pm.org.ctx.Done():
		return
	}
	
	// Build review prompt
	prompt := fmt.Sprintf(`You are the executive team reviewing a product proposal.

PRODUCT: %s

ORIGINAL IDEA:
Problem: %s
Solution: %s
Value Proposition: %s

WORK PACKET SUMMARY:
Market Research: %s

Competitive Analysis: %s

Business Plan: %s

Financial Projections: %s

Marketing Strategy: %s

Based on this work packet, decide if this product should go to the Board of Directors for funding approval.

Respond in this EXACT format:
DECISION: [APPROVED or NEEDS_WORK]
FEEDBACK: [Your overall assessment]
CONCERNS: [Comma-separated list of concerns, or "None"]
SUGGESTIONS: [Comma-separated list of suggestions for improvement]`,
		pipeline.Name,
		pipeline.Idea.Problem, pipeline.Idea.Solution, pipeline.Idea.ValueProp,
		truncate(pipeline.WorkPacket.MarketResearch, 500),
		truncate(pipeline.WorkPacket.CompetitiveAnalysis, 500),
		truncate(pipeline.WorkPacket.BusinessPlan, 500),
		truncate(pipeline.WorkPacket.FinancialProjections, 500),
		truncate(pipeline.WorkPacket.MarketingStrategy, 500))
	
	timeout := time.Duration(pm.org.config.LLMTimeoutMinutes) * time.Minute
	ctx, cancel := context.WithTimeout(pm.org.ctx, timeout)
	defer cancel()
	
	resp, err := provider.Chat(ctx, LLMRequest{
		Messages: []LLMMessage{{Role: "user", Content: prompt}},
		MaxTokens: 600,
		Temperature: 0.3,
	})
	if err != nil {
		log.WithError(err).Error("C-Suite review failed")
		return
	}
	
	// Parse response
	review := &ReviewContent{
		ReviewedAt: time.Now(),
	}
	
	lines := strings.Split(resp.Content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "DECISION:") {
			decision := strings.TrimSpace(strings.TrimPrefix(line, "DECISION:"))
			review.Approved = strings.Contains(strings.ToUpper(decision), "APPROVED") && 
			                  !strings.Contains(strings.ToUpper(decision), "NEEDS")
		} else if strings.HasPrefix(line, "FEEDBACK:") {
			review.Feedback = strings.TrimSpace(strings.TrimPrefix(line, "FEEDBACK:"))
		} else if strings.HasPrefix(line, "CONCERNS:") {
			concerns := strings.TrimSpace(strings.TrimPrefix(line, "CONCERNS:"))
			if concerns != "None" && concerns != "" {
				for _, c := range strings.Split(concerns, ",") {
					if c = strings.TrimSpace(c); c != "" {
						review.Concerns = append(review.Concerns, c)
					}
				}
			}
		} else if strings.HasPrefix(line, "SUGGESTIONS:") {
			suggestions := strings.TrimSpace(strings.TrimPrefix(line, "SUGGESTIONS:"))
			for _, s := range strings.Split(suggestions, ",") {
				if s = strings.TrimSpace(s); s != "" {
					review.Suggestions = append(review.Suggestions, s)
				}
			}
		}
	}
	
	pm.mu.Lock()
	pipeline.CsuiteReview = review
	pipeline.UpdatedAt = time.Now()
	
	if review.Approved {
		pipeline.Stage = StageBoardVote
		log.WithField("pipeline_id", pipeline.ID).Info("C-Suite approved, moving to Board vote")
	} else {
		pipeline.RevisionCount++
		if pipeline.RevisionCount >= 3 {
			pipeline.Stage = StageRejected
			log.WithField("pipeline_id", pipeline.ID).Info("Too many revisions, rejecting pipeline")
		} else {
			pipeline.Stage = StageWorkPacket
			log.WithField("pipeline_id", pipeline.ID).Info("C-Suite requested revisions")
			// Re-assign work
			go pm.AssignWorkPacketTasks(pipeline)
		}
	}
	pm.mu.Unlock()
	
	// Broadcast update
	if pm.org.wsHub != nil {
		pm.org.wsHub.Broadcast(WebSocketMessage{
			Type: "pipeline_update",
			Payload: map[string]interface{}{
				"pipeline_id": pipeline.ID.String(),
				"stage":       pipeline.Stage,
				"approved":    review.Approved,
				"feedback":    review.Feedback,
			},
		})
	}
	
	// If approved, trigger board vote
	if review.Approved {
		go pm.TriggerBoardVote(pipeline)
	}
}

// TriggerBoardVote has the Board of Directors vote on the product
func (pm *PipelineManager) TriggerBoardVote(pipeline *ProductPipeline) {
	// Simulate a board vote
	// TODO: Integrate with actual Board voting mechanism
	decision := &PipelineBoardDecision{
		Approved:     true,
		VotesFor:     8,
		VotesAgainst: 3,
		Abstentions:  1,
		DecidedAt:    time.Now(),
		Comments:     make(map[string]string),
	}
	
	pm.mu.Lock()
	pipeline.BoardDecision = decision
	if decision.Approved {
		pipeline.Stage = StageExecutionPlan
		log.WithField("pipeline_id", pipeline.ID).Info("Board approved, moving to execution planning")
	} else {
		pipeline.Stage = StageRejected
		log.WithField("pipeline_id", pipeline.ID).Info("Board rejected product")
	}
	pipeline.UpdatedAt = time.Now()
	pm.mu.Unlock()
	
	// Broadcast
	if pm.org.wsHub != nil {
		pm.org.wsHub.Broadcast(WebSocketMessage{
			Type: "pipeline_update",
			Payload: map[string]interface{}{
				"pipeline_id":  pipeline.ID.String(),
				"stage":        pipeline.Stage,
				"votes_for":    decision.VotesFor,
				"votes_against": decision.VotesAgainst,
			},
		})
	}
	
	if decision.Approved {
		go pm.GenerateExecutionPlan(pipeline)
	}
}

// GenerateExecutionPlan creates the execution plan
func (pm *PipelineManager) GenerateExecutionPlan(pipeline *ProductPipeline) {
	provider, err := pm.org.providers.GetProvider(pm.org.config.DefaultProvider)
	if err != nil || provider == nil {
		log.Warn("No provider for execution planning")
		return
	}
	
	// Acquire semaphore
	select {
	case pm.org.llmSemaphore <- struct{}{}:
		defer func() { <-pm.org.llmSemaphore }()
	case <-pm.org.ctx.Done():
		return
	}
	
	prompt := fmt.Sprintf(`You are the CEO creating an execution plan for an approved product.

PRODUCT: %s
Problem: %s
Solution: %s
Revenue Model: %s

Create a detailed execution plan.

Respond in this EXACT JSON format:
{
  "timeline": "6 months",
  "budget": "$500,000",
  "team_structure": "Description of team needed",
  "kpis": ["KPI 1", "KPI 2", "KPI 3"],
  "milestones": ["Month 1: ...", "Month 3: ...", "Month 6: ..."],
  "risk_mitigation": "Key risk mitigation strategies",
  "phases": [
    {
      "name": "Phase 1: Foundation",
      "description": "Initial setup and development",
      "duration": "2 months",
      "tasks": ["Task 1", "Task 2"],
      "deliverables": ["Deliverable 1", "Deliverable 2"]
    }
  ]
}`,
		pipeline.Name, pipeline.Idea.Problem, pipeline.Idea.Solution, pipeline.Idea.RevenueModel)
	
	timeout := time.Duration(pm.org.config.LLMTimeoutMinutes) * time.Minute
	ctx, cancel := context.WithTimeout(pm.org.ctx, timeout)
	defer cancel()
	
	resp, err := provider.Chat(ctx, LLMRequest{
		Messages: []LLMMessage{{Role: "user", Content: prompt}},
		MaxTokens: 1500,
		Temperature: 0.4,
	})
	if err != nil {
		log.WithError(err).Error("Execution plan generation failed")
		return
	}
	
	// Parse JSON response
	plan := &ExecutionPlanContent{
		CreatedAt: time.Now(),
	}
	
	// Extract JSON from response
	content := resp.Content
	if idx := strings.Index(content, "{"); idx >= 0 {
		if endIdx := strings.LastIndex(content, "}"); endIdx > idx {
			content = content[idx:endIdx+1]
		}
	}
	
	var parsed struct {
		Timeline       string           `json:"timeline"`
		Budget         string           `json:"budget"`
		TeamStructure  string           `json:"team_structure"`
		KPIs           []string         `json:"kpis"`
		Milestones     []string         `json:"milestones"`
		RiskMitigation string           `json:"risk_mitigation"`
		Phases         []ExecutionPhase `json:"phases"`
	}
	
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		log.WithError(err).Warn("Failed to parse execution plan JSON")
		// Use raw content
		plan.Timeline = "6 months"
		plan.Budget = "TBD"
	} else {
		plan.Timeline = parsed.Timeline
		plan.Budget = parsed.Budget
		plan.TeamStructure = parsed.TeamStructure
		plan.KPIs = parsed.KPIs
		plan.Milestones = parsed.Milestones
		plan.RiskMitigation = parsed.RiskMitigation
		plan.Phases = parsed.Phases
	}
	
	pm.mu.Lock()
	pipeline.ExecutionPlan = plan
	pipeline.Stage = StageLaunched // For now, skip production phase
	pipeline.UpdatedAt = time.Now()
	pm.mu.Unlock()
	
	log.WithField("pipeline_id", pipeline.ID).Info("Execution plan created, product launched!")
	
	// Broadcast
	if pm.org.wsHub != nil {
		pm.org.wsHub.Broadcast(WebSocketMessage{
			Type: "pipeline_complete",
			Payload: map[string]interface{}{
				"pipeline_id": pipeline.ID.String(),
				"name":        pipeline.Name,
				"stage":       pipeline.Stage,
			},
		})
	}
}

// StartPipeline kicks off the full pipeline process
func (pm *PipelineManager) StartPipeline(pipeline *ProductPipeline) {
	timeout := time.Duration(pm.org.config.LLMTimeoutMinutes) * time.Minute
	ctx, cancel := context.WithTimeout(pm.org.ctx, timeout)
	defer cancel()
	
	// Step 1: Generate C-Suite idea
	if err := pm.GenerateCSuiteIdea(ctx, pipeline); err != nil {
		log.WithError(err).Error("Failed to generate C-Suite idea")
		return
	}
	
	// Step 2: Assign work packet tasks
	if err := pm.AssignWorkPacketTasks(pipeline); err != nil {
		log.WithError(err).Error("Failed to assign work packet tasks")
		return
	}
	
	// The rest of the pipeline is triggered by callbacks
}

// Helper function to truncate strings (UTF-8 safe)
func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

// GenerateHTML generates an HTML document for the pipeline that can be printed to PDF
func (pm *PipelineManager) GenerateHTML(pipelineID uuid.UUID) (string, error) {
	pm.mu.RLock()
	pipeline, ok := pm.pipelines[pipelineID]
	pm.mu.RUnlock()
	
	if !ok {
		return "", fmt.Errorf("pipeline not found")
	}
	
	seed := pm.org.GetSeed()
	companyName := "Company"
	if seed != nil {
		companyName = seed.CompanyName
	}
	
	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>%s - Business Execution Plan</title>
    <style>
        body { font-family: 'Segoe UI', Arial, sans-serif; max-width: 800px; margin: 0 auto; padding: 40px; line-height: 1.6; }
        h1 { color: #1a365d; border-bottom: 3px solid #3182ce; padding-bottom: 10px; }
        h2 { color: #2c5282; margin-top: 30px; border-bottom: 1px solid #bee3f8; padding-bottom: 5px; }
        h3 { color: #2b6cb0; }
        .header { text-align: center; margin-bottom: 40px; }
        .logo { font-size: 24px; font-weight: bold; color: #2b6cb0; }
        .meta { color: #718096; font-size: 14px; margin: 10px 0; }
        .section { margin: 20px 0; padding: 15px; background: #f7fafc; border-radius: 8px; }
        .highlight { background: #ebf8ff; padding: 15px; border-left: 4px solid #3182ce; margin: 15px 0; }
        .phase { background: #fff; border: 1px solid #e2e8f0; border-radius: 8px; padding: 20px; margin: 15px 0; }
        .phase-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 10px; }
        .phase-title { font-size: 18px; font-weight: bold; color: #2d3748; }
        .phase-duration { background: #bee3f8; color: #2c5282; padding: 4px 12px; border-radius: 20px; font-size: 12px; }
        ul { padding-left: 20px; }
        li { margin: 8px 0; }
        .kpi { display: inline-block; background: #c6f6d5; color: #276749; padding: 4px 12px; border-radius: 4px; margin: 4px; font-size: 13px; }
        .milestone { padding: 10px; background: #feebc8; border-radius: 4px; margin: 8px 0; }
        .footer { margin-top: 50px; padding-top: 20px; border-top: 1px solid #e2e8f0; color: #718096; font-size: 12px; text-align: center; }
        @media print { body { padding: 20px; } .section { break-inside: avoid; } }
    </style>
</head>
<body>
    <div class="header">
        <div class="logo">%s</div>
        <h1>%s</h1>
        <p class="meta">Business Execution Plan</p>
        <p class="meta">Generated: %s</p>
        <p class="meta">Stage: %s</p>
    </div>
`, pipeline.Name, companyName, pipeline.Name, time.Now().Format("January 2, 2006"), pipeline.Stage)
	
	// Executive Summary
	if pipeline.Idea != nil {
		html += fmt.Sprintf(`
    <h2>Executive Summary</h2>
    <div class="section">
        <h3>The Problem</h3>
        <p>%s</p>
        
        <h3>Our Solution</h3>
        <p>%s</p>
        
        <div class="highlight">
            <strong>Value Proposition:</strong> %s
        </div>
        
        <h3>Target Customer</h3>
        <p>%s</p>
        
        <h3>Revenue Model</h3>
        <p>%s</p>
    </div>
`, pipeline.Idea.Problem, pipeline.Idea.Solution, pipeline.Idea.ValueProp, pipeline.Idea.TargetCustomer, pipeline.Idea.RevenueModel)
	}
	
	// Work Packet - Research & Analysis
	if pipeline.WorkPacket != nil {
		html += `<h2>Research & Analysis</h2>`
		
		if pipeline.WorkPacket.MarketResearch != "" {
			html += fmt.Sprintf(`
    <div class="section">
        <h3>Market Research</h3>
        <p>%s</p>
    </div>
`, cleanOutput(pipeline.WorkPacket.MarketResearch))
		}
		
		if pipeline.WorkPacket.CompetitiveAnalysis != "" {
			html += fmt.Sprintf(`
    <div class="section">
        <h3>Competitive Analysis</h3>
        <p>%s</p>
    </div>
`, cleanOutput(pipeline.WorkPacket.CompetitiveAnalysis))
		}
		
		if pipeline.WorkPacket.FinancialProjections != "" {
			html += fmt.Sprintf(`
    <div class="section">
        <h3>Financial Projections</h3>
        <p>%s</p>
    </div>
`, cleanOutput(pipeline.WorkPacket.FinancialProjections))
		}
		
		if pipeline.WorkPacket.MarketingStrategy != "" {
			html += fmt.Sprintf(`
    <div class="section">
        <h3>Marketing Strategy</h3>
        <p>%s</p>
    </div>
`, cleanOutput(pipeline.WorkPacket.MarketingStrategy))
		}
		
		if pipeline.WorkPacket.BusinessPlan != "" {
			html += fmt.Sprintf(`
    <div class="section">
        <h3>Business Plan</h3>
        <p>%s</p>
    </div>
`, cleanOutput(pipeline.WorkPacket.BusinessPlan))
		}
	}
	
	// Execution Plan
	if pipeline.ExecutionPlan != nil {
		plan := pipeline.ExecutionPlan
		html += fmt.Sprintf(`
    <h2>Execution Plan</h2>
    <div class="highlight">
        <strong>Timeline:</strong> %s<br>
        <strong>Budget:</strong> %s
    </div>
`, plan.Timeline, plan.Budget)
		
		if plan.TeamStructure != "" {
			html += fmt.Sprintf(`
    <div class="section">
        <h3>Team Structure</h3>
        <p>%s</p>
    </div>
`, plan.TeamStructure)
		}
		
		// KPIs
		if len(plan.KPIs) > 0 {
			html += `<h3>Key Performance Indicators</h3><div class="section">`
			for _, kpi := range plan.KPIs {
				html += fmt.Sprintf(`<span class="kpi">%s</span>`, kpi)
			}
			html += `</div>`
		}
		
		// Milestones
		if len(plan.Milestones) > 0 {
			html += `<h3>Milestones</h3><div class="section">`
			for _, m := range plan.Milestones {
				html += fmt.Sprintf(`<div class="milestone">%s</div>`, m)
			}
			html += `</div>`
		}
		
		// Phases
		if len(plan.Phases) > 0 {
			html += `<h3>Implementation Phases</h3>`
			for i, phase := range plan.Phases {
				html += fmt.Sprintf(`
    <div class="phase">
        <div class="phase-header">
            <span class="phase-title">Phase %d: %s</span>
            <span class="phase-duration">%s</span>
        </div>
        <p>%s</p>
`, i+1, phase.Name, phase.Duration, phase.Description)
				
				if len(phase.Tasks) > 0 {
					html += `<strong>Tasks:</strong><ul>`
					for _, task := range phase.Tasks {
						html += fmt.Sprintf(`<li>%s</li>`, task)
					}
					html += `</ul>`
				}
				
				if len(phase.Deliverables) > 0 {
					html += `<strong>Deliverables:</strong><ul>`
					for _, d := range phase.Deliverables {
						html += fmt.Sprintf(`<li>%s</li>`, d)
					}
					html += `</ul>`
				}
				
				html += `</div>`
			}
		}
		
		if plan.RiskMitigation != "" {
			html += fmt.Sprintf(`
    <div class="section">
        <h3>Risk Mitigation</h3>
        <p>%s</p>
    </div>
`, plan.RiskMitigation)
		}
	}
	
	// Board Decision
	if pipeline.BoardDecision != nil {
		bd := pipeline.BoardDecision
		status := "Rejected"
		if bd.Approved {
			status = "Approved"
		}
		html += fmt.Sprintf(`
    <h2>Board Decision</h2>
    <div class="section">
        <p><strong>Status:</strong> %s</p>
        <p><strong>Votes For:</strong> %d | <strong>Against:</strong> %d | <strong>Abstentions:</strong> %d</p>
    </div>
`, status, bd.VotesFor, bd.VotesAgainst, bd.Abstentions)
	}
	
	// Footer
	html += fmt.Sprintf(`
    <div class="footer">
        <p>Generated by AI Corporation Pipeline System</p>
        <p>Pipeline ID: %s</p>
        <p>This document is auto-generated and should be reviewed by human stakeholders before execution.</p>
    </div>
</body>
</html>
`, pipeline.ID.String())
	
	return html, nil
}

// cleanOutput removes <think> tags and cleans up LLM output for display
func cleanOutput(s string) string {
	// Remove <think>...</think> blocks
	for {
		start := strings.Index(s, "<think>")
		if start == -1 {
			break
		}
		end := strings.Index(s, "</think>")
		if end == -1 {
			s = s[:start]
			break
		}
		s = s[:start] + s[end+8:]
	}
	
	// Replace newlines with <br> for HTML
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n\n", "</p><p>")
	s = strings.ReplaceAll(s, "\n", "<br>")
	
	return s
}

// assignField assigns a parsed value to the appropriate IdeaContent field
func assignField(idea *IdeaContent, field string, value string) {
	value = strings.TrimSpace(value)
	switch field {
	case "PROBLEM":
		idea.Problem = value
	case "SOLUTION":
		idea.Solution = value
	case "VALUE_PROP":
		idea.ValueProp = value
	case "TARGET_CUSTOMER":
		idea.TargetCustomer = value
	case "REVENUE_MODEL":
		idea.RevenueModel = value
	}
}
