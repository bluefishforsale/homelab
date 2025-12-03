package main

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// Webhook metrics
	webhooksReceived = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "log_ml_webhooks_received_total",
		Help: "Total webhooks received from Tier 1",
	}, []string{"status"})

	// Anomaly processing metrics
	anomaliesProcessed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "log_ml_anomalies_processed_total",
		Help: "Total anomalies processed",
	}, []string{"severity", "host"})

	anomaliesDuplicate = promauto.NewCounter(prometheus.CounterOpts{
		Name: "log_ml_anomalies_duplicate_total",
		Help: "Total duplicate anomalies skipped",
	})

	anomaliesRateLimited = promauto.NewCounter(prometheus.CounterOpts{
		Name: "log_ml_anomalies_rate_limited_total",
		Help: "Total anomalies rate limited",
	})

	// Problem metrics
	problemsCreated = promauto.NewCounter(prometheus.CounterOpts{
		Name: "log_ml_problems_created_total",
		Help: "Total problems created",
	})

	problemsResolved = promauto.NewCounter(prometheus.CounterOpts{
		Name: "log_ml_problems_resolved_total",
		Help: "Total problems resolved",
	})

	problemsActive = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "log_ml_problems_active",
		Help: "Current number of active problems",
	})

	// Alert metrics
	alertsSent = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "log_ml_alerts_sent_total",
		Help: "Total alerts sent",
	}, []string{"severity", "destination", "status"})

	// Digest metrics
	digestsSent = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "log_ml_digests_sent_total",
		Help: "Total digest emails sent",
	}, []string{"status"})

	// Dead letter queue metrics
	deadLetterSize = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "log_ml_dead_letter_size",
		Help: "Current size of dead letter queue",
	})

	deadLetterReplayed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "log_ml_dead_letter_replayed_total",
		Help: "Total items replayed from dead letter queue",
	})

	// Health metrics
	healthStatus = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "log_ml_health_status",
		Help: "Health status of dependencies (1=up, 0=down)",
	}, []string{"component"})

	// Performance metrics
	processingDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "log_ml_processing_duration_seconds",
		Help:    "Time spent processing",
		Buckets: prometheus.DefBuckets,
	}, []string{"operation"})

	// LLM metrics
	llmRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "log_ml_llm_requests_total",
		Help: "Total LLM analysis requests",
	}, []string{"status"})

	llmRequestDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "log_ml_llm_request_duration_seconds",
		Help:    "Time spent on LLM requests",
		Buckets: []float64{1, 5, 10, 30, 60},
	})
)

// InitMetrics initializes metrics
func InitMetrics() {
	// Initialize health status
	healthStatus.WithLabelValues("postgresql").Set(0)
	healthStatus.WithLabelValues("redis").Set(0)
	healthStatus.WithLabelValues("alertmanager").Set(0)
	healthStatus.WithLabelValues("llm").Set(0)
}
