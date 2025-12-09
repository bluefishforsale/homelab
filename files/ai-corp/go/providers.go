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

// Provider defines the interface for LLM providers
type Provider interface {
	Name() string
	Chat(ctx context.Context, req LLMRequest) (*LLMResponse, error)
	IsAvailable() bool
}

// ProviderManager manages multiple LLM providers
type ProviderManager struct {
	providers map[string]Provider
	config    *Config
}

// NewProviderManager creates a new provider manager
func NewProviderManager(config *Config) *ProviderManager {
	pm := &ProviderManager{
		providers: make(map[string]Provider),
		config:    config,
	}

	// Initialize configured providers
	for name, cfg := range config.Providers {
		if !cfg.Enabled {
			continue
		}

		var provider Provider
		switch cfg.Type {
		case "openai_compatible", "local":
			provider = NewOpenAICompatibleProvider(name, cfg.URL, cfg.Model, cfg.APIKey)
		case "openai":
			apiKey := getEnvOrEmpty("OPENAI_API_KEY")
			if apiKey != "" {
				provider = NewOpenAICompatibleProvider(name, "https://api.openai.com", cfg.Model, apiKey)
			}
		case "anthropic":
			apiKey := getEnvOrEmpty("ANTHROPIC_API_KEY")
			if apiKey != "" {
				provider = NewAnthropicProvider(name, cfg.Model, apiKey)
			}
		case "google":
			apiKey := getEnvOrEmpty("GOOGLE_API_KEY")
			if apiKey != "" {
				provider = NewGoogleProvider(name, cfg.Model, apiKey)
			}
		}

		if provider != nil {
			pm.providers[name] = provider
			log.Infof("Initialized provider: %s (%s)", name, cfg.Type)
		}
	}

	// Ensure at least local provider exists
	if _, ok := pm.providers["local"]; !ok {
		pm.providers["local"] = NewOpenAICompatibleProvider("local", "http://192.168.1.143:8080", "default", "")
		log.Info("Initialized default local provider")
	}

	return pm
}

// GetProvider returns a provider by name
func (pm *ProviderManager) GetProvider(name string) (Provider, error) {
	if p, ok := pm.providers[name]; ok {
		return p, nil
	}
	return nil, fmt.Errorf("provider not found: %s", name)
}

// GetProviderForRole returns the provider configured for a role
func (pm *ProviderManager) GetProviderForRole(role RoleName) (Provider, error) {
	roleConfig, ok := pm.config.GetRole(role)
	if !ok {
		return nil, fmt.Errorf("role not configured: %s", role)
	}

	return pm.GetProvider(roleConfig.Provider)
}

// ListProviders returns all configured providers
func (pm *ProviderManager) ListProviders() []ProviderConfig {
	var configs []ProviderConfig
	for name := range pm.providers {
		if cfg, ok := pm.config.GetProvider(name); ok {
			configs = append(configs, cfg)
		}
	}
	return configs
}

// TestProvider tests connectivity to a provider
func (pm *ProviderManager) TestProvider(name string) error {
	provider, err := pm.GetProvider(name)
	if err != nil {
		return err
	}

	if !provider.IsAvailable() {
		return fmt.Errorf("provider %s is not available", name)
	}

	// Simple test request
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = provider.Chat(ctx, LLMRequest{
		Messages: []LLMMessage{
			{Role: "user", Content: "Say 'test' and nothing else."},
		},
		MaxTokens:   10,
		Temperature: 0,
	})

	return err
}

// OpenAICompatibleProvider implements Provider for OpenAI-compatible APIs
type OpenAICompatibleProvider struct {
	name       string
	baseURL    string
	model      string
	apiKey     string
	httpClient *http.Client
}

// NewOpenAICompatibleProvider creates a new OpenAI-compatible provider
func NewOpenAICompatibleProvider(name, baseURL, model, apiKey string) *OpenAICompatibleProvider {
	return &OpenAICompatibleProvider{
		name:    name,
		baseURL: strings.TrimSuffix(baseURL, "/"),
		model:   model,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 300 * time.Second, // 5 minutes for slow LLM responses
		},
	}
}

func (p *OpenAICompatibleProvider) Name() string {
	return p.name
}

func (p *OpenAICompatibleProvider) IsAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, "GET", p.baseURL+"/v1/models", nil)
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

