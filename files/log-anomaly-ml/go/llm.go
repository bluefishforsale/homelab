package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

// LLMClient handles communication with the LLM server
type LLMClient struct {
	baseURL    string
	httpClient *http.Client
}

// ChatMessage represents a message in the chat format
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest represents the OpenAI-compatible chat completion request
type ChatRequest struct {
	Messages    []ChatMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature float64       `json:"temperature,omitempty"`
	Stream      bool          `json:"stream"`
}

// ChatResponse represents the OpenAI-compatible chat completion response
type ChatResponse struct {
	Choices []struct {
		Message ChatMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// NewLLMClient creates a new LLM client
func NewLLMClient(baseURL string) *LLMClient {
	return &LLMClient{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 60 * time.Second, // LLM can be slow
		},
	}
}

// AnalyzeProblem generates an analysis of the problem using the LLM
func (c *LLMClient) AnalyzeProblem(ctx context.Context, problem *Problem) (string, error) {
	start := time.Now()
	defer func() {
		llmRequestDuration.Observe(time.Since(start).Seconds())
	}()

	// Build context for the LLM
	prompt := c.buildAnalysisPrompt(problem)

	request := ChatRequest{
		Messages: []ChatMessage{
			{
				Role: "system",
				Content: `You are a senior SRE analyzing log anomalies in a homelab environment. 
Provide a brief, actionable analysis (2-3 sentences max) that includes:
1. The likely root cause
2. One specific remediation step

Be concise and technical. No pleasantries or verbose explanations.
Focus on actionable insights for: Docker containers, systemd services, databases, networking.`,
			},
			{
				Role:    "user",
				Content: prompt,
			},
		},
		MaxTokens:   150,
		Temperature: 0.3, // Lower temperature for more focused responses
		Stream:      false,
	}

	jsonData, err := json.Marshal(request)
	if err != nil {
		llmRequestsTotal.WithLabelValues("error").Inc()
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/v1/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		llmRequestsTotal.WithLabelValues("error").Inc()
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		llmRequestsTotal.WithLabelValues("error").Inc()
		return "", fmt.Errorf("failed to call LLM: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		llmRequestsTotal.WithLabelValues("error").Inc()
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		llmRequestsTotal.WithLabelValues("error").Inc()
		return "", fmt.Errorf("LLM returned status %d: %s", resp.StatusCode, string(body))
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		llmRequestsTotal.WithLabelValues("error").Inc()
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if chatResp.Error != nil {
		llmRequestsTotal.WithLabelValues("error").Inc()
		return "", fmt.Errorf("LLM error: %s", chatResp.Error.Message)
	}

	if len(chatResp.Choices) == 0 {
		llmRequestsTotal.WithLabelValues("error").Inc()
		return "", fmt.Errorf("no response from LLM")
	}

	llmRequestsTotal.WithLabelValues("success").Inc()
	analysis := strings.TrimSpace(chatResp.Choices[0].Message.Content)
	
	// Truncate if too long
	if len(analysis) > 500 {
		analysis = analysis[:497] + "..."
	}

	return analysis, nil
}

// buildAnalysisPrompt creates a prompt from the problem data
func (c *LLMClient) buildAnalysisPrompt(problem *Problem) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Problem: %s\n", problem.Title))
	sb.WriteString(fmt.Sprintf("Severity: %s\n", problem.Severity))
	sb.WriteString(fmt.Sprintf("Duration: %s\n", problem.DurationString()))
	sb.WriteString(fmt.Sprintf("Occurrences: %d\n", problem.OccurrenceCount))

	if len(problem.AffectedHosts) > 0 {
		sb.WriteString(fmt.Sprintf("Hosts: %s\n", strings.Join(problem.AffectedHosts, ", ")))
	}
	if len(problem.AffectedServices) > 0 {
		sb.WriteString(fmt.Sprintf("Services: %s\n", strings.Join(problem.AffectedServices, ", ")))
	}

	if len(problem.SampleAnomalies) > 0 {
		sb.WriteString("\nSample log messages:\n")
		for i, sample := range problem.SampleAnomalies {
			if i >= 3 {
				break // Limit to 3 samples
			}
			// Truncate long samples
			if len(sample) > 200 {
				sample = sample[:197] + "..."
			}
			sb.WriteString(fmt.Sprintf("- %s\n", sample))
		}
	}

	return sb.String()
}

// IsAvailable checks if the LLM server is reachable
func (c *LLMClient) IsAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/health", nil)
	if err != nil {
		return false
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Debugf("LLM health check failed: %v", err)
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// AnalyzeProblemAsync analyzes a problem in the background
func (c *LLMClient) AnalyzeProblemAsync(problem *Problem, db *Database) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		analysis, err := c.AnalyzeProblem(ctx, problem)
		if err != nil {
			log.Warnf("LLM analysis failed for problem %s: %v", problem.ID, err)
			return
		}

		// Update problem with analysis
		if err := db.UpdateLLMAnalysis(problem.ID, analysis); err != nil {
			log.Warnf("Failed to save LLM analysis for problem %s: %v", problem.ID, err)
			return
		}

		log.Infof("LLM analysis complete for problem %s", problem.ID)
	}()
}
