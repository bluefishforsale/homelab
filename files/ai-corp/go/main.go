package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

const (
	defaultConfigPath = "/app/config.ini"
	version           = "0.1.0"
)

// App holds all application dependencies
type App struct {
	config       *Config
	db           *Database
	redis        *RedisClient
	storage      *StorageManager
	providers    *ProviderManager
	orchestrator *Orchestrator
	board        *Board
	scheduler    *Scheduler
	org          *Organization
	wsHub        *WSHub
	cache        *SimpleCache
	startTime    time.Time
}

// NewApp creates a new application instance
func NewApp(config *Config) (*App, error) {
	appStart := time.Now()
	log.Info("Initializing application components...")
	
	app := &App{
		config:    config,
		wsHub:     NewWSHub(),
		cache:     NewSimpleCache(),
		startTime: time.Now(),
	}

	// Initialize database
	phaseStart := time.Now()
	log.Info("Connecting to PostgreSQL...")
	db, err := NewDatabase(config)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}
	app.db = db
	log.WithField("duration_ms", time.Since(phaseStart).Milliseconds()).Info("Database connection established")

	// Initialize Redis
	phaseStart = time.Now()
	log.Info("Connecting to Redis...")
	redis, err := NewRedisClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Redis: %w", err)
	}
	app.redis = redis
	log.WithField("duration_ms", time.Since(phaseStart).Milliseconds()).Info("Redis connection established")

	// Initialize storage manager
	phaseStart = time.Now()
	log.Info("Initializing storage manager...")
	storage, err := NewStorageManager(config)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize storage: %w", err)
	}
	app.storage = storage
	log.WithField("duration_ms", time.Since(phaseStart).Milliseconds()).Info("Storage initialized")

	// Initialize provider manager
	phaseStart = time.Now()
	log.Info("Initializing LLM providers...")
	app.providers = NewProviderManager(config)
	log.WithField("duration_ms", time.Since(phaseStart).Milliseconds()).Info("Providers initialized")

	// Initialize orchestrator
	phaseStart = time.Now()
	log.Info("Initializing orchestrator...")
	app.orchestrator = NewOrchestrator(config, db, redis, app.providers, storage)
	log.WithField("duration_ms", time.Since(phaseStart).Milliseconds()).Info("Orchestrator initialized")

	// Initialize board of directors (12 members)
	phaseStart = time.Now()
	log.Info("Initializing board of directors...")
	app.board = NewBoard(config, app.providers, db)
	log.WithField("duration_ms", time.Since(phaseStart).Milliseconds()).Info("Board initialized")

	// Initialize scheduler
	phaseStart = time.Now()
	log.Info("Initializing scheduler...")
	app.scheduler = NewScheduler(db, redis)
	app.scheduler.SetBoard(app.board)
	app.scheduler.SetOrchestrator(app.orchestrator)
	log.WithField("duration_ms", time.Since(phaseStart).Milliseconds()).Info("Scheduler initialized")

	// Initialize organization (employees as goroutines)
	phaseStart = time.Now()
	log.Info("Initializing organization structure...")
	app.org = NewOrganization(config, app.providers, storage, db)
	app.org.SetWSHub(app.wsHub) // Connect WebSocket hub for live updates
	app.scheduler.SetOrganization(app.org)
	app.board.SetOrganization(app.org) // Connect board to org for sprint context
	log.WithField("duration_ms", time.Since(phaseStart).Milliseconds()).Info("Organization initialized")

	log.WithField("total_duration_ms", time.Since(appStart).Milliseconds()).Info("Application initialization complete")
	return app, nil
}

// Close cleans up resources
func (app *App) Close() {
	if app.org != nil {
		app.org.Stop()
	}
	if app.scheduler != nil {
		app.scheduler.Stop()
	}
	if app.orchestrator != nil {
		app.orchestrator.Stop()
	}
	// Persist pending jobs for recovery
	if app.redis != nil {
		if err := app.redis.PersistState(); err != nil {
			log.Warnf("Failed to persist Redis state: %v", err)
		}
		app.redis.Close()
	}
	if app.db != nil {
		app.db.Close()
	}
}

