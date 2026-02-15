package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// MockApp creates a minimal app for testing handlers
func createMockApp() *App {
	cfg, _ := LoadConfig("/nonexistent/config.ini")
	return &App{
		config:    cfg,
		providers: NewProviderManager(cfg),
		startTime: time.Now(),
	}
}

func TestHealthHandler(t *testing.T) {
	// This test requires mocking db and redis, so we'll test a simplified version
	t.Skip("Requires database and redis mocks - run integration tests instead")
}

func TestListRolesHandler(t *testing.T) {
	app := createMockApp()
	handlers := NewHandlers(app)

	req := httptest.NewRequest("GET", "/api/v1/roles", nil)
	rec := httptest.NewRecorder()

	handlers.ListRolesHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	roles, ok := response["roles"].([]interface{})
	if !ok {
		t.Fatal("Expected roles array in response")
	}

	// Should have at least the default roles
	if len(roles) < 6 {
		t.Errorf("Expected at least 6 roles, got %d", len(roles))
	}

	total, ok := response["total"].(float64)
	if !ok {
		t.Fatal("Expected total in response")
	}
	if int(total) != len(roles) {
		t.Errorf("Expected total %d, got %d", len(roles), int(total))
	}
}

func TestListProvidersHandler(t *testing.T) {
	app := createMockApp()
	handlers := NewHandlers(app)

	req := httptest.NewRequest("GET", "/api/v1/providers", nil)
	rec := httptest.NewRecorder()

	handlers.ListProvidersHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	providers, ok := response["providers"].([]interface{})
	if !ok {
		t.Fatal("Expected providers array in response")
	}

	// Should have at least the local provider
	if len(providers) < 1 {
		t.Error("Expected at least 1 provider")
	}

	// Check that local provider exists
	found := false
	for _, p := range providers {
		provider := p.(map[string]interface{})
		if provider["name"] == "local" {
			found = true
			if provider["type"] != "openai_compatible" {
				t.Errorf("Expected local provider type 'openai_compatible', got %v", provider["type"])
			}
			break
		}
	}
	if !found {
		t.Error("Expected local provider in response")
	}
}

func TestCORSMiddleware(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := CORSMiddleware(handler)

	// Test regular request
	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("Expected CORS header Access-Control-Allow-Origin: *")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	// Test OPTIONS preflight
	req = httptest.NewRequest("OPTIONS", "/test", nil)
	rec = httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200 for OPTIONS, got %d", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Methods") == "" {
		t.Error("Expected Access-Control-Allow-Methods header")
	}
}

func TestLoggingMiddleware(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte("test"))
	})

	wrapped := LoggingMiddleware(handler)

	req := httptest.NewRequest("POST", "/test", nil)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("Expected status 201, got %d", rec.Code)
	}
}

func TestWriteError(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, "Test error", http.StatusBadRequest)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", rec.Code)
	}

	var response ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response.Error != "Test error" {
		t.Errorf("Expected error 'Test error', got %s", response.Error)
	}
}

func TestResponseWriter(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: rec, statusCode: http.StatusOK}

	rw.WriteHeader(http.StatusNotFound)

	if rw.statusCode != http.StatusNotFound {
		t.Errorf("Expected status code 404, got %d", rw.statusCode)
	}
	if rec.Code != http.StatusNotFound {
		t.Errorf("Expected recorded status 404, got %d", rec.Code)
	}
}

func TestMidjourneyWebhookHandler(t *testing.T) {
	app := createMockApp()
	handlers := NewHandlers(app)

	payload := map[string]interface{}{
		"id":     "test-id",
		"status": "completed",
		"url":    "https://example.com/image.png",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest("POST", "/webhook/midjourney", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handlers.MidjourneyWebhookHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}
}

func TestMidjourneyWebhookHandlerInvalidJSON(t *testing.T) {
	app := createMockApp()
	handlers := NewHandlers(app)

	req := httptest.NewRequest("POST", "/webhook/midjourney", bytes.NewReader([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handlers.MidjourneyWebhookHandler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", rec.Code)
	}
}

func TestContentType(t *testing.T) {
	app := createMockApp()
	handlers := NewHandlers(app)

	testCases := []struct {
		name       string
		handler    func(http.ResponseWriter, *http.Request)
		method     string
		path       string
		expectJSON bool
	}{
		{"ListRoles", handlers.ListRolesHandler, "GET", "/api/v1/roles", true},
		{"ListProviders", handlers.ListProvidersHandler, "GET", "/api/v1/providers", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			rec := httptest.NewRecorder()
			tc.handler(rec, req)

			contentType := rec.Header().Get("Content-Type")
			if tc.expectJSON && contentType != "application/json" {
				t.Errorf("Expected Content-Type 'application/json', got %s", contentType)
			}
		})
	}
}
