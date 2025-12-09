package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

// ScheduleType represents different types of scheduled tasks
type ScheduleType string

const (
	ScheduleOnce     ScheduleType = "once"
	ScheduleDaily    ScheduleType = "daily"
	ScheduleWeekly   ScheduleType = "weekly"
	ScheduleMonthly  ScheduleType = "monthly"
	ScheduleQuarterly ScheduleType = "quarterly"
	ScheduleCron     ScheduleType = "cron"
)

// TaskType represents different scheduled task types
type TaskType string

const (
	TaskBoardMeeting      TaskType = "board_meeting"
	TaskProjectReview     TaskType = "project_review"
	TaskPerformanceReview TaskType = "performance_review"
	TaskBudgetReview      TaskType = "budget_review"
	TaskStrategySession   TaskType = "strategy_session"
	TaskWorkflowCleanup   TaskType = "workflow_cleanup"
	TaskMetricsReport     TaskType = "metrics_report"
	// SCRUM meetings
	TaskDailyStandup      TaskType = "daily_standup"
	TaskSprintPlanning    TaskType = "sprint_planning"
	TaskSprintReview      TaskType = "sprint_review"
	TaskSprintRetro       TaskType = "sprint_retrospective"
)

// ScheduledTask represents a task to be executed on a schedule
type ScheduledTask struct {
	ID          uuid.UUID              `json:"id"`
	Name        string                 `json:"name"`
	Type        TaskType               `json:"type"`
	Schedule    ScheduleType           `json:"schedule"`
	CronExpr    string                 `json:"cron_expr,omitempty"` // For cron schedules
	NextRun     time.Time              `json:"next_run"`
	LastRun     *time.Time             `json:"last_run,omitempty"`
	Enabled     bool                   `json:"enabled"`
	Config      map[string]interface{} `json:"config,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
}

// TaskExecution represents a single execution of a scheduled task
type TaskExecution struct {
	ID         uuid.UUID  `json:"id"`
	TaskID     uuid.UUID  `json:"task_id"`
	StartedAt  time.Time  `json:"started_at"`
	EndedAt    *time.Time `json:"ended_at,omitempty"`
	Status     string     `json:"status"` // running, completed, failed
	Result     string     `json:"result,omitempty"`
	Error      string     `json:"error,omitempty"`
}

// TaskHandler is a function that handles a scheduled task
type TaskHandler func(ctx context.Context, task *ScheduledTask) error

// Scheduler manages scheduled tasks
type Scheduler struct {
	tasks    map[uuid.UUID]*ScheduledTask
	handlers map[TaskType]TaskHandler
	db       *Database
	redis    *RedisClient
	board    *Board
	orch     *Orchestrator
	org      *Organization
	mu       sync.RWMutex
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

// NewScheduler creates a new scheduler
func NewScheduler(db *Database, redis *RedisClient) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())
	return &Scheduler{
		tasks:    make(map[uuid.UUID]*ScheduledTask),
		handlers: make(map[TaskType]TaskHandler),
		db:       db,
		redis:    redis,
		ctx:      ctx,
		cancel:   cancel,
	}
}

// SetBoard sets the board reference for board-related tasks
func (s *Scheduler) SetBoard(board *Board) {
	s.board = board
}

// SetOrchestrator sets the orchestrator reference
func (s *Scheduler) SetOrchestrator(orch *Orchestrator) {
	s.orch = orch
}

// SetOrganization sets the organization reference for org-related tasks
func (s *Scheduler) SetOrganization(org *Organization) {
	s.org = org
}

// RegisterHandler registers a handler for a task type
func (s *Scheduler) RegisterHandler(taskType TaskType, handler TaskHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers[taskType] = handler
}

// Start begins the scheduler
func (s *Scheduler) Start() error {
	// Register default handlers
	s.registerDefaultHandlers()

	// Load tasks from database or create defaults
	s.initializeDefaultTasks()

	// Start scheduler loop
	s.wg.Add(1)
	go s.run()

	log.Info("Scheduler started")
	return nil
}

// Stop gracefully stops the scheduler
func (s *Scheduler) Stop() {
	s.cancel()
	s.wg.Wait()
	log.Info("Scheduler stopped")
}

// registerDefaultHandlers sets up built-in task handlers
func (s *Scheduler) registerDefaultHandlers() {
	s.RegisterHandler(TaskBoardMeeting, s.handleBoardMeeting)
	s.RegisterHandler(TaskProjectReview, s.handleProjectReview)
	s.RegisterHandler(TaskWorkflowCleanup, s.handleWorkflowCleanup)
	s.RegisterHandler(TaskMetricsReport, s.handleMetricsReport)
	// SCRUM meetings
	s.RegisterHandler(TaskDailyStandup, s.handleDailyStandup)
	s.RegisterHandler(TaskSprintPlanning, s.handleSprintPlanning)
	s.RegisterHandler(TaskSprintReview, s.handleSprintReview)
	s.RegisterHandler(TaskSprintRetro, s.handleSprintRetro)
}

// initializeDefaultTasks creates default scheduled tasks
func (s *Scheduler) initializeDefaultTasks() {
	s.mu.Lock()
	defer s.mu.Unlock()

	defaultTasks := []*ScheduledTask{
		{
			ID:       uuid.New(),
			Name:     "Weekly Board Meeting",
			Type:     TaskBoardMeeting,
			Schedule: ScheduleWeekly,
			NextRun:  s.nextWeekday(time.Monday, 9, 0), // Monday 9 AM
			Enabled:  true,
			Config: map[string]interface{}{
				"meeting_type": "regular",
				"duration":     120, // 2 hours
			},
			CreatedAt: time.Now(),
		},
		{
			ID:       uuid.New(),
			Name:     "Daily Project Review",
			Type:     TaskProjectReview,
			Schedule: ScheduleDaily,
			NextRun:  s.nextTime(17, 0), // 5 PM daily
			Enabled:  true,
			Config: map[string]interface{}{
				"review_type": "status",
			},
			CreatedAt: time.Now(),
		},
		{
			ID:       uuid.New(),
			Name:     "Quarterly Strategy Session",
			Type:     TaskBoardMeeting,
			Schedule: ScheduleQuarterly,
			NextRun:  s.nextQuarter(),
			Enabled:  true,
			Config: map[string]interface{}{
				"meeting_type": "quarterly",
				"duration":     480, // 8 hours (full day)
			},
			CreatedAt: time.Now(),
		},
		{
			ID:       uuid.New(),
			Name:     "Weekly Workflow Cleanup",
			Type:     TaskWorkflowCleanup,
			Schedule: ScheduleWeekly,
			NextRun:  s.nextWeekday(time.Sunday, 2, 0), // Sunday 2 AM
			Enabled:  true,
			Config: map[string]interface{}{
				"retention_days": 30,
			},
			CreatedAt: time.Now(),
		},
		{
			ID:       uuid.New(),
			Name:     "Daily Metrics Report",
			Type:     TaskMetricsReport,
			Schedule: ScheduleDaily,
			NextRun:  s.nextTime(6, 0), // 6 AM daily
			Enabled:  true,
			CreatedAt: time.Now(),
		},
		// SCRUM meetings
		{
			ID:       uuid.New(),
			Name:     "Daily Standup",
			Type:     TaskDailyStandup,
			Schedule: ScheduleDaily,
			NextRun:  s.nextTime(9, 0), // 9 AM daily
			Enabled:  true,
			Config: map[string]interface{}{
				"duration": 15, // 15 minutes
			},
			CreatedAt: time.Now(),
		},
		{
			ID:       uuid.New(),
			Name:     "Sprint Planning",
			Type:     TaskSprintPlanning,
			Schedule: ScheduleWeekly,
			NextRun:  s.nextWeekday(time.Monday, 10, 0), // Monday 10 AM (after standup)
			Enabled:  true,
			Config: map[string]interface{}{
				"duration":    120, // 2 hours
				"sprint_days": 14,  // 2-week sprints
			},
			CreatedAt: time.Now(),
		},
		{
			ID:       uuid.New(),
			Name:     "Sprint Review",
			Type:     TaskSprintReview,
			Schedule: ScheduleWeekly,
			NextRun:  s.nextWeekday(time.Friday, 14, 0), // Friday 2 PM (every 2 weeks)
			Enabled:  true,
			Config: map[string]interface{}{
				"duration": 60, // 1 hour
			},
			CreatedAt: time.Now(),
		},
		{
			ID:       uuid.New(),
			Name:     "Sprint Retrospective",
			Type:     TaskSprintRetro,
			Schedule: ScheduleWeekly,
			NextRun:  s.nextWeekday(time.Friday, 15, 30), // Friday 3:30 PM (after review)
			Enabled:  true,
			Config: map[string]interface{}{
				"duration": 60, // 1 hour
			},
			CreatedAt: time.Now(),
		},
	}

	for _, task := range defaultTasks {
		s.tasks[task.ID] = task
	}

	log.Infof("Initialized %d scheduled tasks", len(defaultTasks))
}

// run is the main scheduler loop
func (s *Scheduler) run() {
	defer s.wg.Done()

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.checkAndRunTasks()
		}
	}
}

// checkAndRunTasks checks for due tasks and runs them
func (s *Scheduler) checkAndRunTasks() {
	s.mu.RLock()
	var dueTasks []*ScheduledTask
	now := time.Now()

	for _, task := range s.tasks {
		if task.Enabled && task.NextRun.Before(now) {
			dueTasks = append(dueTasks, task)
		}
	}
	s.mu.RUnlock()

	for _, task := range dueTasks {
		s.wg.Add(1)
		go func(t *ScheduledTask) {
			defer s.wg.Done()
			s.executeTask(t)
		}(task)
	}
}

// executeTask runs a single task
func (s *Scheduler) executeTask(task *ScheduledTask) {
	handler, ok := s.handlers[task.Type]
	if !ok {
		log.Warnf("No handler for task type: %s", task.Type)
		return
	}

	exec := &TaskExecution{
		ID:        uuid.New(),
		TaskID:    task.ID,
		StartedAt: time.Now(),
		Status:    "running",
	}

	log.WithFields(log.Fields{
		"task_id":   task.ID,
		"task_name": task.Name,
		"task_type": task.Type,
	}).Info("Executing scheduled task")

	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Minute)
	defer cancel()

	err := handler(ctx, task)

	now := time.Now()
	exec.EndedAt = &now

	if err != nil {
		exec.Status = "failed"
		exec.Error = err.Error()
		log.WithError(err).Errorf("Task %s failed", task.Name)
	} else {
		exec.Status = "completed"
		log.Infof("Task %s completed", task.Name)
	}

	// Update task
	s.mu.Lock()
	task.LastRun = &now
	task.NextRun = s.calculateNextRun(task)
	s.mu.Unlock()
}

// calculateNextRun determines when a task should run next
func (s *Scheduler) calculateNextRun(task *ScheduledTask) time.Time {
	now := time.Now()

	switch task.Schedule {
	case ScheduleDaily:
		return now.Add(24 * time.Hour)
	case ScheduleWeekly:
		return now.Add(7 * 24 * time.Hour)
	case ScheduleMonthly:
		return now.AddDate(0, 1, 0)
	case ScheduleQuarterly:
		return now.AddDate(0, 3, 0)
	default:
		return now.Add(24 * time.Hour)
	}
}

// Helper functions for scheduling

func (s *Scheduler) nextTime(hour, minute int) time.Time {
	now := time.Now()
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
	if next.Before(now) {
		next = next.Add(24 * time.Hour)
	}
	return next
}

func (s *Scheduler) nextWeekday(weekday time.Weekday, hour, minute int) time.Time {
	now := time.Now()
	daysUntil := int(weekday - now.Weekday())
	if daysUntil <= 0 {
		daysUntil += 7
	}
	next := time.Date(now.Year(), now.Month(), now.Day()+daysUntil, hour, minute, 0, 0, now.Location())
	return next
}

func (s *Scheduler) nextQuarter() time.Time {
	now := time.Now()
	month := now.Month()
	var nextQuarterMonth time.Month

	switch {
	case month < 4:
		nextQuarterMonth = 4 // April
	case month < 7:
		nextQuarterMonth = 7 // July
	case month < 10:
		nextQuarterMonth = 10 // October
	default:
		nextQuarterMonth = 1 // January next year
	}

	year := now.Year()
	if nextQuarterMonth == 1 {
		year++
	}

	return time.Date(year, nextQuarterMonth, 1, 9, 0, 0, 0, now.Location())
}

// Task Handlers

// handleBoardMeeting conducts a board meeting
func (s *Scheduler) handleBoardMeeting(ctx context.Context, task *ScheduledTask) error {
	if s.board == nil {
		return nil // Board not initialized
	}

	meetingType, _ := task.Config["meeting_type"].(string)
	if meetingType == "" {
		meetingType = "regular"
	}

	log.WithField("meeting_type", meetingType).Info("Starting board meeting")

	// Create meeting with proper title
	title := "Weekly Board Meeting"
	if meetingType == "quarterly" {
		title = "Quarterly Strategy Session"
	} else if meetingType == "emergency" {
		title = "Emergency Board Meeting"
	}
	
	meeting := s.board.CreateMeeting(meetingType, title, time.Now())

	// Get board members as attendees
	attendees := []string{"Board Chair"}
	for memberID := range s.board.members {
		attendees = append(attendees, string(memberID))
	}
	s.board.StartMeeting(meeting.ID, attendees)

	// Opening
	s.board.AddDialogEntry(meeting.ID, "Board Chair", "", "chair",
		"Good morning, board members. I call this meeting to order.", "statement")

	// Build agenda based on meeting type
	if meetingType == "quarterly" {
		s.board.AddDialogEntry(meeting.ID, "Board Chair", "", "chair",
			"Today we'll review quarterly performance, discuss strategic initiatives, vote on budget, and assess risks.", "statement")
		
		s.board.AddDialogEntry(meeting.ID, "CFO", "cfo", "presenter",
			"Quarterly financials show steady progress. Revenue targets met at 98%. Operating costs within budget.", "statement")
		
		s.board.AddDialogEntry(meeting.ID, "CEO", "ceo", "ceo",
			"Strategic initiatives are on track. The AI transformation project has hit all milestones.", "statement")
		
		s.board.AddDialogEntry(meeting.ID, "Board Chair", "", "chair",
			"Any concerns from board members before we vote on the budget?", "question")
	} else {
		s.board.AddDialogEntry(meeting.ID, "Board Chair", "", "chair",
			"Today's agenda: review in-flight projects, new project approvals, and escalations.", "statement")
	}

	// Review in-flight projects
	runs, err := s.db.GetWorkflowRuns("running", 100)
	if err == nil && len(runs) > 0 {
		s.board.AddDialogEntry(meeting.ID, "CEO", "ceo", "presenter",
			fmt.Sprintf("We have %d projects currently in flight. All are progressing as expected.", len(runs)), "statement")
	}

	// Review pending projects
	pendingRuns, err := s.db.GetWorkflowRuns("pending", 100)
	if err == nil && len(pendingRuns) > 0 {
		s.board.AddDialogEntry(meeting.ID, "CEO", "ceo", "presenter",
			fmt.Sprintf("There are %d projects awaiting approval.", len(pendingRuns)), "statement")
	}

	// Closing
	s.board.AddDialogEntry(meeting.ID, "Board Chair", "", "chair",
		"Thank you all for your input. Meeting adjourned.", "statement")

	// End meeting with summary
	keyDecisions := []string{"Project status reviewed", "Budget on track"}
	actionItems := []string{"Follow up on pending approvals", "Prepare next quarter projections"}
	
	s.board.EndMeeting(meeting.ID,
		"Board meeting concluded successfully. All agenda items addressed.",
		keyDecisions, actionItems)

	log.WithField("meeting_id", meeting.ID).Info("Board meeting completed")
	return nil
}

// handleProjectReview reviews project status
func (s *Scheduler) handleProjectReview(ctx context.Context, task *ScheduledTask) error {
	log.Info("Running project review")

	// Get all running workflows
	runs, err := s.db.GetWorkflowRuns("running", 100)
	if err != nil {
		return err
	}

	for _, run := range runs {
		// Check for stale workflows (running > 24 hours)
		if run.StartedAt != nil && time.Since(*run.StartedAt) > 24*time.Hour {
			log.WithField("run_id", run.ID).Warn("Stale workflow detected")
			// Could auto-escalate or notify
		}
	}

	log.Infof("Reviewed %d running projects", len(runs))
	return nil
}

// handleWorkflowCleanup cleans up old workflow data
func (s *Scheduler) handleWorkflowCleanup(ctx context.Context, task *ScheduledTask) error {
	retentionDays := 30
	if days, ok := task.Config["retention_days"].(int); ok {
		retentionDays = days
	}

	log.WithField("retention_days", retentionDays).Info("Running workflow cleanup")

	// This would delete old completed/failed workflows
	// For now, just log
	// s.db.DeleteOldWorkflows(retentionDays)

	return nil
}

// handleMetricsReport generates a metrics report
func (s *Scheduler) handleMetricsReport(ctx context.Context, task *ScheduledTask) error {
	log.Info("Generating metrics report")

	// Gather metrics
	activeRuns, _ := s.db.GetActiveRunsCount()
	queueDepth, _ := s.redis.GetQueueDepth()
	dlqSize, _ := s.redis.GetDeadLetterSize()

	log.WithFields(log.Fields{
		"active_runs":     activeRuns,
		"queue_depth":     queueDepth,
		"dead_letter_size": dlqSize,
	}).Info("Metrics report generated")

	return nil
}

// AddTask adds a new scheduled task
func (s *Scheduler) AddTask(task *ScheduledTask) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tasks[task.ID] = task
}

// RemoveTask removes a scheduled task
func (s *Scheduler) RemoveTask(id uuid.UUID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tasks, id)
}

// GetTasks returns all scheduled tasks
func (s *Scheduler) GetTasks() []*ScheduledTask {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tasks := make([]*ScheduledTask, 0, len(s.tasks))
	for _, t := range s.tasks {
		tasks = append(tasks, t)
	}
	return tasks
}

// TriggerTask manually triggers a task
func (s *Scheduler) TriggerTask(id uuid.UUID) error {
	s.mu.RLock()
	task, ok := s.tasks[id]
	s.mu.RUnlock()

	if !ok {
		return fmt.Errorf("task not found: %s", id)
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.executeTask(task)
	}()

	return nil
}

// handleDailyStandup runs the daily standup meeting
func (s *Scheduler) handleDailyStandup(ctx context.Context, task *ScheduledTask) error {
	if s.board == nil {
		log.Warn("Board not available for standup")
		return nil
	}

	meeting := s.board.CreateMeeting("standup", "Daily Standup", time.Now())
	
	// Get attendees from organization
	attendees := []string{"Scrum Master"}
	if s.orch != nil && s.org != nil {
		s.org.mu.RLock()
		for _, div := range s.org.Divisions {
			for _, head := range div.Heads {
				attendees = append(attendees, head.Name)
				for _, mgr := range head.Managers {
					attendees = append(attendees, mgr.Name)
				}
			}
		}
		s.org.mu.RUnlock()
	}
	
	s.board.StartMeeting(meeting.ID, attendees)
	
	// Add standup dialog
	s.board.AddDialogEntry(meeting.ID, "Scrum Master", "", "chair", 
		"Good morning everyone. Let's start our daily standup. Remember: what did you accomplish yesterday, what are you working on today, and any blockers?", "statement")
	
	// Simulate team updates
	if s.orch != nil && s.org != nil {
		s.org.mu.RLock()
		for _, div := range s.org.Divisions {
			for _, head := range div.Heads {
				s.board.AddDialogEntry(meeting.ID, head.Name, head.ID.String(), "presenter",
					fmt.Sprintf("%s division update: Teams are progressing on current sprint goals. No major blockers.", div.Name), "statement")
			}
		}
		s.org.mu.RUnlock()
	}
	
	s.board.AddDialogEntry(meeting.ID, "Scrum Master", "", "chair", 
		"Thank you all. Remember to update your task status. See you tomorrow!", "statement")
	
	s.board.EndMeeting(meeting.ID, 
		"Daily standup completed. All teams on track.",
		[]string{"Teams progressing on sprint goals"},
		[]string{"Update task boards", "Follow up on any blockers"})
	
	log.WithField("meeting_id", meeting.ID).Info("Daily standup completed")
	return nil
}

// handleSprintPlanning runs the sprint planning meeting
func (s *Scheduler) handleSprintPlanning(ctx context.Context, task *ScheduledTask) error {
	if s.board == nil {
		log.Warn("Board not available for sprint planning")
		return nil
	}

	meeting := s.board.CreateMeeting("planning", "Sprint Planning", time.Now())
	
	attendees := []string{"Product Owner", "Scrum Master"}
	if s.orch != nil && s.org != nil {
		s.org.mu.RLock()
		for _, div := range s.org.Divisions {
			for _, head := range div.Heads {
				for _, mgr := range head.Managers {
					attendees = append(attendees, mgr.Name)
				}
			}
		}
		s.org.mu.RUnlock()
	}
	
	s.board.StartMeeting(meeting.ID, attendees)
	
	// Sprint planning dialog
	s.board.AddDialogEntry(meeting.ID, "Product Owner", "", "presenter",
		"Welcome to sprint planning. Let's review the prioritized backlog and select items for this sprint.", "statement")
	
	s.board.AddDialogEntry(meeting.ID, "Scrum Master", "", "chair",
		"Our velocity from last sprint was on target. Let's aim for similar capacity this sprint.", "statement")
	
	// Get pending work items
	pendingCount := 0
	if s.db != nil {
		runs, _ := s.db.GetWorkflowRuns("pending", 10)
		pendingCount = len(runs)
	}
	
	s.board.AddDialogEntry(meeting.ID, "Product Owner", "", "presenter",
		fmt.Sprintf("We have %d items in the backlog. Let's prioritize based on business value.", pendingCount), "statement")
	
	s.board.AddDialogEntry(meeting.ID, "Scrum Master", "", "chair",
		"Teams, please break down the selected items and estimate. Remember to account for testing and documentation.", "question")
	
	// Simulate team responses
	if s.orch != nil && s.org != nil {
		s.org.mu.RLock()
		for _, div := range s.org.Divisions {
			for _, head := range div.Heads {
				for _, mgr := range head.Managers {
					s.board.AddDialogEntry(meeting.ID, mgr.Name, mgr.ID.String(), "member",
						fmt.Sprintf("%s team can commit to their allocated items. Estimates look reasonable.", head.Title), "statement")
				}
			}
		}
		s.org.mu.RUnlock()
	}
	
	s.board.AddDialogEntry(meeting.ID, "Scrum Master", "", "chair",
		"Sprint goal is set. Let's execute! Daily standups at 9 AM.", "decision")
	
	s.board.EndMeeting(meeting.ID,
		"Sprint planning completed. Teams committed to sprint backlog.",
		[]string{"Sprint backlog defined", "Team capacity allocated", "Sprint goal established"},
		[]string{"Create sprint board", "Break down user stories", "Update estimates"})
	
	log.WithField("meeting_id", meeting.ID).Info("Sprint planning completed")
	return nil
}

// handleSprintReview runs the sprint review meeting
func (s *Scheduler) handleSprintReview(ctx context.Context, task *ScheduledTask) error {
	if s.board == nil {
		log.Warn("Board not available for sprint review")
		return nil
	}

	meeting := s.board.CreateMeeting("review", "Sprint Review", time.Now())
	
	attendees := []string{"Product Owner", "Scrum Master", "Stakeholders"}
	if s.orch != nil && s.org != nil {
		s.org.mu.RLock()
		for _, div := range s.org.Divisions {
			for _, head := range div.Heads {
				attendees = append(attendees, head.Name)
			}
		}
		s.org.mu.RUnlock()
	}
	
	s.board.StartMeeting(meeting.ID, attendees)
	
	// Sprint review dialog
	s.board.AddDialogEntry(meeting.ID, "Scrum Master", "", "chair",
		"Welcome to our sprint review. Let's demonstrate what we've accomplished this sprint.", "statement")
	
	// Get completed work
	completedCount := 0
	if s.db != nil {
		runs, _ := s.db.GetWorkflowRuns("completed", 50)
		completedCount = len(runs)
	}
	
	s.board.AddDialogEntry(meeting.ID, "Scrum Master", "", "presenter",
		fmt.Sprintf("This sprint we completed %d items. Let me walk through the highlights.", completedCount), "statement")
	
	// Demo from each team
	if s.orch != nil && s.org != nil {
		s.org.mu.RLock()
		for _, div := range s.org.Divisions {
			for _, head := range div.Heads {
				s.board.AddDialogEntry(meeting.ID, head.Name, head.ID.String(), "presenter",
					fmt.Sprintf("The %s division completed their sprint commitments. Here's what we delivered...", div.Name), "statement")
			}
		}
		s.org.mu.RUnlock()
	}
	
	s.board.AddDialogEntry(meeting.ID, "Product Owner", "", "member",
		"Great work everyone. The stakeholders are pleased with the progress. A few items will need follow-up next sprint.", "statement")
	
	s.board.AddDialogEntry(meeting.ID, "Scrum Master", "", "chair",
		"Thank you all for the demos. Let's head to the retrospective.", "statement")
	
	s.board.EndMeeting(meeting.ID,
		fmt.Sprintf("Sprint review completed. %d items demonstrated to stakeholders.", completedCount),
		[]string{"Sprint deliverables accepted", "Stakeholder feedback collected"},
		[]string{"Document feedback", "Update product backlog", "Prepare for next sprint"})
	
	log.WithField("meeting_id", meeting.ID).Info("Sprint review completed")
	return nil
}

// handleSprintRetro runs the sprint retrospective meeting
func (s *Scheduler) handleSprintRetro(ctx context.Context, task *ScheduledTask) error {
	if s.board == nil {
		log.Warn("Board not available for retrospective")
		return nil
	}

	meeting := s.board.CreateMeeting("retrospective", "Sprint Retrospective", time.Now())
	
	attendees := []string{"Scrum Master"}
	if s.orch != nil && s.org != nil {
		s.org.mu.RLock()
		for _, div := range s.org.Divisions {
			for _, head := range div.Heads {
				for _, mgr := range head.Managers {
					attendees = append(attendees, mgr.Name)
				}
			}
		}
		s.org.mu.RUnlock()
	}
	
	s.board.StartMeeting(meeting.ID, attendees)
	
	// Retrospective dialog
	s.board.AddDialogEntry(meeting.ID, "Scrum Master", "", "chair",
		"Welcome to our sprint retrospective. Let's reflect on what went well, what could improve, and actions we'll take.", "statement")
	
	s.board.AddDialogEntry(meeting.ID, "Scrum Master", "", "chair",
		"First, what went well this sprint?", "question")
	
	// Simulate team feedback
	wentWell := []string{
		"Good collaboration between teams",
		"Clear sprint goals helped focus",
		"Daily standups kept everyone aligned",
		"Stakeholder communication improved",
	}
	
	for i, item := range wentWell {
		speaker := fmt.Sprintf("Team Member %d", i+1)
		s.board.AddDialogEntry(meeting.ID, speaker, "", "member", item, "answer")
	}
	
	s.board.AddDialogEntry(meeting.ID, "Scrum Master", "", "chair",
		"Great feedback. Now, what could we improve?", "question")
	
	improvements := []string{
		"Need better estimation practices",
		"Technical debt slowing us down",
		"More time needed for testing",
	}
	
	for i, item := range improvements {
		speaker := fmt.Sprintf("Team Member %d", i+1)
		s.board.AddDialogEntry(meeting.ID, speaker, "", "member", item, "answer")
	}
	
	s.board.AddDialogEntry(meeting.ID, "Scrum Master", "", "chair",
		"Let's vote on which improvements to focus on next sprint.", "statement")
	
	s.board.AddDialogEntry(meeting.ID, "Scrum Master", "", "chair",
		"Agreed. We'll focus on improving estimation and allocating time for tech debt. Meeting adjourned.", "decision")
	
	s.board.EndMeeting(meeting.ID,
		"Sprint retrospective completed. Team identified improvements for next sprint.",
		[]string{"Identified what went well", "Agreed on improvement areas", "Action items assigned"},
		[]string{"Improve estimation practices", "Allocate 20% for tech debt", "Add testing buffer to estimates"})
	
	log.WithField("meeting_id", meeting.ID).Info("Sprint retrospective completed")
	return nil
}