func (p *OpenAICompatibleProvider) Chat(ctx context.Context, req LLMRequest) (*LLMResponse, error) {
	start := time.Now()

	// Use configured model if not specified
	model := req.Model
	if model == "" {
		model = p.model
	}

	body := map[string]interface{}{
		"model":    model,
		"messages": req.Messages,
		"stream":   false,
	}
	if req.MaxTokens > 0 {
		body["max_tokens"] = req.MaxTokens
	}
	if req.Temperature > 0 {
		body["temperature"] = req.Temperature
	}

	jsonData, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/v1/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		llmRequests.WithLabelValues(p.name, "", "error").Inc()
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		llmRequests.WithLabelValues(p.name, "", "error").Inc()
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		llmRequests.WithLabelValues(p.name, "", "error").Inc()
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
		Model string `json:"model"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		llmRequests.WithLabelValues(p.name, "", "error").Inc()
		return nil, err
	}

	if len(result.Choices) == 0 {
		llmRequests.WithLabelValues(p.name, "", "error").Inc()
		return nil, fmt.Errorf("no response from LLM")
	}

	latency := int(time.Since(start).Milliseconds())
	llmRequests.WithLabelValues(p.name, "", "success").Inc()
	llmLatency.WithLabelValues(p.name).Observe(time.Since(start).Seconds())
	llmTokensInput.WithLabelValues(p.name).Add(float64(result.Usage.PromptTokens))
	llmTokensOutput.WithLabelValues(p.name).Add(float64(result.Usage.CompletionTokens))

	return &LLMResponse{
		Content:      strings.TrimSpace(result.Choices[0].Message.Content),
		Model:        result.Model,
		InputTokens:  result.Usage.PromptTokens,
		OutputTokens: result.Usage.CompletionTokens,
		LatencyMs:    latency,
	}, nil
}

// AnthropicProvider implements Provider for Claude API
type AnthropicProvider struct {
	name       string
	model      string
	apiKey     string
	httpClient *http.Client
}

// NewAnthropicProvider creates a new Anthropic provider
func NewAnthropicProvider(name, model, apiKey string) *AnthropicProvider {
	return &AnthropicProvider{
		name:   name,
		model:  model,
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

func (p *AnthropicProvider) Name() string {
	return p.name
}

func (p *AnthropicProvider) IsAvailable() bool {
	return p.apiKey != ""
}

func (p *AnthropicProvider) Chat(ctx context.Context, req LLMRequest) (*LLMResponse, error) {
	start := time.Now()

	model := req.Model
	if model == "" {
		model = p.model
	}

	// Convert messages to Anthropic format
	var system string
	var messages []map[string]string

	for _, msg := range req.Messages {
		if msg.Role == "system" {
			system = msg.Content
		} else {
			messages = append(messages, map[string]string{
				"role":    msg.Role,
				"content": msg.Content,
			})
		}
	}

	body := map[string]interface{}{
		"model":      model,
		"messages":   messages,
		"max_tokens": req.MaxTokens,
	}
	if system != "" {
		body["system"] = system
	}
	if req.Temperature > 0 {
		body["temperature"] = req.Temperature
	}

	jsonData, _ := json.Marshal(body)

	httpReq, _ := http.NewRequestWithContext(ctx, "POST", "https://api.anthropic.com/v1/messages", bytes.NewBuffer(jsonData))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		llmRequests.WithLabelValues(p.name, "", "error").Inc()
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		llmRequests.WithLabelValues(p.name, "", "error").Inc()
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
		Model string `json:"model"`
	}

	json.Unmarshal(respBody, &result)

	if len(result.Content) == 0 {
		llmRequests.WithLabelValues(p.name, "", "error").Inc()
		return nil, fmt.Errorf("no response from Claude")
	}

	latency := int(time.Since(start).Milliseconds())
	llmRequests.WithLabelValues(p.name, "", "success").Inc()
	llmLatency.WithLabelValues(p.name).Observe(time.Since(start).Seconds())
	llmTokensInput.WithLabelValues(p.name).Add(float64(result.Usage.InputTokens))
	llmTokensOutput.WithLabelValues(p.name).Add(float64(result.Usage.OutputTokens))

	return &LLMResponse{
		Content:      strings.TrimSpace(result.Content[0].Text),
		Model:        result.Model,
		InputTokens:  result.Usage.InputTokens,
		OutputTokens: result.Usage.OutputTokens,
		LatencyMs:    latency,
	}, nil
}

// GoogleProvider implements Provider for Gemini API
type GoogleProvider struct {
	name       string
	model      string
	apiKey     string
	httpClient *http.Client
}

// NewGoogleProvider creates a new Google Gemini provider
func NewGoogleProvider(name, model, apiKey string) *GoogleProvider {
	return &GoogleProvider{
		name:   name,
		model:  model,
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

func (p *GoogleProvider) Name() string {
	return p.name
}

func (p *GoogleProvider) IsAvailable() bool {
	return p.apiKey != ""
}

func (p *GoogleProvider) Chat(ctx context.Context, req LLMRequest) (*LLMResponse, error) {
	start := time.Now()

	model := req.Model
	if model == "" {
		model = p.model
	}

	// Convert to Gemini format
	var contents []map[string]interface{}
	for _, msg := range req.Messages {
		role := msg.Role
		if role == "assistant" {
			role = "model"
		}
		contents = append(contents, map[string]interface{}{
			"role": role,
			"parts": []map[string]string{
				{"text": msg.Content},
			},
		})
	}

	body := map[string]interface{}{
		"contents": contents,
	}

	if req.MaxTokens > 0 || req.Temperature > 0 {
		config := map[string]interface{}{}
		if req.MaxTokens > 0 {
			config["maxOutputTokens"] = req.MaxTokens
		}
		if req.Temperature > 0 {
			config["temperature"] = req.Temperature
		}
		body["generationConfig"] = config
	}

	jsonData, _ := json.Marshal(body)

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", model, p.apiKey)
	httpReq, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		llmRequests.WithLabelValues(p.name, "", "error").Inc()
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		llmRequests.WithLabelValues(p.name, "", "error").Inc()
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
		UsageMetadata struct {
			PromptTokenCount     int `json:"promptTokenCount"`
			CandidatesTokenCount int `json:"candidatesTokenCount"`
		} `json:"usageMetadata"`
	}

	json.Unmarshal(respBody, &result)

	if len(result.Candidates) == 0 || len(result.Candidates[0].Content.Parts) == 0 {
		llmRequests.WithLabelValues(p.name, "", "error").Inc()
		return nil, fmt.Errorf("no response from Gemini")
	}

	latency := int(time.Since(start).Milliseconds())
	llmRequests.WithLabelValues(p.name, "", "success").Inc()
	llmLatency.WithLabelValues(p.name).Observe(time.Since(start).Seconds())
	llmTokensInput.WithLabelValues(p.name).Add(float64(result.UsageMetadata.PromptTokenCount))
	llmTokensOutput.WithLabelValues(p.name).Add(float64(result.UsageMetadata.CandidatesTokenCount))

	return &LLMResponse{
		Content:      strings.TrimSpace(result.Candidates[0].Content.Parts[0].Text),
		Model:        model,
		InputTokens:  result.UsageMetadata.PromptTokenCount,
		OutputTokens: result.UsageMetadata.CandidatesTokenCount,
		LatencyMs:    latency,
	}, nil
}

// DummyProvider is a mock provider for testing
type DummyProvider struct {
	name string
}

// NewDummyProvider creates a new dummy provider
func NewDummyProvider(name string) *DummyProvider {
	return &DummyProvider{name: name}
}

func (p *DummyProvider) Name() string {
	return p.name
}

func (p *DummyProvider) IsAvailable() bool {
	return true
}

func (p *DummyProvider) Chat(ctx context.Context, req LLMRequest) (*LLMResponse, error) {
	// Simulate processing time
	time.Sleep(100 * time.Millisecond)

	// Generate mock response based on content
	response := fmt.Sprintf("[DUMMY RESPONSE from %s]\nReceived %d messages.\nLast message: %.100s...",
		p.name, len(req.Messages), req.Messages[len(req.Messages)-1].Content)

	return &LLMResponse{
		Content:      response,
		Model:        "dummy-model",
		InputTokens:  100,
		OutputTokens: 50,
		LatencyMs:    100,
	}, nil
}

// getEnvOrEmpty returns env value or empty string
func getEnvOrEmpty(key string) string {
	return strings.TrimSpace(getEnv(key, ""))
}
