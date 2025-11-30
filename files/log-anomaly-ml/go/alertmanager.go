package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"
)

// AlertManagerClient handles AlertManager API communication
type AlertManagerClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewAlertManagerClient creates a new AlertManager client
func NewAlertManagerClient(baseURL string) *AlertManagerClient {
	return &AlertManagerClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// HealthCheck checks if AlertManager is reachable
func (c *AlertManagerClient) HealthCheck() error {
	resp, err := c.httpClient.Get(c.baseURL + "/-/healthy")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("alertmanager returned status %d", resp.StatusCode)
	}
	return nil
}

// SendAlert sends an alert to AlertManager
func (c *AlertManagerClient) SendAlert(problem *Problem) error {
	alert := AlertManagerAlert{
		Labels: map[string]string{
			"alertname":  "LogAnomaly",
			"severity":   problem.Severity,
			"problem_id": problem.ID,
		},
		Annotations: map[string]string{
			"summary":     problem.Title,
			"description": formatAlertDescription(problem),
			"dashboard":   fmt.Sprintf("http://192.168.1.143:8910/d/log-anomalies?var-problem=%s", problem.ID),
		},
		StartsAt: problem.FirstSeen.Format(time.RFC3339),
	}

	// Add host/service labels if available
	if len(problem.AffectedHosts) > 0 {
		alert.Labels["host"] = problem.AffectedHosts[0]
	}
	if len(problem.AffectedServices) > 0 {
		alert.Labels["service"] = problem.AffectedServices[0]
	}

	// AlertManager expects an array of alerts
	alerts := []AlertManagerAlert{alert}

	jsonData, err := json.Marshal(alerts)
	if err != nil {
		return fmt.Errorf("failed to marshal alert: %w", err)
	}

	resp, err := c.httpClient.Post(
		c.baseURL+"/api/v2/alerts",
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		alertsSent.WithLabelValues(problem.Severity, "alertmanager", "error").Inc()
		return fmt.Errorf("failed to send alert: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		alertsSent.WithLabelValues(problem.Severity, "alertmanager", "error").Inc()
		return fmt.Errorf("alertmanager returned status %d", resp.StatusCode)
	}

	alertsSent.WithLabelValues(problem.Severity, "alertmanager", "success").Inc()
	log.WithFields(log.Fields{
		"problem_id": problem.ID,
		"severity":   problem.Severity,
		"title":      problem.Title,
	}).Info("Alert sent to AlertManager")

	return nil
}

// ResolveAlert sends a resolution to AlertManager
func (c *AlertManagerClient) ResolveAlert(problem *Problem) error {
	now := time.Now()
	alert := AlertManagerAlert{
		Labels: map[string]string{
			"alertname":  "LogAnomaly",
			"severity":   problem.Severity,
			"problem_id": problem.ID,
		},
		Annotations: map[string]string{
			"summary": problem.Title + " (resolved)",
		},
		StartsAt: problem.FirstSeen.Format(time.RFC3339),
		EndsAt:   now.Format(time.RFC3339),
	}

	alerts := []AlertManagerAlert{alert}

	jsonData, err := json.Marshal(alerts)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Post(
		c.baseURL+"/api/v2/alerts",
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

func formatAlertDescription(p *Problem) string {
	desc := fmt.Sprintf("%d occurrences over %s.", p.OccurrenceCount, p.DurationString())
	if p.LLMAnalysis != "" {
		desc += " " + p.LLMAnalysis
	}
	return desc
}
