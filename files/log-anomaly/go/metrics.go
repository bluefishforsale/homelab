package main

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Prometheus metrics for the log anomaly detector
var (
	// Pattern matching metrics
	patternsLoadedTotal = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "log_anomaly_patterns_loaded_total",
		Help: "Total number of loaded patterns by category and type",
	}, []string{"category", "type"})

	patternMatchesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "log_anomaly_pattern_matches_total",
		Help: "Total number of pattern matches",
	}, []string{"pattern_name", "category", "host", "service"})

	// Log processing metrics
	logsProcessedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "log_anomaly_logs_processed_total",
		Help: "Total number of logs processed",
	}, []string{"host", "service", "level"})

	logsRecentCount = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "log_anomaly_logs_recent_count",
		Help: "Number of recent logs in memory",
	})

	structuredLogsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "log_anomaly_structured_logs_total",
		Help: "Total number of structured (JSON) logs processed",
	}, []string{"host", "service"})

	// Anomaly detection metrics
	anomaliesDetectedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "log_anomaly_anomalies_detected_total",
		Help: "Total number of anomalies detected",
	}, []string{"anomaly_type", "severity", "host", "service"})

	anomalyScore = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "log_anomaly_score",
		Help:    "Distribution of anomaly scores",
		Buckets: []float64{0.5, 1.0, 1.5, 2.0, 2.5, 3.0, 4.0, 5.0, 7.5, 10.0},
	}, []string{"anomaly_type", "host", "service"})

	// Statistical analysis metrics
	frequencyZScore = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "log_anomaly_frequency_z_score",
		Help:    "Distribution of frequency z-scores",
		Buckets: []float64{-5, -3, -2, -1, 0, 1, 2, 3, 5, 10},
	}, []string{"pattern_name", "host", "service"})

	rateChangeMultiplier = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "log_anomaly_rate_change_multiplier",
		Help:    "Distribution of rate change multipliers",
		Buckets: []float64{0.1, 0.2, 0.5, 1.0, 2.0, 5.0, 10.0, 20.0, 50.0},
	}, []string{"pattern_name", "host", "service"})

	// Performance metrics
	processingDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "log_anomaly_processing_duration_seconds",
		Help:    "Time spent processing log batches",
		Buckets: prometheus.DefBuckets,
	}, []string{"operation"})

	lokiQueriesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "log_anomaly_loki_queries_total",
		Help: "Total number of queries to Loki",
	}, []string{"status"})

	lokiQueryDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "log_anomaly_loki_query_duration_seconds",
		Help:    "Time spent querying Loki",
		Buckets: []float64{0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0},
	})

	// Redis metrics
	redisOperationsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "log_anomaly_redis_operations_total",
		Help: "Total number of Redis operations",
	}, []string{"operation", "status"})

	redisOperationDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "log_anomaly_redis_operation_duration_seconds",
		Help:    "Time spent on Redis operations",
		Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25},
	}, []string{"operation"})

	// System health metrics  
	systemStartTime = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "log_anomaly_start_time_seconds",
		Help: "Unix timestamp of when the service started",
	})

	systemUptime = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "log_anomaly_uptime_seconds",
		Help: "Service uptime in seconds",
	})

	lastProcessedTime = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "log_anomaly_last_processed_time_seconds",
		Help: "Unix timestamp of last log processing",
	})

	// Webhook metrics
	webhooksSentTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "log_anomaly_webhooks_sent_total",
		Help: "Total number of webhooks sent",
	}, []string{"status", "severity"})

	webhookDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "log_anomaly_webhook_duration_seconds",
		Help:    "Time spent sending webhooks",
		Buckets: []float64{0.1, 0.25, 0.5, 1.0, 2.5, 5.0},
	})

	webhookRetriesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "log_anomaly_webhook_retries_total",
		Help: "Total number of webhook retry attempts",
	}, []string{"attempt"})

	// Dead letter queue metrics
	deadLetterTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "log_anomaly_dead_letter_total",
		Help: "Total anomalies sent to dead letter queue",
	})

	deadLetterSize = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "log_anomaly_dead_letter_size",
		Help: "Current size of dead letter queue",
	})

	// Cold start metrics
	coldStartActive = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "log_anomaly_cold_start_active",
		Help: "1 if service is in cold start mode (no baselines), 0 otherwise",
	})

	coldStartThresholdMultiplier = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "log_anomaly_cold_start_threshold_multiplier",
		Help: "Current threshold multiplier during cold start",
	})

	// Health status metrics
	healthStatus = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "log_anomaly_health_status",
		Help: "Health status of dependencies (1=up, 0=down)",
	}, []string{"component"})
)

// InitMetrics initializes metrics with startup values
func InitMetrics() {
	systemStartTime.SetToCurrentTime()
}

// UpdateSystemMetrics updates system-level metrics
func UpdateSystemMetrics(startTime float64) {
	systemUptime.Set(time.Since(time.Unix(int64(startTime), 0)).Seconds())
	lastProcessedTime.SetToCurrentTime()
}
