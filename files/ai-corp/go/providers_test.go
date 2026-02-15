package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDummyProvider(t *testing.T) {
	provider := NewDummyProvider("test-dummy")

	if provider.Name() != "test-dummy" {
		t.Errorf("Expected name 'test-dummy', got %s", provider.Name())
	}

	if !provider.IsAvailable() {
		t.Error("Expected dummy provider to be available")
	}

	ctx := context.Background()
	req := LLMRequest{
		Messages: []LLMMessage{
			{Role: "user", Content: "Hello"},
		},
	}

	resp, err := provider.Chat(ctx, req)
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}

	if resp.Content == "" {
		t.Error("Expected non-empty response content")
	}
	if resp.Model != "dummy-model" {
		t.Errorf("Expected model 'dummy-model', got %s", resp.Model)
	}
	if resp.InputTokens != 100 {
		t.Errorf("Expected input tokens 100, got %d", resp.InputTokens)
	}
	if resp.OutputTokens != 50 {
		t.Errorf("Expected output tokens 50, got %d", resp.OutputTokens)
	}
}

func TestOpenAICompatibleProvider(t *testing.T) {
	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"data": []map[string]string{{"id": "test-model"}},
			})
		case "/v1/chat/completions":
			// Verify request
			var req map[string]interface{}
			json.NewDecoder(r.Body).Decode(&req)

			if req["model"] != "test-model" {
				t.Errorf("Expected model 'test-model', got %v", req["model"])
			}

			// Send response
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"choices": []map[string]interface{}{
					{
						"message": map[string]string{
							"content": "Test response",
						},
					},
				},
				"usage": map[string]int{
					"prompt_tokens":     10,
					"completion_tokens": 5,
				},
				"model": "test-model",
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	provider := NewOpenAICompatibleProvider("test", server.URL, "test-model", "")

	if provider.Name() != "test" {
		t.Errorf("Expected name 'test', got %s", provider.Name())
	}

	if !provider.IsAvailable() {
		t.Error("Expected provider to be available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := provider.Chat(ctx, LLMRequest{
		Messages: []LLMMessage{
			{Role: "user", Content: "Hello"},
		},
	})

	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}

	if resp.Content != "Test response" {
		t.Errorf("Expected content 'Test response', got %s", resp.Content)
	}
	if resp.InputTokens != 10 {
		t.Errorf("Expected input tokens 10, got %d", resp.InputTokens)
	}
	if resp.OutputTokens != 5 {
		t.Errorf("Expected output tokens 5, got %d", resp.OutputTokens)
	}
}

func TestOpenAICompatibleProviderError(t *testing.T) {
	// Create a mock server that returns errors
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	provider := NewOpenAICompatibleProvider("test-error", server.URL, "test-model", "")

	ctx := context.Background()
	_, err := provider.Chat(ctx, LLMRequest{
		Messages: []LLMMessage{
			{Role: "user", Content: "Hello"},
		},
	})

	if err == nil {
		t.Error("Expected error from failed request")
	}
}

func TestOpenAICompatibleProviderEmptyResponse(t *testing.T) {
	// Create a mock server that returns empty choices
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{},
		})
	}))
	defer server.Close()

	provider := NewOpenAICompatibleProvider("test-empty", server.URL, "test-model", "")

	ctx := context.Background()
	_, err := provider.Chat(ctx, LLMRequest{
		Messages: []LLMMessage{
			{Role: "user", Content: "Hello"},
		},
	})

	if err == nil {
		t.Error("Expected error from empty response")
	}
}

func TestProviderManagerCreation(t *testing.T) {
	cfg, err := LoadConfig("/nonexistent/config.ini")
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	pm := NewProviderManager(cfg)

	// Local provider should always exist
	provider, err := pm.GetProvider("local")
	if err != nil {
		t.Errorf("Failed to get local provider: %v", err)
	}
	if provider == nil {
		t.Error("Expected local provider to exist")
	}

	// Nonexistent provider
	_, err = pm.GetProvider("nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent provider")
	}
}

func TestProviderManagerGetProviderForRole(t *testing.T) {
	cfg, err := LoadConfig("/nonexistent/config.ini")
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	pm := NewProviderManager(cfg)

	// CEO should use default provider (local)
	provider, err := pm.GetProviderForRole(RoleCEO)
	if err != nil {
		t.Errorf("Failed to get provider for CEO: %v", err)
	}
	if provider == nil {
		t.Error("Expected provider for CEO")
	}
	if provider.Name() != "local" {
		t.Errorf("Expected local provider for CEO, got %s", provider.Name())
	}
}

func TestProviderManagerListProviders(t *testing.T) {
	cfg, err := LoadConfig("/nonexistent/config.ini")
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	pm := NewProviderManager(cfg)
	providers := pm.ListProviders()

	if len(providers) == 0 {
		t.Error("Expected at least one provider")
	}

	// Check that local provider is in the list
	found := false
	for _, p := range providers {
		if p.Name == "local" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected local provider in list")
	}
}

func TestAnthropicProviderAvailability(t *testing.T) {
	// Without API key
	provider := NewAnthropicProvider("anthropic", "claude-3", "")
	if provider.IsAvailable() {
		t.Error("Expected Anthropic provider without API key to be unavailable")
	}

	// With API key
	provider = NewAnthropicProvider("anthropic", "claude-3", "test-key")
	if !provider.IsAvailable() {
		t.Error("Expected Anthropic provider with API key to be available")
	}
}

func TestGoogleProviderAvailability(t *testing.T) {
	// Without API key
	provider := NewGoogleProvider("google", "gemini-pro", "")
	if provider.IsAvailable() {
		t.Error("Expected Google provider without API key to be unavailable")
	}

	// With API key
	provider = NewGoogleProvider("google", "gemini-pro", "test-key")
	if !provider.IsAvailable() {
		t.Error("Expected Google provider with API key to be available")
	}
}

func TestOpenAICompatibleProviderWithAPIKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify Authorization header
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-api-key" {
			t.Errorf("Expected Authorization header 'Bearer test-api-key', got %s", auth)
		}

		if r.URL.Path == "/v1/models" {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{"data": []interface{}{}})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]string{"content": "OK"}},
			},
			"usage": map[string]int{"prompt_tokens": 1, "completion_tokens": 1},
		})
	}))
	defer server.Close()

	provider := NewOpenAICompatibleProvider("test", server.URL, "model", "test-api-key")

	ctx := context.Background()
	_, err := provider.Chat(ctx, LLMRequest{
		Messages: []LLMMessage{{Role: "user", Content: "Test"}},
	})
	if err != nil {
		t.Errorf("Chat failed: %v", err)
	}
}

func TestProviderContextCancellation(t *testing.T) {
	// Create a slow server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	provider := NewOpenAICompatibleProvider("test", server.URL, "model", "")

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := provider.Chat(ctx, LLMRequest{
		Messages: []LLMMessage{{Role: "user", Content: "Test"}},
	})

	if err == nil {
		t.Error("Expected timeout error")
	}
}
