package main

import (
	"time"

	"github.com/google/uuid"
)

// SprintStatus represents the current status of a sprint
type SprintStatus string

const (
	SprintPlanning   SprintStatus = "planning"
	SprintActive     SprintStatus = "active"
	SprintReview     SprintStatus = "review"
	SprintCompleted  SprintStatus = "completed"
	SprintCancelled  SprintStatus = "cancelled"
)

// Sprint represents a time-boxed iteration
type Sprint struct {
	ID           uuid.UUID    `json:"id"`
	Name         string       `json:"name"`
	Number       int          `json:"number"`
	StartDate    time.Time    `json:"start_date"`
	EndDate      time.Time    `json:"end_date"`
	Goals        []string     `json:"goals"`
	Status       SprintStatus `json:"status"`
	
	// Metrics
	CommittedPoints int `json:"committed_points"`
	CompletedPoints int `json:"completed_points"`
	OpenItems       int `json:"open_items"`
	Risks           []string `json:"risks,omitempty"`
	
	CreatedAt    time.Time  `json:"created_at"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
}

// GetRemainingDays returns the number of days remaining in the sprint
func (s *Sprint) GetRemainingDays() int {
	if s.Status == SprintCompleted || s.Status == SprintCancelled {
		return 0
	}
	now := time.Now()
	if now.After(s.EndDate) {
		return 0
	}
	duration := s.EndDate.Sub(now)
	return int(duration.Hours() / 24)
}

// GetCompletionPercentage returns the percentage of committed points completed
func (s *Sprint) GetCompletionPercentage() float64 {
	if s.CommittedPoints == 0 {
		return 0
	}
	return (float64(s.CompletedPoints) / float64(s.CommittedPoints)) * 100
}

// GetDurationDays returns the total duration of the sprint in days
func (s *Sprint) GetDurationDays() int {
	duration := s.EndDate.Sub(s.StartDate)
	return int(duration.Hours() / 24)
}
