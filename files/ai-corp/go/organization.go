package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

// EmployeeStatus represents employee work status
type EmployeeStatus string

const (
	EmployeeIdle     EmployeeStatus = "idle"
	EmployeeWorking  EmployeeStatus = "working"
	EmployeePaused   EmployeeStatus = "paused"
	EmployeeTerminated EmployeeStatus = "terminated"
)

// QualityRating represents work quality assessment
type QualityRating string

const (
	QualityExcellent QualityRating = "excellent"
	QualityGood      QualityRating = "good"
	QualityAcceptable QualityRating = "acceptable"
	QualityNeedsWork QualityRating = "needs_work"
	QualityRejected  QualityRating = "rejected"
)

// EmployeeSkill represents a focused skill
type EmployeeSkill string

const (
	SkillWriting       EmployeeSkill = "writing"
	SkillCoding        EmployeeSkill = "coding"
	SkillDesign        EmployeeSkill = "design"
	SkillResearch      EmployeeSkill = "research"
	SkillAnalysis      EmployeeSkill = "analysis"
	SkillMarketing     EmployeeSkill = "marketing"
	SkillSales         EmployeeSkill = "sales"
	SkillSupport       EmployeeSkill = "support"
	SkillQA            EmployeeSkill = "qa"
	SkillProjectMgmt   EmployeeSkill = "project_management"
	SkillDataEntry     EmployeeSkill = "data_entry"
	SkillContentReview EmployeeSkill = "content_review"
)

// WorkItem represents a unit of work for an employee
type WorkItem struct {
	ID          uuid.UUID              `json:"id"`
	Type        string                 `json:"type"`
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	Objectives  []string               `json:"objectives"`
	Inputs      map[string]interface{} `json:"inputs"`
	Priority    int                    `json:"priority"` // 1=critical, 5=low
	Deadline    *time.Time             `json:"deadline,omitempty"`
	AssignedTo  uuid.UUID              `json:"assigned_to"`
	AssignedBy  uuid.UUID              `json:"assigned_by"`
	CreatedAt   time.Time              `json:"created_at"`
}

// WorkResult represents completed work from an employee
type WorkResult struct {
	ID           uuid.UUID              `json:"id"`
	WorkItemID   uuid.UUID              `json:"work_item_id"`
	EmployeeID   uuid.UUID              `json:"employee_id"`
	Output       string                 `json:"output"`
	Artifacts    []uuid.UUID            `json:"artifacts,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
	TokensUsed   int                    `json:"tokens_used"`
	CompletedAt  time.Time              `json:"completed_at"`
	Duration     time.Duration          `json:"duration"`
}

// QualityReview represents a manager's review of work
type QualityReview struct {
	ID            uuid.UUID     `json:"id"`
	ResultID      uuid.UUID     `json:"result_id"`
	ReviewerID    uuid.UUID     `json:"reviewer_id"`
	Rating        QualityRating `json:"rating"`
	Feedback      string        `json:"feedback"`
	Approved      bool          `json:"approved"`
	Revisions     []string      `json:"revisions,omitempty"`
	ReviewedAt    time.Time     `json:"reviewed_at"`
}

// ActivityType represents the type of logged activity
type ActivityType string

const (
	ActivityAssigned    ActivityType = "assigned"
	ActivityStarted     ActivityType = "started"
	ActivityCompleted   ActivityType = "completed"
	ActivityPaused      ActivityType = "paused"
	ActivityResumed     ActivityType = "resumed"
	ActivityDecision    ActivityType = "decision"
	ActivityAction      ActivityType = "action"
	ActivityResult      ActivityType = "result"
	ActivityReview      ActivityType = "review"
	ActivityRevision    ActivityType = "revision"
	ActivityError       ActivityType = "error"
)

// ActivityLogEntry represents a single logged activity for an employee
type ActivityLogEntry struct {
	ID          uuid.UUID              `json:"id"`
	Timestamp   time.Time              `json:"timestamp"`
	Type        ActivityType           `json:"type"`
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	WorkItemID  *uuid.UUID             `json:"work_item_id,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	Duration    *time.Duration         `json:"duration,omitempty"`
}

// Employee represents a single-focused worker
type Employee struct {
	ID          uuid.UUID      `json:"id"`
	Name        string         `json:"name"`
	Skill       EmployeeSkill  `json:"skill"`
	ManagerID   uuid.UUID      `json:"manager_id"`
	DeptID      uuid.UUID      `json:"dept_id"`
	Status      EmployeeStatus `json:"status"`
	Persona     string         `json:"persona"`
	CurrentWork *WorkItem      `json:"current_work,omitempty"`
	HiredAt     time.Time      `json:"hired_at"`
	
	// Work tracking
	CurrentWorkStarted *time.Time         `json:"current_work_started,omitempty"`
	WorkHistory        []WorkResult       `json:"work_history,omitempty"`
	ActivityLog        []ActivityLogEntry `json:"activity_log,omitempty"`
	
	// Runtime
	workQueue   chan *WorkItem
	resultChan  chan *WorkResult
	ctx         context.Context
	cancel      context.CancelFunc
	provider    Provider
	mu          sync.RWMutex
	workCount   int64
}

// LogActivity adds an entry to the employee's activity log
func (e *Employee) LogActivity(actType ActivityType, title, description string, workItemID *uuid.UUID, metadata map[string]interface{}) {
	e.mu.Lock()
	defer e.mu.Unlock()
	
	entry := ActivityLogEntry{
		ID:          uuid.New(),
		Timestamp:   time.Now(),
		Type:        actType,
		Title:       title,
		Description: description,
		WorkItemID:  workItemID,
		Metadata:    metadata,
	}
	
	// Keep only last 100 entries
	if len(e.ActivityLog) >= 100 {
		e.ActivityLog = e.ActivityLog[1:]
	}
	e.ActivityLog = append(e.ActivityLog, entry)
}

// GetActivityLog returns the activity log (thread-safe)
func (e *Employee) GetActivityLog() []ActivityLogEntry {
	e.mu.RLock()
	defer e.mu.RUnlock()
	
	result := make([]ActivityLogEntry, len(e.ActivityLog))
	copy(result, e.ActivityLog)
	return result
}

// GetWorkHistory returns completed work history (thread-safe)
func (e *Employee) GetWorkHistory() []WorkResult {
	e.mu.RLock()
	defer e.mu.RUnlock()
	
	result := make([]WorkResult, len(e.WorkHistory))
	copy(result, e.WorkHistory)
	return result
}

// Manager oversees employees and reviews quality
type Manager struct {
	ID          uuid.UUID      `json:"id"`
	Name        string         `json:"name"`
	DeptID      uuid.UUID      `json:"dept_id"`
	Employees   []*Employee    `json:"employees"`
	Specialty   EmployeeSkill  `json:"specialty"`
	MaxReports  int            `json:"max_reports"`
	Persona     string         `json:"persona"`
	
	reviewQueue chan *WorkResult
	ctx         context.Context
	cancel      context.CancelFunc
	provider    Provider
	mu          sync.RWMutex
}

// DepartmentHead leads a department
type DepartmentHead struct {
	ID          uuid.UUID   `json:"id"`
	Name        string      `json:"name"`
	Title       string      `json:"title"`
	DivisionID  uuid.UUID   `json:"division_id"`
	Managers    []*Manager  `json:"managers"`
	Objectives  []string    `json:"objectives"`
	Budget      float64     `json:"budget"`
	Persona     string      `json:"persona"`
	
	ctx         context.Context
	cancel      context.CancelFunc
	provider    Provider
	mu          sync.RWMutex
}

// Division represents a major business unit
type Division struct {
	ID          uuid.UUID          `json:"id"`
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Heads       []*DepartmentHead  `json:"department_heads"`
	Budget      float64            `json:"budget"`
	CreatedAt   time.Time          `json:"created_at"`
	
	mu          sync.RWMutex
}

// CompanyStatus represents the operational status
type CompanyStatus string

const (
	CompanyRunning CompanyStatus = "running"
	CompanyPaused  CompanyStatus = "paused"
	CompanyStopped CompanyStatus = "stopped"
)

// BusinessSector represents a target industry/sector for the AI corporation
type BusinessSector string

const (
	SectorRetail           BusinessSector = "retail"
	SectorEntertainment    BusinessSector = "entertainment"
	SectorOnlineServices   BusinessSector = "online_services"
	SectorHardware         BusinessSector = "hardware_engineering"
	SectorFashion          BusinessSector = "fashion"
	SectorFintech          BusinessSector = "fintech"
	SectorHealthcare       BusinessSector = "healthcare"
	SectorEducation        BusinessSector = "education"
	SectorFood             BusinessSector = "food_beverage"
	SectorTravel           BusinessSector = "travel_hospitality"
	SectorRealEstate       BusinessSector = "real_estate"
	SectorMedia            BusinessSector = "media_publishing"
	SectorGaming           BusinessSector = "gaming"
	SectorSaaS             BusinessSector = "saas"
	SectorEcommerce        BusinessSector = "ecommerce"
	SectorSustainability   BusinessSector = "sustainability"
	SectorAI               BusinessSector = "artificial_intelligence"
	SectorCustom           BusinessSector = "custom"
)

// CompanySeed represents the initial configuration to bootstrap the AI corporation
type CompanySeed struct {
	ID              uuid.UUID      `json:"id" db:"id"`
	Sector          BusinessSector `json:"sector" db:"sector"`
	CustomSector    string         `json:"custom_sector,omitempty" db:"custom_sector"`
	CompanyName     string         `json:"company_name" db:"company_name"`
	Mission         string         `json:"mission" db:"mission"`
	Vision          string         `json:"vision" db:"vision"`
	TargetMarket    string         `json:"target_market" db:"target_market"`
	InitialBudget   float64        `json:"initial_budget" db:"initial_budget"`
	Constraints     []string       `json:"constraints" db:"-"`
	Goals           []string       `json:"goals" db:"-"`
	CreatedAt       time.Time      `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at" db:"updated_at"`
	Active          bool           `json:"active" db:"active"`
}

// SectorInfo provides details about a business sector for UI display
type SectorInfo struct {
	ID          BusinessSector `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Examples    []string       `json:"examples"`
}

// GetAvailableSectors returns all available business sectors with descriptions
func GetAvailableSectors() []SectorInfo {
	return []SectorInfo{
		{SectorRetail, "Retail", "Physical and online retail businesses", []string{"Department stores", "Specialty shops", "Pop-up stores"}},
		{SectorEntertainment, "Entertainment", "Media, events, and entertainment services", []string{"Streaming platforms", "Event venues", "Production studios"}},
		{SectorOnlineServices, "Online Services", "Digital services and platforms", []string{"Marketplaces", "Booking platforms", "Social networks"}},
		{SectorHardware, "Hardware Engineering", "Physical product design and manufacturing", []string{"Consumer electronics", "IoT devices", "Industrial equipment"}},
		{SectorFashion, "Fashion", "Clothing, accessories, and fashion brands", []string{"Apparel brands", "Accessories", "Sustainable fashion"}},
		{SectorFintech, "Fintech", "Financial technology and services", []string{"Payment processing", "Digital banking", "Investment platforms"}},
		{SectorHealthcare, "Healthcare", "Health and wellness products and services", []string{"Telemedicine", "Health tracking", "Medical devices"}},
		{SectorEducation, "Education", "Learning and educational services", []string{"Online courses", "EdTech platforms", "Tutoring services"}},
		{SectorFood, "Food & Beverage", "Food production, delivery, and services", []string{"Meal delivery", "Restaurant concepts", "Food products"}},
		{SectorTravel, "Travel & Hospitality", "Travel, tourism, and hospitality", []string{"Booking platforms", "Hotels", "Experience tours"}},
		{SectorRealEstate, "Real Estate", "Property and real estate services", []string{"PropTech", "Property management", "Co-living spaces"}},
		{SectorMedia, "Media & Publishing", "Content creation and publishing", []string{"Digital magazines", "Content platforms", "Newsletters"}},
		{SectorGaming, "Gaming", "Video games and interactive entertainment", []string{"Mobile games", "Console games", "VR/AR experiences"}},
		{SectorSaaS, "SaaS", "Software as a Service products", []string{"Productivity tools", "Business software", "Developer tools"}},
		{SectorEcommerce, "E-commerce", "Online commerce and marketplaces", []string{"DTC brands", "Marketplaces", "Subscription boxes"}},
		{SectorSustainability, "Sustainability", "Green and sustainable businesses", []string{"Clean energy", "Recycling", "Sustainable products"}},
		{SectorAI, "Artificial Intelligence", "AI-powered products and services", []string{"AI assistants", "Automation tools", "ML platforms"}},
		{SectorCustom, "Custom", "Define your own business sector", []string{"Any industry", "Niche markets", "Experimental"}},
	}
}

// Biography represents an editable biography for any person
type Biography struct {
	ID          uuid.UUID `json:"id" db:"id"`
	PersonID    uuid.UUID `json:"person_id" db:"person_id"`
	PersonType  string    `json:"person_type" db:"person_type"` // employee, manager, department_head, board_member
	Name        string    `json:"name" db:"name"`
	Bio         string    `json:"bio" db:"bio"`
	Background  string    `json:"background" db:"background"`
	Personality string    `json:"personality" db:"personality"`
	Goals       []string  `json:"goals" db:"-"`
	Values      []string  `json:"values" db:"-"`
	Quirks      []string  `json:"quirks" db:"-"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
}