func main() {
	// Configure logging
	log.SetFormatter(&log.JSONFormatter{})
	log.SetLevel(log.InfoLevel)

	log.WithField("version", version).Info("Starting AI Corporation")

	// Initialize metrics
	InitMetrics()

	// Load configuration
	configPath := getEnv("CONFIG_PATH", defaultConfigPath)
	config, err := LoadConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Set log level from config
	if level, err := log.ParseLevel(config.LogLevel); err == nil {
		log.SetLevel(level)
	}

	// Create application
	app, err := NewApp(config)
	if err != nil {
		log.Fatalf("Failed to initialize application: %v", err)
	}
	defer app.Close()

	// Start orchestrator
	phaseStart := time.Now()
	log.Info("Starting orchestrator...")
	if err := app.orchestrator.Start(); err != nil {
		log.Fatalf("Failed to start orchestrator: %v", err)
	}
	log.WithField("duration_ms", time.Since(phaseStart).Milliseconds()).Info("Orchestrator started")

	// Start scheduler
	phaseStart = time.Now()
	log.Info("Starting scheduler...")
	if err := app.scheduler.Start(); err != nil {
		log.Fatalf("Failed to start scheduler: %v", err)
	}
	log.WithField("duration_ms", time.Since(phaseStart).Milliseconds()).Info("Scheduler started")

	log.Infof("Board initialized with %d members", len(app.board.GetAllMembers()))

	// Log organization stats
	orgStats := app.org.GetStats()
	log.WithFields(log.Fields{
		"divisions": orgStats["divisions"],
		"managers":  orgStats["managers"],
		"employees": orgStats["total_employees"],
	}).Info("Organization initialized")

	// Setup HTTP handlers
	handlers := NewHandlers(app)

	router := mux.NewRouter()

	// Apply middleware
	router.Use(CORSMiddleware)
	router.Use(LoggingMiddleware)

	// Health and metrics
	router.HandleFunc("/health", handlers.HealthHandler).Methods("GET")
	router.Handle("/metrics", promhttp.Handler())

	// API v1
	api := router.PathPrefix("/api/v1").Subrouter()

	// Workflows
	api.HandleFunc("/workflows", handlers.ListWorkflowsHandler).Methods("GET")
	api.HandleFunc("/workflows/{id}", handlers.GetWorkflowHandler).Methods("GET")
	api.HandleFunc("/workflows/{id}/run", handlers.StartRunHandler).Methods("POST")

	// Runs
	api.HandleFunc("/runs", handlers.ListRunsHandler).Methods("GET")
	api.HandleFunc("/runs/{id}", handlers.GetRunHandler).Methods("GET")
	api.HandleFunc("/runs/{id}/cancel", handlers.CancelRunHandler).Methods("POST")
	api.HandleFunc("/runs/{id}/artifacts", handlers.GetArtifactsHandler).Methods("GET")
	api.HandleFunc("/runs/{run_id}/artifacts/{artifact_id}/download", handlers.DownloadArtifactHandler).Methods("GET")

	// Storage
	api.HandleFunc("/storage/status", handlers.StorageStatusHandler).Methods("GET")

	// Roles and Providers
	api.HandleFunc("/roles", handlers.ListRolesHandler).Methods("GET")
	api.HandleFunc("/providers", handlers.ListProvidersHandler).Methods("GET")
	api.HandleFunc("/providers/{id}/test", handlers.TestProviderHandler).Methods("POST")

	// Board
	api.HandleFunc("/board/members", handlers.ListBoardMembersHandler).Methods("GET")
	api.HandleFunc("/board/vote", handlers.BoardVoteHandler).Methods("POST")
	api.HandleFunc("/board/meetings", handlers.ListMeetingsHandler).Methods("GET")
	api.HandleFunc("/board/meetings/trigger", handlers.TriggerMeetingHandler).Methods("POST")

	// Scheduler
	api.HandleFunc("/scheduler/tasks", handlers.ListScheduledTasksHandler).Methods("GET")
	api.HandleFunc("/scheduler/tasks/{id}/trigger", handlers.TriggerTaskHandler).Methods("POST")

	// Organization
	api.HandleFunc("/org/stats", handlers.OrgStatsHandler).Methods("GET")
	api.HandleFunc("/org/status", handlers.CompanyStatusHandler).Methods("GET")
	api.HandleFunc("/org/pause", handlers.PauseCompanyHandler).Methods("POST")
	api.HandleFunc("/org/resume", handlers.ResumeCompanyHandler).Methods("POST")
	api.HandleFunc("/org/divisions", handlers.ListDivisionsHandler).Methods("GET")
	api.HandleFunc("/org/divisions", handlers.CreateDivisionHandler).Methods("POST")
	api.HandleFunc("/org/employees", handlers.ListEmployeesHandler).Methods("GET")
	api.HandleFunc("/org/employees/{id}", handlers.GetEmployeeHandler).Methods("GET")
	api.HandleFunc("/org/deliverables", handlers.ListDeliverablesHandler).Methods("GET")
	api.HandleFunc("/org/products", handlers.ListProductsHandler).Methods("GET")
	api.HandleFunc("/org/pipelines", handlers.ListPipelinesHandler).Methods("GET")
	api.HandleFunc("/org/pipelines/{id}/download", handlers.DownloadPipelineHandler).Methods("GET")
	api.HandleFunc("/org/employees/{id}/detail", handlers.GetEmployeeDetailHandler).Methods("GET")
	api.HandleFunc("/org/person/{id}", handlers.GetPersonDetailHandler).Methods("GET")
	api.HandleFunc("/org/managers", handlers.ListManagersHandler).Methods("GET")
	api.HandleFunc("/org/work", handlers.AssignWorkHandler).Methods("POST")
	api.HandleFunc("/org/people", handlers.ListAllPeopleHandler).Methods("GET")

	// Biography management
	api.HandleFunc("/biographies", handlers.ListBiographiesHandler).Methods("GET")
	api.HandleFunc("/biographies", handlers.UpdateBiographyHandler).Methods("POST")
	api.HandleFunc("/biographies/{person_id}", handlers.GetBiographyHandler).Methods("GET")
	api.HandleFunc("/biographies/{person_id}", handlers.DeleteBiographyHandler).Methods("DELETE")
	api.HandleFunc("/org/people", handlers.ListAllPeopleHandler).Methods("GET")

	// Company seed/bootstrap
	api.HandleFunc("/org/sectors", handlers.GetSectorsHandler).Methods("GET")
	api.HandleFunc("/org/seed", handlers.GetSeedHandler).Methods("GET")
	api.HandleFunc("/org/seed", handlers.SetSeedHandler).Methods("POST")

	// Restructuring
	api.HandleFunc("/org/restructuring", handlers.GetRestructuringHistoryHandler).Methods("GET")
	api.HandleFunc("/org/restructuring/pending", handlers.GetPendingRestructuringHandler).Methods("GET")
	api.HandleFunc("/org/restructuring", handlers.ProposeRestructuringHandler).Methods("POST")
	api.HandleFunc("/org/restructuring/{id}/vote", handlers.VoteRestructuringHandler).Methods("POST")
	api.HandleFunc("/org/restructuring/{id}/execute", handlers.ExecuteRestructuringHandler).Methods("POST")
	api.HandleFunc("/org/health", handlers.AnalyzeOrgHealthHandler).Methods("GET")

	// Admin
	api.HandleFunc("/admin/status", handlers.GetSystemStatusHandler).Methods("GET")
	api.HandleFunc("/admin/reset", handlers.ResetOrganizationHandler).Methods("POST")

	// Meetings
	api.HandleFunc("/meetings", handlers.ListMeetingsHandler).Methods("GET")
	api.HandleFunc("/meetings/{id}", handlers.GetMeetingHandler).Methods("GET")

	// Webhooks
	router.HandleFunc("/webhook/midjourney", handlers.MidjourneyWebhookHandler).Methods("POST")

	// WebSocket for live updates
	api.HandleFunc("/ws", handlers.WSHandler)

	// Start WebSocket hub
	go app.wsHub.Run()
	
	// Start periodic org status broadcasts
	app.StartPeriodicBroadcast()

	// Serve static files for web UI (if present)
	staticDir := "/app/web/dist"
	if _, err := os.Stat(staticDir); err == nil {
		spa := spaHandler{staticPath: staticDir, indexPath: "index.html"}
		router.PathPrefix("/").Handler(spa)
		log.Infof("Serving web UI from %s", staticDir)
	}

	// Start HTTP server
	srv := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", config.Host, config.Port),
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 120 * time.Second, // Long timeout for LLM requests
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Infof("HTTP server listening on %s:%d", config.Host, config.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server failed: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("Shutting down server...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Errorf("HTTP server forced to shutdown: %v", err)
	}

	log.Info("Server exited")
}

// spaHandler serves a Single Page Application
type spaHandler struct {
	staticPath string
	indexPath  string
}

func (h spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := h.staticPath + r.URL.Path

	// Check if file exists
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		// Serve index.html for SPA routing
		http.ServeFile(w, r, h.staticPath+"/"+h.indexPath)
		return
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.FileServer(http.Dir(h.staticPath)).ServeHTTP(w, r)
}

// getEnv returns environment variable or default
func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

// getEnvInt returns environment variable as int or default
func getEnvInt(key string, fallback int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return fallback
}
