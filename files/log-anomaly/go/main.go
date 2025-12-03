package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

// Config holds configuration for the anomaly detector
type Config struct {
	LokiURL               string   `json:"loki_url"`
	CheckInterval         int      `json:"check_interval"`
	BatchSize             int      `json:"batch_size"`
	WebhookURL            string   `json:"webhook_url"`
	FrequencySigma        float64  `json:"frequency_sigma"`
	RateChangeThreshold   float64  `json:"rate_change_threshold"`
	EntropyThreshold      float64  `json:"entropy_threshold"`
	LevenshteinThreshold  float64  `json:"levenshtein_threshold"`
	PatternsDir           string   `json:"patterns_dir"`
	DBPath                string   `json:"db_path"`
	// Signal quality settings
	AlertCooldownMinutes  int      `json:"alert_cooldown_minutes"`
	MinSeverity           string   `json:"min_severity"`
	MinPatternScore       float64  `json:"min_pattern_score"`
	MinRepetitionCount    int      `json:"min_repetition_count"`
	SuppressedServices    []string `json:"suppressed_services"`
	// Dashboard link
	DashboardURL          string   `json:"dashboard_url"`
}

// loadConfig loads configuration from environment variables
func loadConfig() Config {
	config := Config{
		LokiURL:               getEnv("LOKI_URL", "http://192.168.1.143:3100"),
		CheckInterval:         getEnvInt("CHECK_INTERVAL", 30),
		BatchSize:             getEnvInt("BATCH_SIZE", 1000),
		WebhookURL:            getEnv("WEBHOOK_URL", "http://192.168.1.143:5678/webhook/log-anomaly"),
		FrequencySigma:        getEnvFloat("FREQUENCY_SIGMA", 4.0),       // Raised from 3.0
		RateChangeThreshold:   getEnvFloat("RATE_CHANGE_THRESHOLD", 10.0), // Raised from 5.0
		EntropyThreshold:      getEnvFloat("ENTROPY_THRESHOLD", 6.5),     // Raised from 4.5
		LevenshteinThreshold:  getEnvFloat("LEVENSHTEIN_THRESHOLD", 0.90), // Raised from 0.7
		PatternsDir:           getEnv("PATTERNS_DIR", "/app/patterns"),
		DBPath:                getEnv("DB_PATH", "/app/data/anomaly_stats.db"),
		// Signal quality defaults
		AlertCooldownMinutes:  getEnvInt("ALERT_COOLDOWN_MINUTES", 60),   // 1 hour between same alerts
		MinSeverity:           getEnv("MIN_SEVERITY", "high"),            // Only high/critical alerts
		MinPatternScore:       getEnvFloat("MIN_PATTERN_SCORE", 3.0),     // Raised from 2.0
		MinRepetitionCount:    getEnvInt("MIN_REPETITION_COUNT", 20),     // Raised from 3
		SuppressedServices:    getEnvList("SUPPRESSED_SERVICES", "blackbox-exporter,promtail,node-exporter"),
		// Dashboard link
		DashboardURL:          getEnv("DASHBOARD_URL", "http://ocean.home/d/log-anomaly-detector/log-anomaly-detector-tier-1"),
	}
	return config
}

// getEnv gets environment variable with fallback
func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

// getEnvInt gets environment variable as int with fallback
func getEnvInt(key string, fallback int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return fallback
}

// getEnvFloat gets environment variable as float64 with fallback
func getEnvFloat(key string, fallback float64) float64 {
	if value := os.Getenv(key); value != "" {
		if f, err := strconv.ParseFloat(value, 64); err == nil {
			return f
		}
	}
	return fallback
}

