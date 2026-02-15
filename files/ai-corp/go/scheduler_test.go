package main

import (
	"testing"
	"time"
)

func TestScheduleTypes(t *testing.T) {
	tests := []struct {
		scheduleType ScheduleType
		expected     string
	}{
		{ScheduleOnce, "once"},
		{ScheduleDaily, "daily"},
		{ScheduleWeekly, "weekly"},
		{ScheduleMonthly, "monthly"},
		{ScheduleQuarterly, "quarterly"},
		{ScheduleCron, "cron"},
	}

	for _, tt := range tests {
		if string(tt.scheduleType) != tt.expected {
			t.Errorf("Expected schedule type %s, got %s", tt.expected, tt.scheduleType)
		}
	}
}

func TestTaskTypes(t *testing.T) {
	tests := []struct {
		taskType TaskType
		expected string
	}{
		{TaskBoardMeeting, "board_meeting"},
		{TaskProjectReview, "project_review"},
		{TaskPerformanceReview, "performance_review"},
		{TaskBudgetReview, "budget_review"},
		{TaskStrategySession, "strategy_session"},
		{TaskWorkflowCleanup, "workflow_cleanup"},
		{TaskMetricsReport, "metrics_report"},
	}

	for _, tt := range tests {
		if string(tt.taskType) != tt.expected {
			t.Errorf("Expected task type %s, got %s", tt.expected, tt.taskType)
		}
	}
}

func TestSchedulerCreation(t *testing.T) {
	scheduler := NewScheduler(nil, nil)

	if scheduler.tasks == nil {
		t.Error("Expected tasks map to be initialized")
	}
	if scheduler.handlers == nil {
		t.Error("Expected handlers map to be initialized")
	}
}

func TestSchedulerRegisterHandler(t *testing.T) {
	scheduler := NewScheduler(nil, nil)

	handlerCalled := false
	scheduler.RegisterHandler(TaskMetricsReport, func(ctx any, task *ScheduledTask) error {
		handlerCalled = true
		return nil
	})

	if _, ok := scheduler.handlers[TaskMetricsReport]; !ok {
		t.Error("Expected handler to be registered")
	}
}

func TestSchedulerAddRemoveTask(t *testing.T) {
	scheduler := NewScheduler(nil, nil)

	task := &ScheduledTask{
		ID:        [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		Name:      "Test Task",
		Type:      TaskMetricsReport,
		Schedule:  ScheduleDaily,
		NextRun:   time.Now().Add(time.Hour),
		Enabled:   true,
		CreatedAt: time.Now(),
	}

	scheduler.AddTask(task)

	tasks := scheduler.GetTasks()
	if len(tasks) != 1 {
		t.Errorf("Expected 1 task, got %d", len(tasks))
	}

	scheduler.RemoveTask(task.ID)

	tasks = scheduler.GetTasks()
	if len(tasks) != 0 {
		t.Errorf("Expected 0 tasks after removal, got %d", len(tasks))
	}
}

func TestSchedulerDefaultTasks(t *testing.T) {
	scheduler := NewScheduler(nil, nil)
	scheduler.initializeDefaultTasks()

	tasks := scheduler.GetTasks()
	if len(tasks) == 0 {
		t.Error("Expected default tasks to be created")
	}

	// Check for specific task types
	hasBoardMeeting := false
	hasProjectReview := false
	hasWorkflowCleanup := false

	for _, task := range tasks {
		switch task.Type {
		case TaskBoardMeeting:
			hasBoardMeeting = true
		case TaskProjectReview:
			hasProjectReview = true
		case TaskWorkflowCleanup:
			hasWorkflowCleanup = true
		}
	}

	if !hasBoardMeeting {
		t.Error("Expected board meeting task")
	}
	if !hasProjectReview {
		t.Error("Expected project review task")
	}
	if !hasWorkflowCleanup {
		t.Error("Expected workflow cleanup task")
	}
}

func TestCalculateNextRun(t *testing.T) {
	scheduler := NewScheduler(nil, nil)
	now := time.Now()

	tests := []struct {
		schedule ScheduleType
		minDiff  time.Duration
	}{
		{ScheduleDaily, 23 * time.Hour},
		{ScheduleWeekly, 6 * 24 * time.Hour},
	}

	for _, tt := range tests {
		task := &ScheduledTask{Schedule: tt.schedule}
		next := scheduler.calculateNextRun(task)
		diff := next.Sub(now)

		if diff < tt.minDiff {
			t.Errorf("Expected next run to be at least %v from now for %s, got %v",
				tt.minDiff, tt.schedule, diff)
		}
	}
}

func TestNextTimeHelper(t *testing.T) {
	scheduler := NewScheduler(nil, nil)

	next := scheduler.nextTime(9, 0)

	if next.Hour() != 9 || next.Minute() != 0 {
		t.Errorf("Expected 9:00, got %02d:%02d", next.Hour(), next.Minute())
	}

	// Should be in the future
	if !next.After(time.Now().Add(-time.Minute)) {
		t.Error("Expected next time to be in the future or very recent")
	}
}

func TestNextWeekdayHelper(t *testing.T) {
	scheduler := NewScheduler(nil, nil)

	next := scheduler.nextWeekday(time.Monday, 9, 0)

	if next.Weekday() != time.Monday {
		t.Errorf("Expected Monday, got %s", next.Weekday())
	}

	if next.Hour() != 9 || next.Minute() != 0 {
		t.Errorf("Expected 9:00, got %02d:%02d", next.Hour(), next.Minute())
	}
}

func TestNextQuarterHelper(t *testing.T) {
	scheduler := NewScheduler(nil, nil)

	next := scheduler.nextQuarter()

	// Should be on the 1st of a quarter month
	if next.Day() != 1 {
		t.Errorf("Expected day 1, got %d", next.Day())
	}

	month := next.Month()
	if month != 1 && month != 4 && month != 7 && month != 10 {
		t.Errorf("Expected quarter month (1, 4, 7, 10), got %d", month)
	}

	// Should be in the future
	if !next.After(time.Now()) {
		t.Error("Expected next quarter to be in the future")
	}
}

func TestScheduledTaskConfig(t *testing.T) {
	task := &ScheduledTask{
		Name: "Test",
		Config: map[string]interface{}{
			"duration":       120,
			"meeting_type":   "quarterly",
			"retention_days": 30,
		},
	}

	duration, ok := task.Config["duration"].(int)
	if !ok || duration != 120 {
		t.Error("Expected duration config to be 120")
	}

	meetingType, ok := task.Config["meeting_type"].(string)
	if !ok || meetingType != "quarterly" {
		t.Error("Expected meeting_type config to be 'quarterly'")
	}
}