// ProductStatus represents the development stage of a product/service
type ProductStatus string

const (
	ProductIdeation    ProductStatus = "ideation"
	ProductPlanning    ProductStatus = "planning"
	ProductDevelopment ProductStatus = "development"
	ProductReview      ProductStatus = "review"
	ProductApproved    ProductStatus = "approved"
	ProductLaunched    ProductStatus = "launched"
	ProductRejected    ProductStatus = "rejected"
)

// Product represents a product or service idea being developed
type Product struct {
	ID          uuid.UUID              `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Category    string                 `json:"category"` // SaaS, API, Platform, Service, etc.
	Status      ProductStatus          `json:"status"`
	TargetMarket string                `json:"target_market"`
	ValueProp   string                 `json:"value_proposition"`
	Features    []string               `json:"features,omitempty"`
	CreatedBy   uuid.UUID              `json:"created_by"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	Deliverables []uuid.UUID           `json:"deliverables,omitempty"` // Related work products
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// DeliverableStatus represents the state of a deliverable
type DeliverableStatus string

const (
	DeliverableInProgress DeliverableStatus = "in_progress"
	DeliverableCompleted  DeliverableStatus = "completed"
	DeliverableInReview   DeliverableStatus = "in_review"
	DeliverableApproved   DeliverableStatus = "approved"
	DeliverableRejected   DeliverableStatus = "rejected"
)

// Deliverable represents a work product
type Deliverable struct {
	ID           uuid.UUID              `json:"id"`
	Title        string                 `json:"title"`
	Type         string                 `json:"type"` // research, design, code, document, etc.
	Description  string                 `json:"description"`
	Output       string                 `json:"output"`
	Status       DeliverableStatus      `json:"status"`
	EmployeeID   uuid.UUID              `json:"employee_id"`
	EmployeeName string                 `json:"employee_name"`
	Skill        string                 `json:"skill"`
	ReviewerID   *uuid.UUID             `json:"reviewer_id,omitempty"`
	ReviewNotes  string                 `json:"review_notes,omitempty"`
	CreatedAt    time.Time              `json:"created_at"`
	CompletedAt  *time.Time             `json:"completed_at,omitempty"`
	ReviewedAt   *time.Time             `json:"reviewed_at,omitempty"`
	Duration     time.Duration          `json:"duration"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// Organization represents the entire company structure
type Organization struct {
	CEO         *DepartmentHead          `json:"ceo"`
	Divisions   map[uuid.UUID]*Division  `json:"divisions"`
	AllEmployees map[uuid.UUID]*Employee `json:"all_employees"`
	AllManagers  map[uuid.UUID]*Manager  `json:"all_managers"`
	Biographies  map[uuid.UUID]*Biography `json:"biographies"`
	Products     map[uuid.UUID]*Product   `json:"products"`
	Deliverables map[uuid.UUID]*Deliverable `json:"deliverables"`
	Seed        *CompanySeed             `json:"seed"`
	Restructuring *RestructuringHistory  `json:"restructuring"`
	CurrentSprint *Sprint                 `json:"current_sprint,omitempty"`
	
	config      *Config
	providers   *ProviderManager
	storage     *StorageManager
	db          *Database
	wsHub       *WSHub
	pipeline    *PipelineManager
	
	// Employee pool management
	minPoolSize     int
	maxPoolSize     int
	scaleThreshold  float64
	
	// Company status
	status          CompanyStatus
	pauseChan       chan struct{}
	resumeChan      chan struct{}
	
	// Concurrency control for LLM requests
	llmSemaphore    chan struct{}
	
	mu              sync.RWMutex
	deliverablesMu  sync.RWMutex // Separate lock for deliverables to reduce contention
	ctx             context.Context
	cancel          context.CancelFunc
	wg              sync.WaitGroup
}

// NewOrganization creates a new organizational structure
func NewOrganization(config *Config, providers *ProviderManager, storage *StorageManager, db *Database) *Organization {
	orgStart := time.Now()
	ctx, cancel := context.WithCancel(context.Background())
	
	org := &Organization{
		Divisions:     make(map[uuid.UUID]*Division),
		AllEmployees:  make(map[uuid.UUID]*Employee),
		AllManagers:   make(map[uuid.UUID]*Manager),
		Biographies:   make(map[uuid.UUID]*Biography),
		Products:      make(map[uuid.UUID]*Product),
		Deliverables:  make(map[uuid.UUID]*Deliverable),
		Restructuring: NewRestructuringHistory(),
		config:        config,
		providers:     providers,
		storage:       storage,
		db:            db,
		minPoolSize:   2,
		maxPoolSize:   50,
		scaleThreshold: 0.8,
		status:        CompanyRunning,
		pauseChan:     make(chan struct{}),
		resumeChan:    make(chan struct{}),
		llmSemaphore:  make(chan struct{}, 5), // Allow 5 concurrent LLM requests for better throughput
		ctx:           ctx,
		cancel:        cancel,
	}
	
	// Load biographies from database
	phaseStart := time.Now()
	org.loadBiographies()
	log.WithField("duration_ms", time.Since(phaseStart).Milliseconds()).Debug("Biographies loaded from database")
	
	// Load seed from database if exists
	phaseStart = time.Now()
	org.loadSeed()
	log.WithField("duration_ms", time.Since(phaseStart).Milliseconds()).Debug("Seed loaded from database")
	
	// Initialize default structure
	phaseStart = time.Now()
	org.initializeStructure()
	log.WithField("duration_ms", time.Since(phaseStart).Milliseconds()).Info("Organization structure initialized")
	
	// Initialize pipeline manager
	phaseStart = time.Now()
	org.pipeline = NewPipelineManager(org)
	log.WithField("duration_ms", time.Since(phaseStart).Milliseconds()).Debug("Pipeline manager initialized")
	
	// Start pipeline if we have a seed loaded from database
	if org.Seed != nil {
		log.Info("Resuming pipeline operation from loaded seed")
		org.pipeline.StartContinuousOperation()
	}
	
	log.WithField("total_duration_ms", time.Since(orgStart).Milliseconds()).Debug("Organization creation complete")
	return org
}

// SetWSHub sets the WebSocket hub for broadcasting events
func (org *Organization) SetWSHub(hub *WSHub) {
	org.wsHub = hub
}

// GetStatus returns the current company status
func (org *Organization) GetStatus() CompanyStatus {
	org.mu.RLock()
	defer org.mu.RUnlock()
	return org.status
}

// Pause pauses all company operations
func (org *Organization) Pause() {
	org.mu.Lock()
	if org.status != CompanyRunning {
		org.mu.Unlock()
		return
	}
	org.status = CompanyPaused
	// Copy employee list while holding lock
	employees := make([]*Employee, 0, len(org.AllEmployees))
	for _, emp := range org.AllEmployees {
		employees = append(employees, emp)
	}
	org.mu.Unlock()
	
	// Pause employees without holding org lock
	// Use defer pattern to ensure unlock even on panic
	for _, emp := range employees {
		func() {
			emp.mu.Lock()
			defer emp.mu.Unlock()
			if emp.Status == EmployeeIdle {
				emp.Status = EmployeePaused
			}
		}()
	}
	
	log.Info("Company paused")
}

// Resume resumes company operations
func (org *Organization) Resume() {
	org.mu.Lock()
	if org.status != CompanyPaused {
		org.mu.Unlock()
		return
	}
	org.status = CompanyRunning
	// Copy employee list while holding lock
	employees := make([]*Employee, 0, len(org.AllEmployees))
	for _, emp := range org.AllEmployees {
		employees = append(employees, emp)
	}
	org.mu.Unlock()
	
	// Resume employees without holding org lock
	// Use defer pattern to ensure unlock even on panic
	for _, emp := range employees {
		func() {
			emp.mu.Lock()
			defer emp.mu.Unlock()
			if emp.Status == EmployeePaused {
				emp.Status = EmployeeIdle
			}
		}()
	}
	
	log.Info("Company resumed")
}

// loadBiographies loads biographies from database
func (org *Organization) loadBiographies() {
	if org.db == nil {
		return
	}
	
	// Biographies will be loaded when needed
	// This is a placeholder for DB integration
}

// GetBiography returns a biography for a person
func (org *Organization) GetBiography(personID uuid.UUID) (*Biography, bool) {
	org.mu.RLock()
	defer org.mu.RUnlock()
	bio, ok := org.Biographies[personID]
	return bio, ok
}

// SetBiography creates or updates a biography
func (org *Organization) SetBiography(bio *Biography) error {
	now := time.Now()
	
	org.mu.Lock()
	if existing, ok := org.Biographies[bio.PersonID]; ok {
		// Update existing
		existing.Name = bio.Name
		existing.Bio = bio.Bio
		existing.Background = bio.Background
		existing.Personality = bio.Personality
		existing.Goals = bio.Goals
		existing.Values = bio.Values
		existing.Quirks = bio.Quirks
		existing.UpdatedAt = now
		bio = existing
	} else {
		// Create new
		bio.ID = uuid.New()
		bio.CreatedAt = now
		bio.UpdatedAt = now
		org.Biographies[bio.PersonID] = bio
	}
	org.mu.Unlock()
	
	// Update the person's persona based on biography (acquires its own locks)
	org.updatePersonaFromBio(bio)
	
	// Save to database (has its own timeout)
	if org.db != nil {
		org.saveBiography(bio)
	}
	
	log.WithFields(log.Fields{
		"person_id":   bio.PersonID,
		"person_type": bio.PersonType,
	}).Info("Biography updated")
	
	return nil
}

// updatePersonaFromBio updates a person's persona based on their biography
func (org *Organization) updatePersonaFromBio(bio *Biography) {
	persona := buildPersonaFromBio(bio)
	
	switch bio.PersonType {
	case "employee":
		if emp, ok := org.AllEmployees[bio.PersonID]; ok {
			emp.mu.Lock()
			emp.Name = bio.Name
			emp.Persona = persona
			emp.mu.Unlock()
		}
	case "manager":
		if mgr, ok := org.AllManagers[bio.PersonID]; ok {
			mgr.mu.Lock()
			mgr.Name = bio.Name
			mgr.Persona = persona
			mgr.mu.Unlock()
		}
	}
}

// buildPersonaFromBio creates an LLM persona from a biography
func buildPersonaFromBio(bio *Biography) string {
	persona := fmt.Sprintf("You are %s.\n\n", bio.Name)
	
	if bio.Bio != "" {
		persona += fmt.Sprintf("ABOUT YOU:\n%s\n\n", bio.Bio)
	}
	
	if bio.Background != "" {
		persona += fmt.Sprintf("BACKGROUND:\n%s\n\n", bio.Background)
	}
	
	if bio.Personality != "" {
		persona += fmt.Sprintf("PERSONALITY:\n%s\n\n", bio.Personality)
	}
	
	if len(bio.Goals) > 0 {
		persona += "YOUR GOALS:\n"
		for _, g := range bio.Goals {
			persona += fmt.Sprintf("- %s\n", g)
		}
		persona += "\n"
	}
	
	if len(bio.Values) > 0 {
		persona += "YOUR VALUES:\n"
		for _, v := range bio.Values {
			persona += fmt.Sprintf("- %s\n", v)
		}
		persona += "\n"
	}
	
	if len(bio.Quirks) > 0 {
		persona += "YOUR QUIRKS:\n"
		for _, q := range bio.Quirks {
			persona += fmt.Sprintf("- %s\n", q)
		}
		persona += "\n"
	}
	
	persona += "Stay in character and make decisions based on your personality, goals, and values."
	
	return persona
}

// saveBiography saves a biography to the database
func (org *Organization) saveBiography(bio *Biography) {
	// Placeholder for database save
	// Will be implemented with database schema
}

// GetAllBiographies returns all biographies
func (org *Organization) GetAllBiographies() []*Biography {
	org.mu.RLock()
	defer org.mu.RUnlock()
	
	bios := make([]*Biography, 0, len(org.Biographies))
	for _, bio := range org.Biographies {
		bios = append(bios, bio)
	}
	return bios
}

// DeleteBiography removes a biography
func (org *Organization) DeleteBiography(personID uuid.UUID) {
	org.mu.Lock()
	defer org.mu.Unlock()
	delete(org.Biographies, personID)
}

// GetSeed returns the current company seed
func (org *Organization) GetSeed() *CompanySeed {
	org.mu.RLock()
	defer org.mu.RUnlock()
	return org.Seed
}

// SetSeed sets the company seed and bootstraps the organization
func (org *Organization) SetSeed(seed *CompanySeed) error {
	now := time.Now()
	seed.ID = uuid.New()
	seed.CreatedAt = now
	seed.UpdatedAt = now
	seed.Active = true
	
	// Generate company mission and vision BEFORE acquiring lock (LLM call can be slow)
	if seed.Mission == "" || seed.Vision == "" {
		org.generateMissionVision(seed)
	}
	
	// Now acquire lock for the quick state updates
	org.mu.Lock()
	
	// Deactivate previous seed if exists
	if org.Seed != nil {
		org.Seed.Active = false
	}
	
	org.Seed = seed
	
	// Notify the organization about new seed
	log.WithFields(log.Fields{
		"sector":      seed.Sector,
		"company":     seed.CompanyName,
		"target":      seed.TargetMarket,
	}).Info("Company seed configured")
	
	// Release lock before database and pipeline operations
	org.mu.Unlock()
	
	// Save to database (has its own timeout)
	if org.db != nil {
		org.saveSeed(seed)
	}
	
	// Start continuous pipeline operation
	org.pipeline.StartContinuousOperation()
	
	return nil
}

// startInitialWork creates initial work items for all employees based on the company seed
func (org *Organization) startInitialWork(seed *CompanySeed) {
	log.WithField("company", seed.CompanyName).Info("Starting initial work assignments")
	
	sectorName := string(seed.Sector)
	if seed.Sector == SectorCustom && seed.CustomSector != "" {
		sectorName = seed.CustomSector
	}
	
	// Define initial work for each skill type based on the company seed
	initialTasks := map[EmployeeSkill][]struct {
		Title       string
		Description string
	}{
		SkillResearch: {
			{Title: "Market Research", Description: fmt.Sprintf("Research the %s market for %s. Identify key competitors, market size, and growth trends.", sectorName, seed.CompanyName)},
			{Title: "Customer Research", Description: fmt.Sprintf("Research the target market: %s. Identify pain points, needs, and preferences.", seed.TargetMarket)},
		},
		SkillAnalysis: {
			{Title: "Competitive Analysis", Description: fmt.Sprintf("Analyze competitors in the %s sector. Identify strengths, weaknesses, and opportunities for %s.", sectorName, seed.CompanyName)},
			{Title: "SWOT Analysis", Description: fmt.Sprintf("Create a comprehensive SWOT analysis for %s entering the %s market.", seed.CompanyName, sectorName)},
		},
		SkillWriting: {
			{Title: "Brand Story", Description: fmt.Sprintf("Write a compelling brand story for %s. Mission: %s. Vision: %s.", seed.CompanyName, seed.Mission, seed.Vision)},
			{Title: "Product Description", Description: fmt.Sprintf("Write product descriptions for %s's core offerings in the %s sector.", seed.CompanyName, sectorName)},
		},
		SkillMarketing: {
			{Title: "Marketing Strategy", Description: fmt.Sprintf("Develop an initial marketing strategy for %s targeting %s.", seed.CompanyName, seed.TargetMarket)},
			{Title: "Social Media Plan", Description: fmt.Sprintf("Create a social media content plan for %s in the %s industry.", seed.CompanyName, sectorName)},
		},
		SkillDesign: {
			{Title: "Brand Identity Concepts", Description: fmt.Sprintf("Design brand identity concepts for %s - a %s company.", seed.CompanyName, sectorName)},
			{Title: "UI/UX Wireframes", Description: fmt.Sprintf("Create initial wireframes for %s's primary product interface.", seed.CompanyName)},
		},
		SkillCoding: {
			{Title: "Technical Architecture", Description: fmt.Sprintf("Design the technical architecture for %s's core platform.", seed.CompanyName)},
			{Title: "MVP Feature List", Description: fmt.Sprintf("Define the MVP feature list for %s with technical specifications.", seed.CompanyName)},
		},
		SkillQA: {
			{Title: "Quality Standards", Description: fmt.Sprintf("Define quality standards and testing criteria for %s products.", seed.CompanyName)},
			{Title: "Process Documentation", Description: fmt.Sprintf("Document QA processes for %s's development workflow.", seed.CompanyName)},
		},
		SkillProjectMgmt: {
			{Title: "Project Roadmap", Description: fmt.Sprintf("Create a 6-month project roadmap for %s launch.", seed.CompanyName)},
			{Title: "Resource Planning", Description: fmt.Sprintf("Develop resource allocation plan for %s's initial phase.", seed.CompanyName)},
		},
		SkillSales: {
			{Title: "Sales Strategy", Description: fmt.Sprintf("Develop initial sales strategy for %s targeting %s.", seed.CompanyName, seed.TargetMarket)},
			{Title: "Pricing Research", Description: fmt.Sprintf("Research pricing models for %s in the %s sector.", seed.CompanyName, sectorName)},
		},
		SkillSupport: {
			{Title: "Support Framework", Description: fmt.Sprintf("Design the customer support framework for %s.", seed.CompanyName)},
			{Title: "FAQ Development", Description: fmt.Sprintf("Develop initial FAQ content for %s's products and services.", seed.CompanyName)},
		},
		SkillDataEntry: {
			{Title: "Data Collection", Description: fmt.Sprintf("Collect and organize industry data for %s's market analysis.", seed.CompanyName)},
			{Title: "Competitor Database", Description: fmt.Sprintf("Build a competitor database for %s in the %s sector.", seed.CompanyName, sectorName)},
		},
	}
	
	// Assign work to employees
	for skill, tasks := range initialTasks {
		for _, task := range tasks {
			work := &WorkItem{
				ID:          uuid.New(),
				Title:       task.Title,
				Description: task.Description,
				Priority:    1,
				CreatedAt:   time.Now(),
			}
			
			if err := org.AssignWork(skill, work); err != nil {
				log.WithFields(log.Fields{
					"skill": skill,
					"task":  task.Title,
					"error": err,
				}).Debug("Could not assign initial work")
			} else {
				log.WithFields(log.Fields{
					"skill": skill,
					"task":  task.Title,
				}).Info("Initial work assigned")
			}
		}
	}
	
	log.WithField("company", seed.CompanyName).Info("Initial work assignments complete")
}

// generateProductIdeas uses LLM to generate product/service ideas for the company
func (org *Organization) generateProductIdeas(seed *CompanySeed) {
	provider, err := org.providers.GetProvider(org.config.DefaultProvider)
	if err != nil || provider == nil {
		log.Warn("No provider available for product ideation, creating placeholder products")
		org.createPlaceholderProducts(seed)
		return
	}
	
	sectorName := string(seed.Sector)
	if seed.Sector == SectorCustom && seed.CustomSector != "" {
		sectorName = seed.CustomSector
	}
	
	// Acquire semaphore for LLM request
	select {
	case org.llmSemaphore <- struct{}{}:
		defer func() { <-org.llmSemaphore }()
	case <-org.ctx.Done():
		log.Warn("Context cancelled, creating placeholder products")
		org.createPlaceholderProducts(seed)
		return
	}
	
	prompt := fmt.Sprintf(`Product strategist for %s (%s sector, target: %s). Mission: %s. Vision: %s.

Generate 3 product ideas. Rules: NO AI/ML/quantum, bootstrappable, tangible.

Format (use --- separator):
PRODUCT: [name]
CATEGORY: [type]
DESCRIPTION: [2 sentences]
VALUE_PROP: [key benefit]
FEATURES: [3-5 items]`, seed.CompanyName, sectorName, seed.TargetMarket, seed.Mission, seed.Vision)
	
	timeout := time.Duration(org.config.LLMTimeoutMinutes) * time.Minute
	ctx, cancel := context.WithTimeout(org.ctx, timeout)
	defer cancel()
	
	llmStart := time.Now()
	log.WithFields(log.Fields{
		"company":   seed.CompanyName,
		"operation": "generate_product_ideas",
	}).Info("Starting LLM request...")
	
	resp, err := provider.Chat(ctx, LLMRequest{
		Messages: []LLMMessage{
			{Role: "user", Content: prompt},
		},
		MaxTokens:   1000, // Reduced from 1500
		Temperature: 0.8,
	})
	
	if err != nil {
		log.WithError(err).WithField("duration_ms", time.Since(llmStart).Milliseconds()).Warn("Failed to generate product ideas, using placeholders")
		org.createPlaceholderProducts(seed)
		return
	}
	
	log.WithFields(log.Fields{
		"company":     seed.CompanyName,
		"duration_ms": time.Since(llmStart).Milliseconds(),
		"length":      len(resp.Content),
	}).Info("LLM request completed")
	
	// Parse the response
	org.parseProductIdeas(seed, resp.Content)
}

// parseProductIdeas parses LLM output into Product structs
func (org *Organization) parseProductIdeas(seed *CompanySeed, content string) {
	products := strings.Split(content, "---")
	count := 0
	
	for _, block := range products {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		
		product := &Product{
			ID:          uuid.New(),
			Status:      ProductIdeation,
			TargetMarket: seed.TargetMarket,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		
		lines := strings.Split(block, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "PRODUCT:") {
				product.Name = strings.TrimSpace(strings.TrimPrefix(line, "PRODUCT:"))
			} else if strings.HasPrefix(line, "CATEGORY:") {
				product.Category = strings.TrimSpace(strings.TrimPrefix(line, "CATEGORY:"))
			} else if strings.HasPrefix(line, "DESCRIPTION:") {
				product.Description = strings.TrimSpace(strings.TrimPrefix(line, "DESCRIPTION:"))
			} else if strings.HasPrefix(line, "VALUE_PROP:") {
				product.ValueProp = strings.TrimSpace(strings.TrimPrefix(line, "VALUE_PROP:"))
			} else if strings.HasPrefix(line, "FEATURES:") {
				featuresStr := strings.TrimSpace(strings.TrimPrefix(line, "FEATURES:"))
				parts := strings.Split(featuresStr, ",")
				for _, p := range parts {
					if f := strings.TrimSpace(p); f != "" {
						product.Features = append(product.Features, f)
					}
				}
			}
		}
		
		if product.Name != "" {
			org.mu.Lock()
			org.Products[product.ID] = product
			org.mu.Unlock()
			count++
			
			log.WithFields(log.Fields{
				"product":  product.Name,
				"category": product.Category,
			}).Info("Product idea generated")
			
			// Broadcast product creation
			if org.wsHub != nil {
				org.wsHub.Broadcast(WebSocketMessage{
					Type: "product_created",
					Payload: map[string]interface{}{
						"id":          product.ID.String(),
						"name":        product.Name,
						"category":    product.Category,
						"description": product.Description,
						"status":      product.Status,
					},
				})
			}
		}
	}
	
	if count == 0 {
		log.Warn("No products parsed from LLM response, creating placeholders")
		org.createPlaceholderProducts(seed)
	} else {
		log.WithField("count", count).Info("Product ideas created")
	}
}

// createPlaceholderProducts creates default product ideas when LLM is unavailable
func (org *Organization) createPlaceholderProducts(seed *CompanySeed) {
	sectorName := string(seed.Sector)
	if seed.Sector == SectorCustom && seed.CustomSector != "" {
		sectorName = seed.CustomSector
	}
	
	placeholders := []struct {
		Name        string
		Category    string
		Description string
		ValueProp   string
	}{
		{
			Name:        fmt.Sprintf("%s Platform", seed.CompanyName),
			Category:    "SaaS Platform",
			Description: fmt.Sprintf("A comprehensive %s platform for %s", sectorName, seed.TargetMarket),
			ValueProp:   "Streamline operations and increase efficiency",
		},
		{
			Name:        fmt.Sprintf("%s API", seed.CompanyName),
			Category:    "API Service",
			Description: fmt.Sprintf("Developer-friendly API for %s integrations", sectorName),
			ValueProp:   "Easy integration with existing systems",
		},
		{
			Name:        fmt.Sprintf("%s Analytics", seed.CompanyName),
			Category:    "Analytics Tool",
			Description: fmt.Sprintf("Data analytics and insights for %s", sectorName),
			ValueProp:   "Data-driven decision making",
		},
	}
	
	for _, p := range placeholders {
		product := &Product{
			ID:           uuid.New(),
			Name:         p.Name,
			Category:     p.Category,
			Description:  p.Description,
			ValueProp:    p.ValueProp,
			Status:       ProductIdeation,
			TargetMarket: seed.TargetMarket,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}
		
		org.mu.Lock()
		org.Products[product.ID] = product
		org.mu.Unlock()
		
		log.WithField("product", product.Name).Info("Placeholder product created")
	}
}

// generateMissionVision uses LLM to generate mission and vision statements
func (org *Organization) generateMissionVision(seed *CompanySeed) {
	provider, err := org.providers.GetProvider(org.config.DefaultProvider)
	if err != nil || provider == nil {
		// Use defaults
		seed.Mission = fmt.Sprintf("To innovate and lead in the %s sector", seed.Sector)
		seed.Vision = fmt.Sprintf("Becoming the most trusted name in %s", seed.Sector)
		return
	}
	
	sectorName := string(seed.Sector)
	if seed.Sector == SectorCustom && seed.CustomSector != "" {
		sectorName = seed.CustomSector
	}
	
	prompt := fmt.Sprintf(`You are helping to bootstrap a new AI-powered company.

Business Sector: %s
Company Name: %s
Target Market: %s

Generate a compelling mission statement and vision statement for this company.
Keep each statement concise (1-2 sentences).

Respond in this exact format:
MISSION: [mission statement]
VISION: [vision statement]`, sectorName, seed.CompanyName, seed.TargetMarket)

	resp, err := provider.Chat(org.ctx, LLMRequest{
		Messages: []LLMMessage{
			{Role: "user", Content: prompt},
		},
		MaxTokens:   200,
		Temperature: 0.7,
	})
	
	if err != nil || resp.Content == "" {
		seed.Mission = fmt.Sprintf("To innovate and lead in the %s sector", sectorName)
		seed.Vision = fmt.Sprintf("Becoming the most trusted name in %s", sectorName)
		return
	}
	
	// Parse response
	lines := strings.Split(resp.Content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "MISSION:") {
			seed.Mission = strings.TrimSpace(strings.TrimPrefix(line, "MISSION:"))
		} else if strings.HasPrefix(line, "VISION:") {
			seed.Vision = strings.TrimSpace(strings.TrimPrefix(line, "VISION:"))
		}
	}
	
	// Fallback if parsing failed
	if seed.Mission == "" {
		seed.Mission = fmt.Sprintf("To innovate and lead in the %s sector", sectorName)
	}
	if seed.Vision == "" {
		seed.Vision = fmt.Sprintf("Becoming the most trusted name in %s", sectorName)
	}
}

// saveSeed saves the seed to the database
func (org *Organization) saveSeed(seed *CompanySeed) {
	// Use a timeout context to prevent blocking
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	query := `
		INSERT INTO company_seeds (
			id, sector, custom_sector, company_name, mission, vision, 
			target_market, initial_budget, active, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (id) DO UPDATE SET
			sector = EXCLUDED.sector,
			custom_sector = EXCLUDED.custom_sector,
			company_name = EXCLUDED.company_name,
			mission = EXCLUDED.mission,
			vision = EXCLUDED.vision,
			target_market = EXCLUDED.target_market,
			initial_budget = EXCLUDED.initial_budget,
			active = EXCLUDED.active,
			updated_at = EXCLUDED.updated_at
	`
	
	_, err := org.db.db.ExecContext(ctx, query,
		seed.ID, seed.Sector, seed.CustomSector, seed.CompanyName,
		seed.Mission, seed.Vision, seed.TargetMarket, seed.InitialBudget,
		seed.Active, seed.CreatedAt, seed.UpdatedAt,
	)
	
	if err != nil {
		log.WithError(err).Error("Failed to save company seed")
	} else {
		log.WithField("seed_id", seed.ID).Info("Company seed saved to database")
	}
}

// loadSeed loads the active seed from the database
func (org *Organization) loadSeed() {
	if org.db == nil {
		return
	}
	
	// Use a timeout context to prevent blocking indefinitely
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	query := `
		SELECT id, sector, custom_sector, company_name, mission, vision, 
		       target_market, initial_budget, active, created_at, updated_at
		FROM company_seeds
		WHERE active = true
		ORDER BY created_at DESC
		LIMIT 1
	`
	
	seed := &CompanySeed{}
	err := org.db.db.QueryRowContext(ctx, query).Scan(
		&seed.ID, &seed.Sector, &seed.CustomSector, &seed.CompanyName,
		&seed.Mission, &seed.Vision, &seed.TargetMarket, &seed.InitialBudget,
		&seed.Active, &seed.CreatedAt, &seed.UpdatedAt,
	)
	
	if err != nil {
		if err != sql.ErrNoRows {
			log.WithError(err).Warn("Failed to load company seed from database")
		}
		return
	}
	
	org.Seed = seed
	log.WithFields(log.Fields{
		"seed_id": seed.ID,
		"company": seed.CompanyName,
		"sector":  seed.Sector,
	}).Info("Company seed loaded from database")
}

// GetSeedContext returns context for LLM prompts based on the current seed
func (org *Organization) GetSeedContext() string {
	org.mu.RLock()
	defer org.mu.RUnlock()
	
	if org.Seed == nil {
		return "No business context configured. Operating in general mode."
	}
	
	seed := org.Seed
	sectorName := string(seed.Sector)
	if seed.Sector == SectorCustom && seed.CustomSector != "" {
		sectorName = seed.CustomSector
	}
	
	ctx := fmt.Sprintf(`COMPANY CONTEXT:
Company Name: %s
Business Sector: %s
Target Market: %s
Mission: %s
Vision: %s`, seed.CompanyName, sectorName, seed.TargetMarket, seed.Mission, seed.Vision)
	
	if len(seed.Goals) > 0 {
		ctx += "\n\nCOMPANY GOALS:\n"
		for _, g := range seed.Goals {
			ctx += "- " + g + "\n"
		}
	}
	
	if len(seed.Constraints) > 0 {
		ctx += "\nCONSTRAINTS:\n"
		for _, c := range seed.Constraints {
			ctx += "- " + c + "\n"
		}
	}
	
	return ctx
}

// IsSeeded returns true if the organization has been seeded
func (org *Organization) IsSeeded() bool {
	org.mu.RLock()
	defer org.mu.RUnlock()
	return org.Seed != nil && org.Seed.Active
}

// RestructuringType represents the type of organizational change
type RestructuringType string

const (
	RestructureCreateDivision  RestructuringType = "create_division"
	RestructureMergeDivisions  RestructuringType = "merge_divisions"
	RestructureDissolveDivision RestructuringType = "dissolve_division"
	RestructureCreateDepartment RestructuringType = "create_department"
	RestructureMergeDepartments RestructuringType = "merge_departments"
	RestructureReassignEmployee RestructuringType = "reassign_employee"
	RestructurePromote          RestructuringType = "promote"
	RestructureDemote           RestructuringType = "demote"
	RestructureHire             RestructuringType = "hire"
	RestructureTerminate        RestructuringType = "terminate"
	RestructureScaleTeam        RestructuringType = "scale_team"
	RestructureReorganize       RestructuringType = "reorganize"
)

// RestructuringStatus represents the status of a restructuring proposal
type RestructuringStatus string

const (
	RestructuringPending   RestructuringStatus = "pending"
	RestructuringApproved  RestructuringStatus = "approved"
	RestructuringRejected  RestructuringStatus = "rejected"
	RestructuringExecuted  RestructuringStatus = "executed"
	RestructuringFailed    RestructuringStatus = "failed"
)

// RestructuringProposal represents a proposed organizational change
type RestructuringProposal struct {
	ID            uuid.UUID           `json:"id"`
	Type          RestructuringType   `json:"type"`
	Status        RestructuringStatus `json:"status"`
	ProposedBy    uuid.UUID           `json:"proposed_by"`
	ProposerName  string              `json:"proposer_name"`
	ProposerRole  string              `json:"proposer_role"`
	Title         string              `json:"title"`
	Rationale     string              `json:"rationale"`
	ExpectedBenefit string            `json:"expected_benefit"`
	RiskAssessment string             `json:"risk_assessment"`
	AffectedUnits []uuid.UUID         `json:"affected_units"`
	AffectedPeople []uuid.UUID        `json:"affected_people"`
	Parameters    map[string]interface{} `json:"parameters"`
	Votes         map[uuid.UUID]bool  `json:"votes"`
	VoteComments  map[uuid.UUID]string `json:"vote_comments"`
	CreatedAt     time.Time           `json:"created_at"`
	DecidedAt     *time.Time          `json:"decided_at,omitempty"`
	ExecutedAt    *time.Time          `json:"executed_at,omitempty"`
	ExecutionLog  []string            `json:"execution_log"`
}

// RestructuringHistory tracks all restructuring events
type RestructuringHistory struct {
	Proposals []RestructuringProposal `json:"proposals"`
	mu        sync.RWMutex
}

// NewRestructuringHistory creates a new history tracker
func NewRestructuringHistory() *RestructuringHistory {
	return &RestructuringHistory{
		Proposals: make([]RestructuringProposal, 0),
	}
}

// Add adds a proposal to history
func (h *RestructuringHistory) Add(p RestructuringProposal) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Proposals = append(h.Proposals, p)
}

// Get returns a proposal by ID
func (h *RestructuringHistory) Get(id uuid.UUID) *RestructuringProposal {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for i := range h.Proposals {
		if h.Proposals[i].ID == id {
			return &h.Proposals[i]
		}
	}
	return nil
}

// Update updates a proposal in history
func (h *RestructuringHistory) Update(p RestructuringProposal) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for i := range h.Proposals {
		if h.Proposals[i].ID == p.ID {
			h.Proposals[i] = p
			return
		}
	}
}

// GetPending returns all pending proposals
func (h *RestructuringHistory) GetPending() []RestructuringProposal {
	h.mu.RLock()
	defer h.mu.RUnlock()
	pending := make([]RestructuringProposal, 0)
	for _, p := range h.Proposals {
		if p.Status == RestructuringPending {
			pending = append(pending, p)
		}
	}
	return pending
}

// GetRecent returns the most recent n proposals
func (h *RestructuringHistory) GetRecent(n int) []RestructuringProposal {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if n > len(h.Proposals) {
		n = len(h.Proposals)
	}
	// Return in reverse order (most recent first)
	result := make([]RestructuringProposal, n)
	for i := 0; i < n; i++ {
		result[i] = h.Proposals[len(h.Proposals)-1-i]
	}
	return result
}

// ProposeRestructuring creates a new restructuring proposal
func (org *Organization) ProposeRestructuring(
	proposerID uuid.UUID,
	proposerName string,
	proposerRole string,
	restructureType RestructuringType,
	title string,
	rationale string,
	params map[string]interface{},
) (*RestructuringProposal, error) {
	proposal := &RestructuringProposal{
		ID:            uuid.New(),
		Type:          restructureType,
		Status:        RestructuringPending,
		ProposedBy:    proposerID,
		ProposerName:  proposerName,
		ProposerRole:  proposerRole,
		Title:         title,
		Rationale:     rationale,
		Parameters:    params,
		Votes:         make(map[uuid.UUID]bool),
		VoteComments:  make(map[uuid.UUID]string),
		CreatedAt:     time.Now(),
		ExecutionLog:  make([]string, 0),
	}
	
	// Use LLM to analyze expected benefits and risks
	org.analyzeRestructuringProposal(proposal)
	
	org.Restructuring.Add(*proposal)
	
	log.WithFields(log.Fields{
		"proposal_id": proposal.ID,
		"type":        proposal.Type,
		"proposer":    proposerName,
		"title":       title,
	}).Info("Restructuring proposal created")
	
	return proposal, nil
}

// analyzeRestructuringProposal uses LLM to assess benefits and risks
func (org *Organization) analyzeRestructuringProposal(proposal *RestructuringProposal) {
	provider, err := org.providers.GetProvider(org.config.DefaultProvider)
	if err != nil || provider == nil {
		proposal.ExpectedBenefit = "Analysis unavailable"
		proposal.RiskAssessment = "Analysis unavailable"
		return
	}
	
	seedContext := org.GetSeedContext()
	
	prompt := fmt.Sprintf(`You are a senior business consultant analyzing a proposed organizational restructuring.

%s

PROPOSAL:
Type: %s
Title: %s
Rationale: %s
Proposed By: %s (%s)

Analyze this proposal and provide:
1. EXPECTED_BENEFIT: A concise statement of the expected positive outcomes (1-2 sentences)
2. RISK_ASSESSMENT: Key risks and potential negative impacts (1-2 sentences)

Respond in exactly this format:
EXPECTED_BENEFIT: [your analysis]
RISK_ASSESSMENT: [your analysis]`,
		seedContext, proposal.Type, proposal.Title, proposal.Rationale, proposal.ProposerName, proposal.ProposerRole)
	
	resp, err := provider.Chat(org.ctx, LLMRequest{
		Messages: []LLMMessage{
			{Role: "user", Content: prompt},
		},
		MaxTokens:   300,
		Temperature: 0.5,
	})
	
	if err != nil || resp.Content == "" {
		proposal.ExpectedBenefit = "Improved organizational efficiency and alignment with strategic goals"
		proposal.RiskAssessment = "Potential temporary disruption to operations during transition"
		return
	}
	
	lines := strings.Split(resp.Content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "EXPECTED_BENEFIT:") {
			proposal.ExpectedBenefit = strings.TrimSpace(strings.TrimPrefix(line, "EXPECTED_BENEFIT:"))
		} else if strings.HasPrefix(line, "RISK_ASSESSMENT:") {
			proposal.RiskAssessment = strings.TrimSpace(strings.TrimPrefix(line, "RISK_ASSESSMENT:"))
		}
	}
	
	if proposal.ExpectedBenefit == "" {
		proposal.ExpectedBenefit = "Improved organizational efficiency"
	}
	if proposal.RiskAssessment == "" {
		proposal.RiskAssessment = "Standard transition risks"
	}
}

// VoteOnRestructuring allows an executive to vote on a proposal
func (org *Organization) VoteOnRestructuring(proposalID uuid.UUID, voterID uuid.UUID, approve bool, comment string) error {
	proposal := org.Restructuring.Get(proposalID)
	if proposal == nil {
		return fmt.Errorf("proposal not found: %s", proposalID)
	}
	
	if proposal.Status != RestructuringPending {
		return fmt.Errorf("proposal is no longer pending: %s", proposal.Status)
	}
	
	proposal.Votes[voterID] = approve
	if comment != "" {
		proposal.VoteComments[voterID] = comment
	}
	
	org.Restructuring.Update(*proposal)
	
	// Check if we have enough votes to decide
	org.evaluateRestructuringVotes(proposal)
	
	return nil
}

// evaluateRestructuringVotes checks if a proposal should be approved or rejected
func (org *Organization) evaluateRestructuringVotes(proposal *RestructuringProposal) {
	// Need majority of executives to approve
	// For now, we'll require at least 2 votes with majority approval
	if len(proposal.Votes) < 2 {
		return
	}
	
	approvals := 0
	rejections := 0
	for _, vote := range proposal.Votes {
		if vote {
			approvals++
		} else {
			rejections++
		}
	}
	
	now := time.Now()
	if approvals > rejections {
		proposal.Status = RestructuringApproved
		proposal.DecidedAt = &now
		proposal.ExecutionLog = append(proposal.ExecutionLog, fmt.Sprintf("Approved at %s with %d approvals, %d rejections", now.Format(time.RFC3339), approvals, rejections))
		log.WithFields(log.Fields{
			"proposal_id": proposal.ID,
			"approvals":   approvals,
			"rejections":  rejections,
		}).Info("Restructuring proposal approved")
	} else if rejections > approvals {
		proposal.Status = RestructuringRejected
		proposal.DecidedAt = &now
		proposal.ExecutionLog = append(proposal.ExecutionLog, fmt.Sprintf("Rejected at %s with %d approvals, %d rejections", now.Format(time.RFC3339), approvals, rejections))
		log.WithFields(log.Fields{
			"proposal_id": proposal.ID,
			"approvals":   approvals,
			"rejections":  rejections,
		}).Info("Restructuring proposal rejected")
	}
	
	org.Restructuring.Update(*proposal)
}

// ExecuteRestructuring executes an approved restructuring proposal
func (org *Organization) ExecuteRestructuring(proposalID uuid.UUID) error {
	proposal := org.Restructuring.Get(proposalID)
	if proposal == nil {
		return fmt.Errorf("proposal not found: %s", proposalID)
	}
	
	if proposal.Status != RestructuringApproved {
		return fmt.Errorf("proposal must be approved before execution: %s", proposal.Status)
	}
	
	org.mu.Lock()
	defer org.mu.Unlock()
	
	var err error
	switch proposal.Type {
	case RestructureCreateDivision:
		err = org.executeCreateDivision(proposal)
	case RestructureMergeDivisions:
		err = org.executeMergeDivisions(proposal)
	case RestructureDissolveDivision:
		err = org.executeDissolveDivision(proposal)
	case RestructureCreateDepartment:
		err = org.executeCreateDepartment(proposal)
	case RestructureReassignEmployee:
		err = org.executeReassignEmployee(proposal)
	case RestructurePromote:
		err = org.executePromote(proposal)
	case RestructureHire:
		err = org.executeHire(proposal)
	case RestructureTerminate:
		err = org.executeTerminate(proposal)
	case RestructureScaleTeam:
		err = org.executeScaleTeam(proposal)
	default:
		err = fmt.Errorf("unsupported restructuring type: %s", proposal.Type)
	}
	
	now := time.Now()
	if err != nil {
		proposal.Status = RestructuringFailed
		proposal.ExecutionLog = append(proposal.ExecutionLog, fmt.Sprintf("Execution failed at %s: %s", now.Format(time.RFC3339), err.Error()))
		org.Restructuring.Update(*proposal)
		return err
	}
	
	proposal.Status = RestructuringExecuted
	proposal.ExecutedAt = &now
	proposal.ExecutionLog = append(proposal.ExecutionLog, fmt.Sprintf("Successfully executed at %s", now.Format(time.RFC3339)))
	org.Restructuring.Update(*proposal)
	
	log.WithFields(log.Fields{
		"proposal_id": proposal.ID,
		"type":        proposal.Type,
	}).Info("Restructuring executed successfully")
	
	return nil
}

// Execution methods for each restructuring type

func (org *Organization) executeCreateDivision(proposal *RestructuringProposal) error {
	name, _ := proposal.Parameters["name"].(string)
	description, _ := proposal.Parameters["description"].(string)
	
	if name == "" {
		return fmt.Errorf("division name is required")
	}
	
	division := &Division{
		ID:          uuid.New(),
		Name:        name,
		Description: description,
		Heads:       make([]*DepartmentHead, 0),
		CreatedAt:   time.Now(),
	}
	
	org.Divisions[division.ID] = division
	proposal.ExecutionLog = append(proposal.ExecutionLog, fmt.Sprintf("Created division: %s (%s)", name, division.ID))
	return nil
}

func (org *Organization) executeMergeDivisions(proposal *RestructuringProposal) error {
	sourceIDs, _ := proposal.Parameters["source_ids"].([]interface{})
	targetName, _ := proposal.Parameters["target_name"].(string)
	
	if len(sourceIDs) < 2 {
		return fmt.Errorf("need at least 2 divisions to merge")
	}
	
	// Create new merged division
	merged := &Division{
		ID:        uuid.New(),
		Name:      targetName,
		Heads:     make([]*DepartmentHead, 0),
		CreatedAt: time.Now(),
	}
	
	// Move department heads from source divisions
	for _, idRaw := range sourceIDs {
		idStr, ok := idRaw.(string)
		if !ok {
			continue
		}
		sourceID, err := uuid.Parse(idStr)
		if err != nil {
			continue
		}
		if source, exists := org.Divisions[sourceID]; exists {
			for _, head := range source.Heads {
				head.DivisionID = merged.ID
				merged.Heads = append(merged.Heads, head)
			}
			delete(org.Divisions, sourceID)
			proposal.ExecutionLog = append(proposal.ExecutionLog, fmt.Sprintf("Merged division %s into %s", source.Name, targetName))
		}
	}
	
	org.Divisions[merged.ID] = merged
	return nil
}

func (org *Organization) executeDissolveDivision(proposal *RestructuringProposal) error {
	divisionIDStr, _ := proposal.Parameters["division_id"].(string)
	divisionID, err := uuid.Parse(divisionIDStr)
	if err != nil {
		return fmt.Errorf("invalid division ID: %s", divisionIDStr)
	}
	
	division, exists := org.Divisions[divisionID]
	if !exists {
		return fmt.Errorf("division not found: %s", divisionID)
	}
	
	// Move employees to unassigned
	for _, head := range division.Heads {
		for _, mgr := range head.Managers {
			for _, emp := range mgr.Employees {
				emp.Status = EmployeeIdle
				proposal.ExecutionLog = append(proposal.ExecutionLog, fmt.Sprintf("Employee %s now unassigned", emp.Name))
			}
		}
	}
	
	delete(org.Divisions, divisionID)
	proposal.ExecutionLog = append(proposal.ExecutionLog, fmt.Sprintf("Dissolved division: %s", division.Name))
	return nil
}

func (org *Organization) executeCreateDepartment(proposal *RestructuringProposal) error {
	divisionIDStr, _ := proposal.Parameters["division_id"].(string)
	name, _ := proposal.Parameters["name"].(string)
	title, _ := proposal.Parameters["title"].(string)
	
	divisionID, err := uuid.Parse(divisionIDStr)
	if err != nil {
		return fmt.Errorf("invalid division ID")
	}
	
	division, exists := org.Divisions[divisionID]
	if !exists {
		return fmt.Errorf("division not found")
	}
	
	if title == "" {
		title = "Head of " + name
	}
	
	head := &DepartmentHead{
		ID:         uuid.New(),
		Name:       name + " Head",
		Title:      title,
		DivisionID: divisionID,
		Managers:   make([]*Manager, 0),
		Objectives: []string{},
	}
	
	division.Heads = append(division.Heads, head)
	proposal.ExecutionLog = append(proposal.ExecutionLog, fmt.Sprintf("Created department %s in %s", name, division.Name))
	return nil
}

func (org *Organization) executeReassignEmployee(proposal *RestructuringProposal) error {
	employeeIDStr, _ := proposal.Parameters["employee_id"].(string)
	targetManagerIDStr, _ := proposal.Parameters["target_manager_id"].(string)
	
	employeeID, err := uuid.Parse(employeeIDStr)
	if err != nil {
		return fmt.Errorf("invalid employee ID")
	}
	
	employee, exists := org.AllEmployees[employeeID]
	if !exists {
		return fmt.Errorf("employee not found")
	}
	
	targetManagerID, err := uuid.Parse(targetManagerIDStr)
	if err != nil {
		return fmt.Errorf("invalid target manager ID")
	}
	
	// Find and remove from current manager
	for _, mgr := range org.AllManagers {
		for i, emp := range mgr.Employees {
			if emp.ID == employeeID {
				mgr.Employees = append(mgr.Employees[:i], mgr.Employees[i+1:]...)
				break
			}
		}
	}
	
	// Add to target manager
	if targetMgr, exists := org.AllManagers[targetManagerID]; exists {
		targetMgr.Employees = append(targetMgr.Employees, employee)
		employee.ManagerID = targetManagerID
		proposal.ExecutionLog = append(proposal.ExecutionLog, fmt.Sprintf("Reassigned %s to manager %s", employee.Name, targetMgr.Name))
		return nil
	}
	
	return fmt.Errorf("target manager not found")
}

func (org *Organization) executePromote(proposal *RestructuringProposal) error {
	employeeIDStr, _ := proposal.Parameters["employee_id"].(string)
	newRole, _ := proposal.Parameters["new_role"].(string)
	
	employeeID, err := uuid.Parse(employeeIDStr)
	if err != nil {
		return fmt.Errorf("invalid employee ID")
	}
	
	employee, exists := org.AllEmployees[employeeID]
	if !exists {
		return fmt.Errorf("employee not found")
	}
	
	oldSkill := employee.Skill
	// Upgrade skill level based on new role
	if newRole == "manager" {
		// Create a new manager from this employee
		manager := &Manager{
			ID:         uuid.New(),
			Name:       employee.Name,
			Specialty:  employee.Skill,
			MaxReports: 5,
			Employees:  make([]*Employee, 0),
		}
		org.AllManagers[manager.ID] = manager
		delete(org.AllEmployees, employeeID)
		proposal.ExecutionLog = append(proposal.ExecutionLog, fmt.Sprintf("Promoted %s from %s to Manager", employee.Name, oldSkill))
	} else {
		proposal.ExecutionLog = append(proposal.ExecutionLog, fmt.Sprintf("Promoted %s in role", employee.Name))
	}
	
	return nil
}

func (org *Organization) executeHire(proposal *RestructuringProposal) error {
	name, _ := proposal.Parameters["name"].(string)
	skillStr, _ := proposal.Parameters["skill"].(string)
	managerIDStr, _ := proposal.Parameters["manager_id"].(string)
	
	if name == "" {
		name = generateEmployeeName()
	}
	
	skill := EmployeeSkill(skillStr)
	if skill == "" {
		skill = SkillWriting // default skill
	}
	
	employee := &Employee{
		ID:      uuid.New(),
		Name:    name,
		Skill:   skill,
		Status:  EmployeeIdle,
		HiredAt: time.Now(),
	}
	
	org.AllEmployees[employee.ID] = employee
	
	// Add to manager if specified
	if managerIDStr != "" {
		managerID, _ := uuid.Parse(managerIDStr)
		if mgr, exists := org.AllManagers[managerID]; exists {
			mgr.Employees = append(mgr.Employees, employee)
			employee.ManagerID = managerID
		}
	}
	
	proposal.ExecutionLog = append(proposal.ExecutionLog, fmt.Sprintf("Hired new employee: %s (%s)", name, skill))
	return nil
}

func (org *Organization) executeTerminate(proposal *RestructuringProposal) error {
	employeeIDStr, _ := proposal.Parameters["employee_id"].(string)
	
	employeeID, err := uuid.Parse(employeeIDStr)
	if err != nil {
		return fmt.Errorf("invalid employee ID")
	}
	
	employee, exists := org.AllEmployees[employeeID]
	if !exists {
		return fmt.Errorf("employee not found")
	}
	
	// Remove from manager's team
	for _, mgr := range org.AllManagers {
		for i, emp := range mgr.Employees {
			if emp.ID == employeeID {
				mgr.Employees = append(mgr.Employees[:i], mgr.Employees[i+1:]...)
				break
			}
		}
	}
	
	delete(org.AllEmployees, employeeID)
	proposal.ExecutionLog = append(proposal.ExecutionLog, fmt.Sprintf("Terminated employee: %s", employee.Name))
	return nil
}

func (org *Organization) executeScaleTeam(proposal *RestructuringProposal) error {
	managerIDStr, _ := proposal.Parameters["manager_id"].(string)
	targetSize, _ := proposal.Parameters["target_size"].(float64)
	skillStr, _ := proposal.Parameters["skill"].(string)
	
	managerID, err := uuid.Parse(managerIDStr)
	if err != nil {
		return fmt.Errorf("invalid manager ID")
	}
	
	skill := EmployeeSkill(skillStr)
	if skill == "" {
		skill = SkillWriting // default skill
	}
	
	// Find manager
	targetMgr, exists := org.AllManagers[managerID]
	if !exists {
		return fmt.Errorf("manager not found")
	}
	
	currentSize := len(targetMgr.Employees)
	targetSizeInt := int(targetSize)
	
	if targetSizeInt > currentSize {
		// Hire more employees
		for i := currentSize; i < targetSizeInt; i++ {
			emp := &Employee{
				ID:        uuid.New(),
				Name:      generateEmployeeName(),
				Skill:     skill,
				Status:    EmployeeIdle,
				ManagerID: managerID,
				HiredAt:   time.Now(),
			}
			org.AllEmployees[emp.ID] = emp
			targetMgr.Employees = append(targetMgr.Employees, emp)
			proposal.ExecutionLog = append(proposal.ExecutionLog, fmt.Sprintf("Hired %s for scaling", emp.Name))
		}
	} else if targetSizeInt < currentSize {
		// Reduce team size (move to unassigned, not terminate)
		count := 0
		for len(targetMgr.Employees) > targetSizeInt && count < currentSize-targetSizeInt {
			emp := targetMgr.Employees[len(targetMgr.Employees)-1]
			targetMgr.Employees = targetMgr.Employees[:len(targetMgr.Employees)-1]
			emp.ManagerID = uuid.Nil
			proposal.ExecutionLog = append(proposal.ExecutionLog, fmt.Sprintf("Removed employee %s from team", emp.Name))
			count++
		}
	}
	
	proposal.ExecutionLog = append(proposal.ExecutionLog, fmt.Sprintf("Scaled %s's team from %d to %d employees", targetMgr.Name, currentSize, targetSizeInt))
	return nil
}

// generateEmployeeName creates a random employee name
func generateEmployeeName() string {
	firstNames := []string{"Alex", "Jordan", "Casey", "Morgan", "Taylor", "Riley", "Avery", "Quinn", "Skyler", "Dakota"}
	lastNames := []string{"Smith", "Chen", "Patel", "Kim", "Garcia", "Johnson", "Williams", "Brown", "Jones", "Davis"}
	return firstNames[time.Now().UnixNano()%int64(len(firstNames))] + " " + lastNames[time.Now().UnixNano()%int64(len(lastNames))]
}

// AnalyzeOrganizationHealth uses LLM to assess if restructuring is needed
func (org *Organization) AnalyzeOrganizationHealth() (map[string]interface{}, error) {
	provider, err := org.providers.GetProvider(org.config.DefaultProvider)
	if err != nil {
		return nil, err
	}
	
	stats := org.GetStats()
	seedContext := org.GetSeedContext()
	
	prompt := fmt.Sprintf(`You are an organizational consultant analyzing company health.

%s

CURRENT ORGANIZATION STATS:
- Total Employees: %d
- Divisions: %d
- Working: %d
- Idle: %d

Analyze the organization and determine if restructuring might be beneficial.
Consider:
1. Are there too many idle employees?
2. Is the structure aligned with the business sector?
3. Are there opportunities to improve efficiency?

Respond in this exact JSON format:
{
  "health_score": 0-100,
  "restructuring_recommended": true/false,
  "recommendations": ["recommendation 1", "recommendation 2"],
  "areas_of_concern": ["concern 1"],
  "strengths": ["strength 1"]
}`,
		seedContext,
		stats["total_employees"],
		stats["divisions"],
		stats["working"],
		stats["idle"])
	
	resp, err := provider.Chat(org.ctx, LLMRequest{
		Messages: []LLMMessage{
			{Role: "user", Content: prompt},
		},
		MaxTokens:   500,
		Temperature: 0.3,
	})
	
	if err != nil {
		return nil, err
	}
	
	// Parse JSON response
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(resp.Content), &result); err != nil {
		// Return basic analysis if parsing fails
		return map[string]interface{}{
			"health_score":              75,
			"restructuring_recommended": false,
			"recommendations":           []string{"Continue monitoring organization performance"},
			"raw_analysis":              resp.Content,
		}, nil
	}
	
	return result, nil
}

// GetRestructuringHistory returns the restructuring history
func (org *Organization) GetRestructuringHistory() *RestructuringHistory {
	return org.Restructuring
}

// initializeStructure creates the default organizational tree
func (org *Organization) initializeStructure() {
	provider, _ := org.providers.GetProvider(org.config.DefaultProvider)
	
	// Create CEO
	org.CEO = &DepartmentHead{
		ID:    uuid.New(),
		Name:  "AI Executive",
		Title: "Chief Executive Officer",
		Objectives: []string{
			"Drive company growth",
			"Ensure operational excellence",
			"Maintain strategic vision",
		},
		Persona: `You are the CEO. You set direction, approve major initiatives, and ensure all divisions work toward company goals. You delegate to department heads and expect results.`,
	}
	org.CEO.ctx, org.CEO.cancel = context.WithCancel(org.ctx)
	org.CEO.provider = provider
	
	// Create default divisions
	divisions := []struct {
		name        string
		description string
		departments []struct {
			title    string
			skills   []EmployeeSkill
		}
	}{
		{
			name:        "Technology",
			description: "Engineering, R&D, and technical operations",
			departments: []struct {
				title  string
				skills []EmployeeSkill
			}{
				{"Engineering", []EmployeeSkill{SkillCoding, SkillQA}},
				{"Research", []EmployeeSkill{SkillResearch, SkillAnalysis}},
			},
		},
		{
			name:        "Marketing",
			description: "Brand, content, and growth",
			departments: []struct {
				title  string
				skills []EmployeeSkill
			}{
				{"Content", []EmployeeSkill{SkillWriting, SkillDesign}},
				{"Growth", []EmployeeSkill{SkillMarketing, SkillAnalysis}},
			},
		},
		{
			name:        "Operations",
			description: "Day-to-day operations and support",
			departments: []struct {
				title  string
				skills []EmployeeSkill
			}{
				{"Customer Success", []EmployeeSkill{SkillSupport, SkillSales}},
				{"Project Management", []EmployeeSkill{SkillProjectMgmt, SkillDataEntry}},
			},
		},
	}
	
	for _, divDef := range divisions {
		div := org.CreateDivision(divDef.name, divDef.description)
		
		for _, deptDef := range divDef.departments {
			head := org.CreateDepartmentHead(div.ID, deptDef.title)
			
			// Create a manager for each skill
			for _, skill := range deptDef.skills {
				manager := org.CreateManager(head.ID, skill)
				
				// Create initial employees
				for i := 0; i < org.minPoolSize; i++ {
					org.CreateEmployee(manager.ID, skill)
				}
			}
		}
	}
	
	log.WithFields(log.Fields{
		"divisions":  len(org.Divisions),
		"employees":  len(org.AllEmployees),
		"managers":   len(org.AllManagers),
	}).Info("Organization structure initialized")
}

// CreateDivision creates a new division
func (org *Organization) CreateDivision(name, description string) *Division {
	org.mu.Lock()
	defer org.mu.Unlock()
	
	div := &Division{
		ID:          uuid.New(),
		Name:        name,
		Description: description,
		Heads:       make([]*DepartmentHead, 0),
		CreatedAt:   time.Now(),
	}
	
	org.Divisions[div.ID] = div
	
	log.WithFields(log.Fields{
		"division_id":   div.ID,
		"division_name": name,
	}).Info("Division created")
	
	return div
}

// CreateDepartmentHead creates a new department head
func (org *Organization) CreateDepartmentHead(divisionID uuid.UUID, title string) *DepartmentHead {
	org.mu.Lock()
	defer org.mu.Unlock()
	
	provider, _ := org.providers.GetProvider(org.config.DefaultProvider)
	
	head := &DepartmentHead{
		ID:         uuid.New(),
		Name:       fmt.Sprintf("%s Director", title),
		Title:      fmt.Sprintf("Head of %s", title),
		DivisionID: divisionID,
		Managers:   make([]*Manager, 0),
		Objectives: []string{},
		Persona: fmt.Sprintf(`You are the Head of %s. You set objectives for your department, review manager reports, and ensure your team delivers quality work aligned with division goals.`, title),
	}
	head.ctx, head.cancel = context.WithCancel(org.ctx)
	head.provider = provider
	
	// Add to division
	if div, ok := org.Divisions[divisionID]; ok {
		div.mu.Lock()
		div.Heads = append(div.Heads, head)
		div.mu.Unlock()
	}
	
	return head
}

// CreateManager creates a new manager
func (org *Organization) CreateManager(deptHeadID uuid.UUID, specialty EmployeeSkill) *Manager {
	org.mu.Lock()
	defer org.mu.Unlock()
	
	provider, _ := org.providers.GetProvider(org.config.DefaultProvider)
	
	manager := &Manager{
		ID:         uuid.New(),
		Name:       fmt.Sprintf("%s Manager", specialty),
		Specialty:  specialty,
		Employees:  make([]*Employee, 0),
		MaxReports: 10,
		Persona: fmt.Sprintf(`You are a %s Manager. You assign work to employees, review their output for quality, and provide constructive feedback. You ensure work meets department standards before presenting to leadership.

When reviewing work, respond in JSON:
{
  "rating": "excellent|good|acceptable|needs_work|rejected",
  "approved": true|false,
  "feedback": "specific feedback",
  "revisions": ["revision1", "revision2"]
}`, specialty),
		reviewQueue: make(chan *WorkResult, 100),
	}
	manager.ctx, manager.cancel = context.WithCancel(org.ctx)
	manager.provider = provider
	
	org.AllManagers[manager.ID] = manager
	
	// Start manager goroutine
	org.wg.Add(1)
	go org.runManager(manager)
	
	return manager
}

// CreateEmployee creates a new employee worker
func (org *Organization) CreateEmployee(managerID uuid.UUID, skill EmployeeSkill) *Employee {
	org.mu.Lock()
	defer org.mu.Unlock()
	
	provider, _ := org.providers.GetProvider(org.config.DefaultProvider)
	
	employee := &Employee{
		ID:        uuid.New(),
		Name:      fmt.Sprintf("%s Worker %d", skill, len(org.AllEmployees)+1),
		Skill:     skill,
		ManagerID: managerID,
		Status:    EmployeeIdle,
		Persona:   org.getEmployeePersona(skill),
		workQueue: make(chan *WorkItem, 10),
		resultChan: make(chan *WorkResult, 10),
	}
	employee.ctx, employee.cancel = context.WithCancel(org.ctx)
	employee.provider = provider
	
	org.AllEmployees[employee.ID] = employee
	
	// Add to manager
	if manager, ok := org.AllManagers[managerID]; ok {
		manager.mu.Lock()
		manager.Employees = append(manager.Employees, employee)
		employee.DeptID = manager.ID
		manager.mu.Unlock()
	}
	
	// Start employee goroutine
	org.wg.Add(1)
	go org.runEmployee(employee)
	
	log.WithFields(log.Fields{
		"employee_id": employee.ID,
		"skill":       skill,
		"manager_id":  managerID,
	}).Debug("Employee created")
	
	return employee
}

// getEmployeePersona returns a focused persona for an employee skill
func (org *Organization) getEmployeePersona(skill EmployeeSkill) string {
	personas := map[EmployeeSkill]string{
		SkillWriting: `You are a professional writer. You focus exclusively on writing clear, engaging, and well-structured content. You follow style guides and write for the target audience. Output only the written content.`,
		
		SkillCoding: `You are a software developer. You write clean, efficient, well-documented code. You follow best practices and coding standards. Output only the code with minimal comments.`,
		
		SkillDesign: `You are a designer. You create visual concepts and design specifications. Describe designs clearly with colors, layouts, and styling. Output design specifications or image generation prompts.`,
		
		SkillResearch: `You are a researcher. You gather, analyze, and synthesize information from various sources. Provide factual, well-sourced findings. Output structured research findings.`,
		
		SkillAnalysis: `You are a data analyst. You analyze data, identify patterns, and draw insights. Provide clear, actionable analysis with supporting evidence. Output analysis reports.`,
		
		SkillMarketing: `You are a marketing specialist. You create marketing strategies, campaigns, and messaging. Focus on target audience and value proposition. Output marketing plans and copy.`,
		
		SkillSales: `You are a sales specialist. You craft persuasive pitches, handle objections, and close deals. Focus on customer needs and value. Output sales materials and scripts.`,
		
		SkillSupport: `You are a customer support specialist. You help customers solve problems with empathy and efficiency. Provide clear, helpful responses. Output support responses.`,
		
		SkillQA: `You are a QA specialist. You review work for errors, inconsistencies, and quality issues. Be thorough and constructive. Output detailed QA reports.`,
		
		SkillProjectMgmt: `You are a project manager. You plan, coordinate, and track project progress. Focus on timelines, resources, and deliverables. Output project plans and status reports.`,
		
		SkillDataEntry: `You are a data entry specialist. You accurately input, organize, and format data. Focus on accuracy and consistency. Output structured data.`,
		
		SkillContentReview: `You are a content reviewer. You review content for accuracy, tone, and quality. Provide constructive feedback. Output review notes.`,
	}
	
	if persona, ok := personas[skill]; ok {
		return persona
	}
	return "You are a skilled worker. Complete assigned tasks efficiently and accurately."
}

// runEmployee is the employee worker goroutine
func (org *Organization) runEmployee(emp *Employee) {
	defer org.wg.Done()
	
	// Panic recovery to prevent deadlocks from held locks
	defer func() {
		if r := recover(); r != nil {
			log.WithFields(log.Fields{
				"employee_id": emp.ID,
				"panic":       r,
			}).Error("Employee goroutine panicked, recovering")
			// Mark employee as terminated to prevent further work assignment
			emp.mu.Lock()
			emp.Status = EmployeeTerminated
			emp.mu.Unlock()
		}
	}()
	
	log.WithField("employee_id", emp.ID).Debug("Employee started")
	
	for {
		select {
		case <-emp.ctx.Done():
			func() {
				emp.mu.Lock()
				defer emp.mu.Unlock()
				emp.Status = EmployeeTerminated
			}()
			log.WithField("employee_id", emp.ID).Debug("Employee terminated")
			return
			
		case work := <-emp.workQueue:
			log.WithFields(log.Fields{
				"employee_id":   emp.ID,
				"employee_name": emp.Name,
				"work_title":    work.Title,
			}).Info("Employee received work, starting execution")
			
			func() {
				emp.mu.Lock()
				defer emp.mu.Unlock()
				emp.Status = EmployeeWorking
				emp.CurrentWork = work
			}()
			
			// Broadcast work_started event
			if org.wsHub != nil {
				org.wsHub.Broadcast(WebSocketMessage{
					Type: "work_started",
					Payload: map[string]interface{}{
						"employee_id":   emp.ID.String(),
						"employee_name": emp.Name,
						"skill":         string(emp.Skill),
						"work_title":    work.Title,
						"work_id":       work.ID.String(),
					},
				})
			}
			
			result := org.executeWork(emp, work)
			
			hasError := result.Metadata["error"] != nil
			log.WithFields(log.Fields{
				"employee_id":   emp.ID,
				"employee_name": emp.Name,
				"work_title":    work.Title,
				"has_error":     hasError,
			}).Info("Employee completed work")
			
			var workCount int64
			func() {
				emp.mu.Lock()
				defer emp.mu.Unlock()
				emp.Status = EmployeeIdle
				emp.CurrentWork = nil
				workCount = atomic.AddInt64(&emp.workCount, 1)
			}()
			
			// Create deliverable from completed work
			completedAt := time.Now()
			status := DeliverableCompleted
			if hasError {
				status = DeliverableRejected
			}
			deliverable := &Deliverable{
				ID:           result.ID,
				Title:        work.Title,
				Type:         string(emp.Skill),
				Description:  work.Description,
				Output:       result.Output,
				Status:       status,
				EmployeeID:   emp.ID,
				EmployeeName: emp.Name,
				Skill:        string(emp.Skill),
				CreatedAt:    work.CreatedAt,
				CompletedAt:  &completedAt,
				Duration:     result.Duration,
				Metadata:     result.Metadata,
			}
			
			org.deliverablesMu.Lock()
			org.Deliverables[deliverable.ID] = deliverable
			org.deliverablesMu.Unlock()
			
			// If this work is for a pipeline, notify the pipeline manager (AFTER releasing org lock to avoid deadlock)
			if pipelineIDStr, ok := work.Inputs["pipeline_id"].(string); ok {
				if field, ok := work.Inputs["field"].(string); ok {
					if pipelineID, err := uuid.Parse(pipelineIDStr); err == nil && org.pipeline != nil {
						// Call this in a goroutine to prevent blocking and avoid any lock ordering issues
						log.WithFields(log.Fields{
							"work_id":     work.ID,
							"pipeline_id": pipelineID,
							"field":       field,
						}).Info("Notifying pipeline manager of work completion")
						go org.pipeline.OnWorkComplete(work.ID, result.Output, pipelineID, field)
					} else {
						log.WithFields(log.Fields{
							"work_id":          work.ID,
							"pipeline_id_str":  pipelineIDStr,
							"has_pipeline_mgr": org.pipeline != nil,
							"parse_error":      err != nil,
						}).Debug("Skipping pipeline notification")
					}
				} else {
					log.WithFields(log.Fields{
						"work_id":         work.ID,
						"has_field":       field != "",
						"inputs_keys":     getInputKeys(work.Inputs),
					}).Debug("Work inputs missing field")
				}
			} else {
				log.WithFields(log.Fields{
					"work_id":      work.ID,
					"inputs_keys":  getInputKeys(work.Inputs),
				}).Debug("Work not for pipeline")
			}
			
			// Broadcast work_complete event
			if org.wsHub != nil {
				org.wsHub.Broadcast(WebSocketMessage{
					Type: "work_complete",
					Payload: map[string]interface{}{
						"employee_id":    emp.ID.String(),
						"employee_name":  emp.Name,
						"skill":          string(emp.Skill),
						"work_title":     work.Title,
						"work_id":        work.ID.String(),
						"deliverable_id": deliverable.ID.String(),
						"has_error":      hasError,
						"work_count":     workCount,
						"duration_ms":    result.Duration.Milliseconds(),
					},
				})
			}
			
			// Send result to manager for review
			if manager, ok := org.AllManagers[emp.ManagerID]; ok {
				select {
				case manager.reviewQueue <- result:
				default:
					log.Warn("Manager review queue full")
				}
			}
		}
	}
}

// executeWork performs the actual work
func (org *Organization) executeWork(emp *Employee, work *WorkItem) *WorkResult {
	start := time.Now()
	
	// Acquire semaphore to limit concurrent LLM requests
	select {
	case org.llmSemaphore <- struct{}{}:
		defer func() { <-org.llmSemaphore }()
	case <-emp.ctx.Done():
		return &WorkResult{
			ID:          uuid.New(),
			WorkItemID:  work.ID,
			EmployeeID:  emp.ID,
			CompletedAt: time.Now(),
			Duration:    time.Since(start),
			Output:      "Error: context cancelled while waiting for LLM slot",
			Metadata:    map[string]interface{}{"error": true},
		}
	}
	
	prompt := fmt.Sprintf(`TASK: %s

DESCRIPTION: %s

OBJECTIVES:
%s

Complete this task according to your role and skill. Provide only the output, no explanations.`,
		work.Title,
		work.Description,
		formatObjectives(work.Objectives),
	)
	
	timeout := time.Duration(org.config.LLMTimeoutMinutes) * time.Minute
	ctx, cancel := context.WithTimeout(emp.ctx, timeout)
	defer cancel()
	
	// Check if provider is available
	if emp.provider == nil {
		return &WorkResult{
			ID:          uuid.New(),
			WorkItemID:  work.ID,
			EmployeeID:  emp.ID,
			CompletedAt: time.Now(),
			Duration:    time.Since(start),
			Output:      "Error: No LLM provider available",
			Metadata:    map[string]interface{}{"error": true},
		}
	}
	
	llmStart := time.Now()
	log.WithFields(log.Fields{
		"employee_id":   emp.ID,
		"employee_name": emp.Name,
		"work_id":       work.ID,
		"operation":     "execute_work",
	}).Info("Starting LLM request...")
	
	resp, err := emp.provider.Chat(ctx, LLMRequest{
		Messages: []LLMMessage{
			{Role: "system", Content: emp.Persona},
			{Role: "user", Content: prompt},
		},
		MaxTokens:   2000,
		Temperature: 0.7,
	})
	
	log.WithFields(log.Fields{
		"employee_id": emp.ID,
		"work_id":     work.ID,
		"duration_ms": time.Since(llmStart).Milliseconds(),
		"has_error":   err != nil,
	}).Info("LLM request completed")
	
	result := &WorkResult{
		ID:          uuid.New(),
		WorkItemID:  work.ID,
		EmployeeID:  emp.ID,
		CompletedAt: time.Now(),
		Duration:    time.Since(start),
		Metadata:    make(map[string]interface{}),
	}
	
	if err != nil {
		result.Output = fmt.Sprintf("Error: %v", err)
		result.Metadata["error"] = true
		log.WithFields(log.Fields{
			"employee_id":   emp.ID,
			"employee_name": emp.Name,
			"work_title":    work.Title,
			"error":         err.Error(),
		}).Warn("Employee work failed")
	} else {
		result.Output = resp.Content
		result.TokensUsed = resp.InputTokens + resp.OutputTokens
	}
	
	log.WithFields(log.Fields{
		"employee_id": emp.ID,
		"work_id":     work.ID,
		"duration":    result.Duration,
	}).Debug("Work completed")
	
	return result
}

// runManager is the manager goroutine that reviews work
func (org *Organization) runManager(mgr *Manager) {
	defer org.wg.Done()
	
	log.WithField("manager_id", mgr.ID).Debug("Manager started")
	
	for {
		select {
		case <-mgr.ctx.Done():
			log.WithField("manager_id", mgr.ID).Debug("Manager terminated")
			return
			
		case result := <-mgr.reviewQueue:
			review := org.reviewWork(mgr, result)
			
			if !review.Approved {
				// Reassign work with revisions
				org.handleRejectedWork(mgr, result, review)
			} else {
				log.WithFields(log.Fields{
					"result_id": result.ID,
					"rating":    review.Rating,
				}).Debug("Work approved")
			}
		}
	}
}

// reviewWork has manager review employee work
func (org *Organization) reviewWork(mgr *Manager, result *WorkResult) *QualityReview {
	prompt := fmt.Sprintf(`Review the following work output:

WORK OUTPUT:
%s

Rate the quality and provide feedback. Respond in JSON format.`,
		result.Output,
	)
	
	timeout := time.Duration(org.config.LLMTimeoutMinutes) * time.Minute
	ctx, cancel := context.WithTimeout(mgr.ctx, timeout)
	defer cancel()
	
	llmStart := time.Now()
	log.WithFields(log.Fields{
		"manager_id":   mgr.ID,
		"manager_name": mgr.Name,
		"result_id":    result.ID,
		"operation":    "review_work",
	}).Info("Starting LLM request...")
	
	resp, err := mgr.provider.Chat(ctx, LLMRequest{
		Messages: []LLMMessage{
			{Role: "system", Content: mgr.Persona},
			{Role: "user", Content: prompt},
		},
		MaxTokens:   500,
		Temperature: 0.5,
	})
	
	log.WithFields(log.Fields{
		"manager_id":  mgr.ID,
		"result_id":   result.ID,
		"duration_ms": time.Since(llmStart).Milliseconds(),
		"has_error":   err != nil,
	}).Info("LLM request completed")
	
	review := &QualityReview{
		ID:         uuid.New(),
		ResultID:   result.ID,
		ReviewerID: mgr.ID,
		ReviewedAt: time.Now(),
	}
	
	if err != nil {
		// Default to acceptable if review fails
		review.Rating = QualityAcceptable
		review.Approved = true
		review.Feedback = "Auto-approved due to review error"
		return review
	}
	
	// Parse JSON response
	var reviewData struct {
		Rating    string   `json:"rating"`
		Approved  bool     `json:"approved"`
		Feedback  string   `json:"feedback"`
		Revisions []string `json:"revisions"`
	}
	
	if err := json.Unmarshal([]byte(resp.Content), &reviewData); err != nil {
		review.Rating = QualityAcceptable
		review.Approved = true
		review.Feedback = resp.Content
		return review
	}
	
	review.Rating = QualityRating(reviewData.Rating)
	review.Approved = reviewData.Approved
	review.Feedback = reviewData.Feedback
	review.Revisions = reviewData.Revisions
	
	return review
}

// handleRejectedWork reassigns work with revisions
func (org *Organization) handleRejectedWork(mgr *Manager, result *WorkResult, review *QualityReview) {
	// Find an idle employee
	mgr.mu.RLock()
	var idleEmployee *Employee
	for _, emp := range mgr.Employees {
		emp.mu.RLock()
		if emp.Status == EmployeeIdle {
			idleEmployee = emp
			emp.mu.RUnlock()
			break
		}
		emp.mu.RUnlock()
	}
	mgr.mu.RUnlock()
	
	if idleEmployee == nil {
		log.Warn("No idle employees to reassign work")
		return
	}
	
	// Create revision work item
	revisionWork := &WorkItem{
		ID:          uuid.New(),
		Type:        "revision",
		Title:       "Revision Required",
		Description: fmt.Sprintf("Previous work needs revision:\n\nFEEDBACK: %s\n\nREVISIONS NEEDED:\n%s\n\nORIGINAL OUTPUT:\n%s",
			review.Feedback,
			formatRevisions(review.Revisions),
			result.Output,
		),
		Objectives:  review.Revisions,
		Priority:    2,
		AssignedTo:  idleEmployee.ID,
		AssignedBy:  mgr.ID,
		CreatedAt:   time.Now(),
	}
	
	select {
	case idleEmployee.workQueue <- revisionWork:
		log.WithField("employee_id", idleEmployee.ID).Debug("Revision assigned")
	default:
		log.Warn("Employee work queue full")
	}
}

// AssignWork assigns work to an available employee
func (org *Organization) AssignWork(skill EmployeeSkill, work *WorkItem) error {
	// First pass: try to find an idle employee (read lock only)
	org.mu.RLock()
	for _, emp := range org.AllEmployees {
		emp.mu.RLock()
		if emp.Skill == skill && emp.Status == EmployeeIdle {
			emp.mu.RUnlock()
			
			work.AssignedTo = emp.ID
			
			select {
			case emp.workQueue <- work:
				org.mu.RUnlock()
				log.WithFields(log.Fields{
					"employee_id": emp.ID,
					"work_id":     work.ID,
				}).Debug("Work assigned")
				return nil
			default:
				// Queue full, try next employee
			}
		} else {
			emp.mu.RUnlock()
		}
	}
	org.mu.RUnlock()
	
	// No idle employees found - check if we should scale up
	// Note: scaleUp calls CreateEmployee which needs write lock,
	// so we must NOT hold any read lock here to avoid deadlock
	if org.shouldScaleUp(skill) {
		emp := org.scaleUp(skill)
		if emp != nil {
			work.AssignedTo = emp.ID
			select {
			case emp.workQueue <- work:
				return nil
			default:
			}
		}
	}
	
	return fmt.Errorf("no available employees for skill: %s", skill)
}

// shouldScaleUp checks if we need more employees
func (org *Organization) shouldScaleUp(skill EmployeeSkill) bool {
	org.mu.RLock()
	defer org.mu.RUnlock()
	
	total := 0
	busy := 0
	
	for _, emp := range org.AllEmployees {
		if emp.Skill == skill {
			total++
			emp.mu.RLock()
			if emp.Status == EmployeeWorking {
				busy++
			}
			emp.mu.RUnlock()
		}
	}
	
	if total >= org.maxPoolSize {
		return false
	}
	
	if total == 0 {
		return true
	}
	
	utilization := float64(busy) / float64(total)
	return utilization >= org.scaleThreshold
}

// scaleUp creates a new employee
func (org *Organization) scaleUp(skill EmployeeSkill) *Employee {
	// Find a manager for this skill
	for _, mgr := range org.AllManagers {
		if mgr.Specialty == skill {
			return org.CreateEmployee(mgr.ID, skill)
		}
	}
	return nil
}

// GetStats returns organization statistics
// Uses snapshot approach to avoid lock contention issues
func (org *Organization) GetStats() map[string]interface{} {
	// Copy employee list while holding lock
	org.mu.RLock()
	stats := map[string]interface{}{
		"divisions":       len(org.Divisions),
		"managers":        len(org.AllManagers),
		"total_employees": len(org.AllEmployees),
	}
	
	// Pre-compute status/skill counts while holding org lock
	// by reading employee fields that don't require emp.mu
	// This avoids the nested lock acquisition that can cause deadlocks
	statusCounts := make(map[EmployeeStatus]int)
	skillCounts := make(map[EmployeeSkill]int)
	
	for _, emp := range org.AllEmployees {
		// Use TryRLock to avoid blocking - skip employees with held locks
		if emp.mu.TryRLock() {
			statusCounts[emp.Status]++
			skillCounts[emp.Skill]++
			emp.mu.RUnlock()
		} else {
			// Employee lock is held, count as "unknown" status
			// This prevents indefinite blocking
			statusCounts[EmployeeWorking]++ // Assume working if lock held
			skillCounts[emp.Skill]++        // Skill doesn't change, safe to read
		}
	}
	org.mu.RUnlock()
	
	stats["by_status"] = statusCounts
	stats["by_skill"] = skillCounts
	
	return stats
}

// Stop gracefully stops all workers
func (org *Organization) Stop() {
	org.cancel()
	org.wg.Wait()
	log.Info("Organization stopped")
}

// Helper functions

func formatObjectives(objectives []string) string {
	result := ""
	for i, obj := range objectives {
		result += fmt.Sprintf("%d. %s\n", i+1, obj)
	}
	return result
}

func formatRevisions(revisions []string) string {
	result := ""
	for i, rev := range revisions {
		result += fmt.Sprintf("- %d. %s\n", i+1, rev)
	}
	return result
}

func getInputKeys(inputs map[string]interface{}) []string {
	keys := make([]string, 0, len(inputs))
	for k := range inputs {
		keys = append(keys, k)
	}
	return keys
}
