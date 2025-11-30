package main

import (
	"context"
	"encoding/json"
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

// Config holds configuration for the ML processor
type Config struct {
	Port              int    `json:"port"`
	PostgresHost      string `json:"postgres_host"`
	PostgresPort      int    `json:"postgres_port"`
	PostgresDB        string `json:"postgres_db"`
	PostgresUser      string `json:"postgres_user"`
	PostgresPassword  string `json:"postgres_password"`
	RedisAddr         string `json:"redis_addr"`
	RedisPassword     string `json:"redis_password"`
	AlertManagerURL   string `json:"alertmanager_url"`
	SMTPHost          string `json:"smtp_host"`
	SMTPPort          int    `json:"smtp_port"`
	SMTPUser          string `json:"smtp_user"`
	SMTPPassword      string `json:"smtp_password"`
	SMTPFrom          string `json:"smtp_from"`
	DigestRecipient   string `json:"digest_recipient"`
	DigestHour        int    `json:"digest_hour"`
	LLMUrl            string `json:"llm_url"`
}

// loadConfig loads configuration from environment variables
func loadConfig() Config {
	return Config{
		Port:              getEnvInt("PORT", 8087),
		PostgresHost:      getEnv("POSTGRES_HOST", "localhost"),
		PostgresPort:      getEnvInt("POSTGRES_PORT", 5433),
		PostgresDB:        getEnv("POSTGRES_DB", "log_anomaly_ml"),
		PostgresUser:      getEnv("POSTGRES_USER", "log_anomaly_ml"),
		PostgresPassword:  getEnv("POSTGRES_PASSWORD", ""),
		RedisAddr:         getEnv("REDIS_ADDR", "redis:6379"),
		RedisPassword:     getEnv("REDIS_PASSWORD", ""),
		AlertManagerURL:   getEnv("ALERTMANAGER_URL", "http://192.168.1.143:9093"),
		SMTPHost:          getEnv("SMTP_HOST", "smtp.gmail.com"),
		SMTPPort:          getEnvInt("SMTP_PORT", 587),
		SMTPUser:          getEnv("SMTP_USER", ""),
		SMTPPassword:      getEnv("SMTP_PASSWORD", ""),
		SMTPFrom:          getEnv("SMTP_FROM", "homelab-alerts@gmail.com"),
		DigestRecipient:   getEnv("DIGEST_RECIPIENT", ""),
		DigestHour:        getEnvInt("DIGEST_HOUR", 6),
		LLMUrl:            getEnv("LLM_URL", "http://192.168.1.143:8080"),
	}
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return fallback
}

// App holds all application dependencies
type App struct {
	config    Config
	db        *Database
	redis     *RedisClient
	alertMgr  *AlertManagerClient
	processor *Processor
	startTime time.Time
}

// NewApp creates a new application instance
func NewApp(config Config) (*App, error) {
	app := &App{
		config:    config,
		startTime: time.Now(),
	}

	// Initialize database
	db, err := NewDatabase(config)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}
	app.db = db

	// Initialize Redis
	redis, err := NewRedisClient(config.RedisAddr, config.RedisPassword)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Redis: %w", err)
	}
	app.redis = redis

	// Initialize AlertManager client
	app.alertMgr = NewAlertManagerClient(config.AlertManagerURL)

	// Initialize processor
	app.processor = NewProcessor(db, redis, app.alertMgr, config)

	return app, nil
}

// Close cleans up resources
func (app *App) Close() {
	if app.db != nil {
		app.db.Close()
	}
	if app.redis != nil {
		app.redis.Close()
	}
}

// Handlers holds HTTP handlers
type Handlers struct {
	app *App
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

	// Check AlertManager
	amStart := time.Now()
	if err := h.app.alertMgr.HealthCheck(); err != nil {
		checks["alertmanager"] = ComponentHealth{Status: "down", Error: err.Error()}
		healthStatus.WithLabelValues("alertmanager").Set(0)
		downCount++
	} else {
		checks["alertmanager"] = ComponentHealth{Status: "up", LatencyMs: time.Since(amStart).Milliseconds()}
		healthStatus.WithLabelValues("alertmanager").Set(1)
	}

	// Get queue sizes
	dlSize, _ := h.app.redis.GetDeadLetterSize()
	deadLetterSize.Set(float64(dlSize))

	// Get active problem count
	activeCount, _ := h.app.db.GetActiveProblemsCount()
	problemsActive.Set(float64(activeCount))

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
		Status:         status,
		Mode:           mode,
		Service:        "log-anomaly-ml-processor",
		UptimeSeconds:  int64(time.Since(h.app.startTime).Seconds()),
		Checks:         checks,
		ProblemsActive: int(activeCount),
		QueueDepth:     int(dlSize),
	}

	w.Header().Set("Content-Type", "application/json")
	if status == "unhealthy" {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	json.NewEncoder(w).Encode(response)
}

