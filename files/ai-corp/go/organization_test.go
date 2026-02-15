package main

import (
	"testing"
)

func TestEmployeeStatus(t *testing.T) {
	tests := []struct {
		status   EmployeeStatus
		expected string
	}{
		{EmployeeIdle, "idle"},
		{EmployeeWorking, "working"},
		{EmployeePaused, "paused"},
		{EmployeeTerminated, "terminated"},
	}

	for _, tt := range tests {
		if string(tt.status) != tt.expected {
			t.Errorf("Expected %s, got %s", tt.expected, tt.status)
		}
	}
}

func TestQualityRating(t *testing.T) {
	tests := []struct {
		rating   QualityRating
		expected string
	}{
		{QualityExcellent, "excellent"},
		{QualityGood, "good"},
		{QualityAcceptable, "acceptable"},
		{QualityNeedsWork, "needs_work"},
		{QualityRejected, "rejected"},
	}

	for _, tt := range tests {
		if string(tt.rating) != tt.expected {
			t.Errorf("Expected %s, got %s", tt.expected, tt.rating)
		}
	}
}

func TestEmployeeSkills(t *testing.T) {
	expectedSkills := []EmployeeSkill{
		SkillWriting,
		SkillCoding,
		SkillDesign,
		SkillResearch,
		SkillAnalysis,
		SkillMarketing,
		SkillSales,
		SkillSupport,
		SkillQA,
		SkillProjectMgmt,
		SkillDataEntry,
		SkillContentReview,
	}

	if len(expectedSkills) != 12 {
		t.Errorf("Expected 12 skills, got %d", len(expectedSkills))
	}
}

func TestOrganizationCreation(t *testing.T) {
	cfg, _ := LoadConfig("/nonexistent/config.ini")
	pm := NewProviderManager(cfg)

	org := NewOrganization(cfg, pm, nil, nil)
	defer org.Stop()

	// Check divisions created
	if len(org.Divisions) == 0 {
		t.Error("Expected divisions to be created")
	}

	// Check employees created
	if len(org.AllEmployees) == 0 {
		t.Error("Expected employees to be created")
	}

	// Check managers created
	if len(org.AllManagers) == 0 {
		t.Error("Expected managers to be created")
	}

	// Check CEO created
	if org.CEO == nil {
		t.Error("Expected CEO to be created")
	}
}

func TestOrganizationStats(t *testing.T) {
	cfg, _ := LoadConfig("/nonexistent/config.ini")
	pm := NewProviderManager(cfg)

	org := NewOrganization(cfg, pm, nil, nil)
	defer org.Stop()

	stats := org.GetStats()

	if _, ok := stats["divisions"]; !ok {
		t.Error("Expected divisions in stats")
	}
	if _, ok := stats["managers"]; !ok {
		t.Error("Expected managers in stats")
	}
	if _, ok := stats["total_employees"]; !ok {
		t.Error("Expected total_employees in stats")
	}
	if _, ok := stats["by_status"]; !ok {
		t.Error("Expected by_status in stats")
	}
	if _, ok := stats["by_skill"]; !ok {
		t.Error("Expected by_skill in stats")
	}
}

func TestCreateDivision(t *testing.T) {
	cfg, _ := LoadConfig("/nonexistent/config.ini")
	pm := NewProviderManager(cfg)

	org := NewOrganization(cfg, pm, nil, nil)
	defer org.Stop()

	initialCount := len(org.Divisions)

	div := org.CreateDivision("New Division", "A new division for testing")

	if div.Name != "New Division" {
		t.Errorf("Expected name 'New Division', got %s", div.Name)
	}

	if len(org.Divisions) != initialCount+1 {
		t.Error("Expected division count to increase")
	}
}

func TestGetEmployeePersona(t *testing.T) {
	cfg, _ := LoadConfig("/nonexistent/config.ini")
	pm := NewProviderManager(cfg)

	org := NewOrganization(cfg, pm, nil, nil)
	defer org.Stop()

	// Test each skill has a persona
	skills := []EmployeeSkill{
		SkillWriting, SkillCoding, SkillDesign, SkillResearch,
		SkillAnalysis, SkillMarketing, SkillSales, SkillSupport,
		SkillQA, SkillProjectMgmt, SkillDataEntry, SkillContentReview,
	}

	for _, skill := range skills {
		persona := org.getEmployeePersona(skill)
		if persona == "" {
			t.Errorf("Expected persona for skill %s", skill)
		}
	}
}

func TestFormatObjectives(t *testing.T) {
	objectives := []string{"First objective", "Second objective", "Third objective"}
	result := formatObjectives(objectives)

	if result == "" {
		t.Error("Expected formatted objectives")
	}

	// Should contain numbered items
	if len(result) < 10 {
		t.Error("Expected longer formatted output")
	}
}

func TestFormatRevisions(t *testing.T) {
	revisions := []string{"Fix typo", "Add more detail"}
	result := formatRevisions(revisions)

	if result == "" {
		t.Error("Expected formatted revisions")
	}
}

func TestWorkItemStructure(t *testing.T) {
	work := WorkItem{
		Type:        "task",
		Title:       "Test Task",
		Description: "A test task",
		Objectives:  []string{"Complete the task"},
		Priority:    1,
	}

	if work.Title != "Test Task" {
		t.Errorf("Expected title 'Test Task', got %s", work.Title)
	}
	if work.Priority != 1 {
		t.Errorf("Expected priority 1, got %d", work.Priority)
	}
}

func TestScaleThreshold(t *testing.T) {
	cfg, _ := LoadConfig("/nonexistent/config.ini")
	pm := NewProviderManager(cfg)

	org := NewOrganization(cfg, pm, nil, nil)
	defer org.Stop()

	// Check default scale settings
	if org.minPoolSize != 2 {
		t.Errorf("Expected minPoolSize 2, got %d", org.minPoolSize)
	}
	if org.maxPoolSize != 50 {
		t.Errorf("Expected maxPoolSize 50, got %d", org.maxPoolSize)
	}
	if org.scaleThreshold != 0.8 {
		t.Errorf("Expected scaleThreshold 0.8, got %f", org.scaleThreshold)
	}
}

func TestDefaultDivisions(t *testing.T) {
	cfg, _ := LoadConfig("/nonexistent/config.ini")
	pm := NewProviderManager(cfg)

	org := NewOrganization(cfg, pm, nil, nil)
	defer org.Stop()

	// Should have Technology, Marketing, Operations
	expectedDivisions := 3
	if len(org.Divisions) != expectedDivisions {
		t.Errorf("Expected %d divisions, got %d", expectedDivisions, len(org.Divisions))
	}

	// Find division names
	divNames := make(map[string]bool)
	for _, div := range org.Divisions {
		divNames[div.Name] = true
	}

	if !divNames["Technology"] {
		t.Error("Expected Technology division")
	}
	if !divNames["Marketing"] {
		t.Error("Expected Marketing division")
	}
	if !divNames["Operations"] {
		t.Error("Expected Operations division")
	}
}