// getEnvList gets environment variable as comma-separated list with fallback
func getEnvList(key, fallback string) []string {
	value := os.Getenv(key)
	if value == "" {
		value = fallback
	}
	if value == "" {
		return []string{}
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// HTTP handlers
type HTTPHandlers struct {
	detector            *AnomalyDetector
	statisticalAnalyzer *RedisStatisticalAnalyzer
	startTime           time.Time
	config              Config
}

// ComponentHealth represents health of a single component
type ComponentHealth struct {
	Status    string `json:"status"`
	LatencyMs int64  `json:"latency_ms,omitempty"`
	Error     string `json:"error,omitempty"`
}

// HealthResponse represents the health check response
type HealthResponse struct {
	Status        string                     `json:"status"`
	Mode          string                     `json:"mode"`
	Service       string                     `json:"service"`
	UptimeSeconds int64                      `json:"uptime_seconds"`
	Checks        map[string]ComponentHealth `json:"checks"`
	ColdStart     bool                       `json:"cold_start"`
}

// HealthHandler handles health check requests
func (h *HTTPHandlers) HealthHandler(w http.ResponseWriter, r *http.Request) {
	checks := make(map[string]ComponentHealth)
	downCount := 0

	// Check Redis
	redisStart := time.Now()
	if err := h.statisticalAnalyzer.Ping(); err != nil {
		checks["redis"] = ComponentHealth{Status: "down", Error: err.Error()}
		healthStatus.WithLabelValues("redis").Set(0)
		downCount++
	} else {
		checks["redis"] = ComponentHealth{Status: "up", LatencyMs: time.Since(redisStart).Milliseconds()}
		healthStatus.WithLabelValues("redis").Set(1)
	}

	// Check Loki connectivity
	lokiStart := time.Now()
	lokiResp, err := http.Get(h.config.LokiURL + "/ready")
	if err != nil || (lokiResp != nil && lokiResp.StatusCode != http.StatusOK) {
		errMsg := "connection failed"
		if err != nil {
			errMsg = err.Error()
		} else if lokiResp != nil {
			errMsg = fmt.Sprintf("status %d", lokiResp.StatusCode)
			lokiResp.Body.Close()
		}
		checks["loki"] = ComponentHealth{Status: "down", Error: errMsg}
		healthStatus.WithLabelValues("loki").Set(0)
		downCount++
	} else {
		lokiResp.Body.Close()
		checks["loki"] = ComponentHealth{Status: "up", LatencyMs: time.Since(lokiStart).Milliseconds()}
		healthStatus.WithLabelValues("loki").Set(1)
	}

	// Check for cold start (no baselines)
	isColdStart := false
	if empty, err := h.statisticalAnalyzer.IsEmpty(); err == nil && empty {
		isColdStart = true
		coldStartActive.Set(1)
	} else {
		coldStartActive.Set(0)
	}

	// Check dead letter queue size
	if dlSize, err := h.statisticalAnalyzer.GetDeadLetterSize(); err == nil {
		deadLetterSize.Set(float64(dlSize))
	}

	// Determine overall status and mode
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
		Service:       "log-anomaly-detector",
		UptimeSeconds: int64(time.Since(h.startTime).Seconds()),
		Checks:        checks,
		ColdStart:     isColdStart,
	}

	w.Header().Set("Content-Type", "application/json")
	if status == "unhealthy" {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	json.NewEncoder(w).Encode(response)
}

// StatusHandler handles status requests
func (h *HTTPHandlers) StatusHandler(w http.ResponseWriter, r *http.Request) {
	if h.detector == nil {
		http.Error(w, "Service not ready", http.StatusServiceUnavailable)
		return
	}
	
	response := map[string]interface{}{
		"status":              "running",
		"patterns_loaded":     h.detector.GetPatternsCount(),
		"recent_logs_count":   h.detector.GetRecentLogsCount(),
		"config": map[string]interface{}{
			"check_interval": h.detector.config.CheckInterval,
			"batch_size":     h.detector.config.BatchSize,
			"loki_url":       h.detector.config.LokiURL,
		},
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// PatternsHandler handles patterns requests
func (h *HTTPHandlers) PatternsHandler(w http.ResponseWriter, r *http.Request) {
	if h.detector == nil {
		http.Error(w, "Service not ready", http.StatusServiceUnavailable)
		return
	}
	
	patterns := h.detector.GetPatterns()
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(patterns)
}

// SourcesHandler returns auto-discovered hosts and services from logs
func (h *HTTPHandlers) SourcesHandler(w http.ResponseWriter, r *http.Request) {
	if h.detector == nil {
		http.Error(w, "Service not ready", http.StatusServiceUnavailable)
		return
	}
	
	sources := h.detector.GetDiscoveredSources()
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sources)
}

// FlushHandler clears all Redis data (patterns, baselines, queues)
func (h *HTTPHandlers) FlushHandler(w http.ResponseWriter, r *http.Request) {
	if err := h.statisticalAnalyzer.FlushAll(); err != nil {
		log.Printf("Flush failed: %v", err)
		http.Error(w, fmt.Sprintf("Flush failed: %v", err), http.StatusInternalServerError)
		return
	}
	
	// Reset metrics
	coldStartActive.Set(1)
	deadLetterSize.Set(0)
	
	log.Println("All data flushed successfully")
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "success",
		"message": "All data flushed (patterns, baselines, queues)",
	})
}

func main() {
	startTime := time.Now()
	log.Println("Starting Log Anomaly Detector (Go)")
	
	// Initialize Prometheus metrics
	InitMetrics()
	
	// Load configuration
	config := loadConfig()
	
	// Initialize structured pattern manager  
	patternManager, err := NewStructuredPatternManager(config.PatternsDir)
	if err != nil {
		log.Fatalf("Failed to create structured pattern manager: %v", err)
	}
	
	// Initialize Redis statistical analyzer
	redisAddr := getEnv("REDIS_ADDR", "redis:6379")
	redisPassword := getEnv("REDIS_PASSWORD", "")
	statisticalAnalyzer, err := NewRedisStatisticalAnalyzer(redisAddr, redisPassword)
	if err != nil {
		log.Fatalf("Failed to create Redis statistical analyzer: %v", err)
	}
	
	// Check for cold start (empty baselines)
	if empty, err := statisticalAnalyzer.IsEmpty(); err == nil && empty {
		log.Warn("COLD START: No baselines found in Redis. Statistical alerts will use 10x higher thresholds for 24h.")
		coldStartActive.Set(1)
		coldStartThresholdMultiplier.Set(10.0)
	} else {
		coldStartActive.Set(0)
		coldStartThresholdMultiplier.Set(1.0)
	}
	
	// Initialize anomaly detector
	detector := NewAnomalyDetector(config, patternManager, statisticalAnalyzer)
	
	// Start the detector
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	detector.Start(ctx)
	
	// Setup HTTP handlers
	handlers := &HTTPHandlers{
		detector:            detector,
		statisticalAnalyzer: statisticalAnalyzer,
		startTime:           startTime,
		config:              config,
	}
	
	router := mux.NewRouter()
	router.HandleFunc("/health", handlers.HealthHandler).Methods("GET")
	router.HandleFunc("/status", handlers.StatusHandler).Methods("GET")
	router.HandleFunc("/patterns", handlers.PatternsHandler).Methods("GET")
	router.HandleFunc("/sources", handlers.SourcesHandler).Methods("GET")
	router.HandleFunc("/admin/flush", handlers.FlushHandler).Methods("POST")
	router.Handle("/metrics", promhttp.Handler())
	
	// Start HTTP server
	srv := &http.Server{
		Addr:         ":8085",
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	
	go func() {
		log.Printf("HTTP server listening on :8085")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server failed: %v", err)
		}
	}()
	
	// Wait for interrupt signal to gracefully shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	
	log.Println("Shutting down server...")
	cancel()
	
	// Gracefully shutdown HTTP server
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP server forced to shutdown: %v", err)
	}
	
	log.Println("Server exited")
}