// WebhookHandler handles incoming anomalies from Tier 1
func (h *Handlers) WebhookHandler(w http.ResponseWriter, r *http.Request) {
	var anomaly Anomaly
	if err := json.NewDecoder(r.Body).Decode(&anomaly); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		webhooksReceived.WithLabelValues("error").Inc()
		return
	}

	webhooksReceived.WithLabelValues("success").Inc()

	// Process the anomaly
	result, err := h.app.processor.ProcessAnomaly(anomaly)
	if err != nil {
		log.Errorf("Failed to process anomaly: %v", err)
		http.Error(w, "Processing failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// ProblemsHandler handles problem listing
func (h *Handlers) ProblemsHandler(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	if status == "" {
		status = "active"
	}

	problems, err := h.app.db.GetProblems(status)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"problems": problems,
		"total":    len(problems),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// ProblemHandler handles single problem retrieval
func (h *Handlers) ProblemHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	problem, err := h.app.db.GetProblem(id)
	if err != nil {
		http.Error(w, "Problem not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(problem)
}

// ResolveProblemHandler handles manual problem resolution
func (h *Handlers) ResolveProblemHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	if err := h.app.db.ResolveProblem(id); err != nil {
		http.Error(w, "Failed to resolve problem", http.StatusInternalServerError)
		return
	}

	problemsResolved.Inc()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":      "resolved",
		"resolved_at": time.Now().Format(time.RFC3339),
	})
}

// SuppressProblemHandler handles problem suppression
func (h *Handlers) SuppressProblemHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	var body struct {
		Reason string `json:"reason"`
	}
	json.NewDecoder(r.Body).Decode(&body)

	if err := h.app.db.SuppressProblem(id, body.Reason); err != nil {
		http.Error(w, "Failed to suppress problem", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "suppressed",
		"reason": body.Reason,
	})
}

// DigestTriggerHandler manually triggers a digest email
func (h *Handlers) DigestTriggerHandler(w http.ResponseWriter, r *http.Request) {
	count, err := h.app.processor.SendDigest()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to send digest: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":            "sent",
		"recipient":         h.app.config.DigestRecipient,
		"problems_included": count,
	})
}

// StatsHandler returns problem statistics
func (h *Handlers) StatsHandler(w http.ResponseWriter, r *http.Request) {
	stats, err := h.app.db.GetStats()
	if err != nil {
		http.Error(w, "Failed to get stats", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// FlushHandler clears all data from PostgreSQL and Redis
func (h *Handlers) FlushHandler(w http.ResponseWriter, r *http.Request) {
	var errors []string

	// Flush PostgreSQL
	if err := h.app.db.FlushAll(); err != nil {
		errors = append(errors, fmt.Sprintf("postgres: %v", err))
	}

	// Flush Redis
	if err := h.app.redis.FlushAll(); err != nil {
		errors = append(errors, fmt.Sprintf("redis: %v", err))
	}

	if len(errors) > 0 {
		http.Error(w, fmt.Sprintf("Flush partially failed: %v", errors), http.StatusInternalServerError)
		return
	}

	// Reset metrics
	problemsActive.Set(0)
	deadLetterSize.Set(0)

	log.Info("All data flushed successfully")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "success",
		"message": "All data flushed (problems, dedup cache, rate limits, queues)",
	})
}

func main() {
	log.SetFormatter(&log.JSONFormatter{})
	log.SetLevel(log.InfoLevel)
	log.Info("Starting Log Anomaly ML Processor")

	// Initialize metrics
	InitMetrics()

	// Load configuration
	config := loadConfig()

	// Create application
	app, err := NewApp(config)
	if err != nil {
		log.Fatalf("Failed to initialize application: %v", err)
	}
	defer app.Close()

	// Replay dead letter queue on startup
	go func() {
		time.Sleep(5 * time.Second) // Wait for service to stabilize
		count := app.processor.ReplayDeadLetter()
		if count > 0 {
			log.Infof("Replayed %d anomalies from dead letter queue", count)
		}
	}()

	// Start background jobs
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go app.processor.StartScheduler(ctx)
	go app.processor.StartProblemResolver(ctx)

	// Setup HTTP handlers
	handlers := &Handlers{app: app}

	router := mux.NewRouter()
	router.HandleFunc("/health", handlers.HealthHandler).Methods("GET")
	router.HandleFunc("/webhook/anomaly", handlers.WebhookHandler).Methods("POST")
	router.HandleFunc("/problems", handlers.ProblemsHandler).Methods("GET")
	router.HandleFunc("/problems/stats", handlers.StatsHandler).Methods("GET")
	router.HandleFunc("/problems/{id}", handlers.ProblemHandler).Methods("GET")
	router.HandleFunc("/problems/{id}/resolve", handlers.ResolveProblemHandler).Methods("POST")
	router.HandleFunc("/problems/{id}/suppress", handlers.SuppressProblemHandler).Methods("POST")
	router.HandleFunc("/digest/trigger", handlers.DigestTriggerHandler).Methods("POST")
	router.HandleFunc("/admin/flush", handlers.FlushHandler).Methods("POST")
	router.Handle("/metrics", promhttp.Handler())

	// Start HTTP server
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", config.Port),
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Infof("HTTP server listening on :%d", config.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server failed: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("Shutting down server...")
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Errorf("HTTP server forced to shutdown: %v", err)
	}

	log.Info("Server exited")
}
