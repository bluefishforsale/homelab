package main

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// Workflow metrics
	workflowsStarted = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aicorp_workflows_started_total",
		Help: "Total workflows started",
	}, []string{"template"})

	workflowsCompleted = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aicorp_workflows_completed_total",
		Help: "Total workflows completed",
	}, []string{"template", "status"})

	workflowDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "aicorp_workflow_duration_seconds",
		Help:    "Workflow execution duration",
		Buckets: []float64{1, 5, 10, 30, 60, 120, 300, 600},
	}, []string{"template"})

	activeWorkflows = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "aicorp_active_workflows",
		Help: "Currently running workflows",
	})

	// Step metrics
	stepsExecuted = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aicorp_steps_executed_total",
		Help: "Total workflow steps executed",
	}, []string{"role", "status"})

	stepDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "aicorp_step_duration_seconds",
		Help:    "Step execution duration",
		Buckets: []float64{0.5, 1, 2, 5, 10, 30, 60},
	}, []string{"role"})

	// LLM metrics
	llmRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aicorp_llm_requests_total",
		Help: "Total LLM API requests",
	}, []string{"provider", "role", "status"})

	llmLatency = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "aicorp_llm_latency_seconds",
		Help:    "LLM request latency",
		Buckets: []float64{0.5, 1, 2, 5, 10, 30, 60},
	}, []string{"provider"})

	llmTokensInput = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aicorp_llm_tokens_input_total",
		Help: "Total input tokens used",
	}, []string{"provider"})

	llmTokensOutput = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aicorp_llm_tokens_output_total",
		Help: "Total output tokens generated",
	}, []string{"provider"})

	// Queue metrics
	queueDepth = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "aicorp_queue_depth",
		Help: "Jobs waiting in queue",
	})

	queueProcessed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aicorp_queue_processed_total",
		Help: "Total jobs processed from queue",
	})

	// Health metrics
	healthStatus = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "aicorp_health_status",
		Help: "Health status of dependencies (1=up, 0=down)",
	}, []string{"component"})

	// API metrics
	httpRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aicorp_http_requests_total",
		Help: "Total HTTP requests",
	}, []string{"method", "path", "status"})

	httpDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "aicorp_http_duration_seconds",
		Help:    "HTTP request duration",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "path"})

	// WebSocket metrics
	wsConnections = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "aicorp_websocket_connections",
		Help: "Current WebSocket connections",
	})

	wsMessages = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aicorp_websocket_messages_total",
		Help: "Total WebSocket messages sent",
	}, []string{"type"})
)

// InitMetrics initializes health status metrics
func InitMetrics() {
	healthStatus.WithLabelValues("postgresql").Set(0)
	healthStatus.WithLabelValues("redis").Set(0)
	healthStatus.WithLabelValues("llm").Set(0)
}
