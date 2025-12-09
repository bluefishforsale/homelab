package main

import (
	"os"
	"testing"
)

func TestLoadConfigDefaults(t *testing.T) {
	// Test loading config with no file (should use defaults)
	cfg, err := LoadConfig("/nonexistent/config.ini")
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Check defaults
	if cfg.Port != 8088 {
		t.Errorf("Expected port 8088, got %d", cfg.Port)
	}
	if cfg.Host != "0.0.0.0" {
		t.Errorf("Expected host 0.0.0.0, got %s", cfg.Host)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("Expected log level info, got %s", cfg.LogLevel)
	}
	if cfg.PostgresHost != "postgres" {
		t.Errorf("Expected postgres host 'postgres', got %s", cfg.PostgresHost)
	}
	if cfg.PostgresPort != 5432 {
		t.Errorf("Expected postgres port 5432, got %d", cfg.PostgresPort)
	}
	if cfg.RedisAddr != "redis:6379" {
		t.Errorf("Expected redis addr 'redis:6379', got %s", cfg.RedisAddr)
	}
	if cfg.DefaultProvider != "local" {
		t.Errorf("Expected default provider 'local', got %s", cfg.DefaultProvider)
	}
	if cfg.MaxConcurrentWorkflows != 5 {
		t.Errorf("Expected max concurrent workflows 5, got %d", cfg.MaxConcurrentWorkflows)
	}
}

func TestLoadConfigEnvOverrides(t *testing.T) {
	// Set environment variables
	os.Setenv("PORT", "9000")
	os.Setenv("HOST", "127.0.0.1")
	os.Setenv("LOG_LEVEL", "debug")
	os.Setenv("POSTGRES_HOST", "localhost")
	os.Setenv("POSTGRES_PORT", "5433")
	os.Setenv("POSTGRES_PASSWORD", "testpass")
	defer func() {
		os.Unsetenv("PORT")
		os.Unsetenv("HOST")
		os.Unsetenv("LOG_LEVEL")
		os.Unsetenv("POSTGRES_HOST")
		os.Unsetenv("POSTGRES_PORT")
		os.Unsetenv("POSTGRES_PASSWORD")
	}()

	cfg, err := LoadConfig("/nonexistent/config.ini")
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.Port != 9000 {
		t.Errorf("Expected port 9000, got %d", cfg.Port)
	}
	if cfg.Host != "127.0.0.1" {
		t.Errorf("Expected host 127.0.0.1, got %s", cfg.Host)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("Expected log level debug, got %s", cfg.LogLevel)
	}
	if cfg.PostgresHost != "localhost" {
		t.Errorf("Expected postgres host localhost, got %s", cfg.PostgresHost)
	}
	if cfg.PostgresPort != 5433 {
		t.Errorf("Expected postgres port 5433, got %d", cfg.PostgresPort)
	}
	if cfg.PostgresPassword != "testpass" {
		t.Errorf("Expected postgres password testpass, got %s", cfg.PostgresPassword)
	}
}

func TestConfigValidation(t *testing.T) {
	cfg := &Config{
		Port: 0,
	}
	err := cfg.validate()
	if err == nil {
		t.Error("Expected validation error for port 0")
	}

	cfg.Port = 70000
	err = cfg.validate()
	if err == nil {
		t.Error("Expected validation error for port 70000")
	}

	cfg.Port = 8088
	err = cfg.validate()
	if err != nil {
		t.Errorf("Unexpected validation error: %v", err)
	}
}

func TestDefaultRoles(t *testing.T) {
	cfg, err := LoadConfig("/nonexistent/config.ini")
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Check all default roles exist
	expectedRoles := []RoleName{RoleBoard, RoleCEO, RoleCTO, RoleMarketing, RoleArtist, RoleWorker}
	for _, role := range expectedRoles {
		if _, ok := cfg.GetRole(role); !ok {
			t.Errorf("Expected role %s to exist", role)
		}
	}

	// Check role properties
	ceo, ok := cfg.GetRole(RoleCEO)
	if !ok {
		t.Fatal("CEO role not found")
	}
	if ceo.Label != "CEO" {
		t.Errorf("Expected CEO label 'CEO', got %s", ceo.Label)
	}
	if ceo.Persona == "" {
		t.Error("Expected CEO persona to be set")
	}
}

func TestGetProvider(t *testing.T) {
	cfg, err := LoadConfig("/nonexistent/config.ini")
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Local provider should exist by default
	provider, ok := cfg.GetProvider("local")
	if !ok {
		t.Error("Expected local provider to exist")
	}
	if provider.Type != "openai_compatible" {
		t.Errorf("Expected local provider type 'openai_compatible', got %s", provider.Type)
	}

	// Nonexistent provider
	_, ok = cfg.GetProvider("nonexistent")
	if ok {
		t.Error("Expected nonexistent provider to not exist")
	}
}

func TestStorageConfigDefaults(t *testing.T) {
	cfg, err := LoadConfig("/nonexistent/config.ini")
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Storage defaults
	if cfg.StorageType != StorageLocal {
		t.Errorf("Expected storage type 'local', got %s", cfg.StorageType)
	}
	if cfg.StorageBasePath != "/app/artifacts" {
		t.Errorf("Expected storage base path '/app/artifacts', got %s", cfg.StorageBasePath)
	}
}

func TestStorageConfigFromFile(t *testing.T) {
	content := `
[storage]
type = s3
base_path = /custom/path

[storage.s3]
bucket = my-bucket
region = us-west-2
`
	tmpFile, err := os.CreateTemp("", "config-storage-*.ini")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}
	tmpFile.Close()

	cfg, err := LoadConfig(tmpFile.Name())
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.StorageType != StorageS3 {
		t.Errorf("Expected storage type 's3', got %s", cfg.StorageType)
	}
	if cfg.StorageBasePath != "/custom/path" {
		t.Errorf("Expected storage base path '/custom/path', got %s", cfg.StorageBasePath)
	}
	if cfg.S3Bucket != "my-bucket" {
		t.Errorf("Expected S3 bucket 'my-bucket', got %s", cfg.S3Bucket)
	}
	if cfg.S3Region != "us-west-2" {
		t.Errorf("Expected S3 region 'us-west-2', got %s", cfg.S3Region)
	}
}

func TestStorageEnvOverrides(t *testing.T) {
	os.Setenv("STORAGE_TYPE", "gcs")
	os.Setenv("STORAGE_BASE_PATH", "/env/path")
	os.Setenv("GCS_BUCKET", "env-bucket")
	defer func() {
		os.Unsetenv("STORAGE_TYPE")
		os.Unsetenv("STORAGE_BASE_PATH")
		os.Unsetenv("GCS_BUCKET")
	}()

	cfg, err := LoadConfig("/nonexistent/config.ini")
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.StorageType != StorageGCS {
		t.Errorf("Expected storage type 'gcs', got %s", cfg.StorageType)
	}
	if cfg.StorageBasePath != "/env/path" {
		t.Errorf("Expected storage base path '/env/path', got %s", cfg.StorageBasePath)
	}
	if cfg.GCSBucket != "env-bucket" {
		t.Errorf("Expected GCS bucket 'env-bucket', got %s", cfg.GCSBucket)
	}
}

func TestLoadConfigFromFile(t *testing.T) {
	// Create a temporary config file
	content := `
[server]
port = 9999
host = 0.0.0.0
log_level = debug

[database]
host = testdb
port = 5555
name = testdb
user = testuser

[redis]
addr = testredis:6380
db = 1

[providers]
default = testprovider

[providers.testprovider]
type = openai_compatible
url = http://test:8080
model = testmodel

[roles.custom]
name = Custom Role
provider = testprovider
persona = Custom persona text

[limits]
max_concurrent_workflows = 10
rate_limit_rpm = 120
`
	tmpFile, err := os.CreateTemp("", "config-*.ini")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}
	tmpFile.Close()

	cfg, err := LoadConfig(tmpFile.Name())
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.Port != 9999 {
		t.Errorf("Expected port 9999, got %d", cfg.Port)
	}
	if cfg.PostgresHost != "testdb" {
		t.Errorf("Expected postgres host 'testdb', got %s", cfg.PostgresHost)
	}
	if cfg.PostgresPort != 5555 {
		t.Errorf("Expected postgres port 5555, got %d", cfg.PostgresPort)
	}
	if cfg.RedisAddr != "testredis:6380" {
		t.Errorf("Expected redis addr 'testredis:6380', got %s", cfg.RedisAddr)
	}
	if cfg.DefaultProvider != "testprovider" {
		t.Errorf("Expected default provider 'testprovider', got %s", cfg.DefaultProvider)
	}
	if cfg.MaxConcurrentWorkflows != 10 {
		t.Errorf("Expected max concurrent workflows 10, got %d", cfg.MaxConcurrentWorkflows)
	}

	// Check custom provider
	provider, ok := cfg.GetProvider("testprovider")
	if !ok {
		t.Error("Expected testprovider to exist")
	}
	if provider.URL != "http://test:8080" {
		t.Errorf("Expected provider URL 'http://test:8080', got %s", provider.URL)
	}

	// Check custom role
	role, ok := cfg.GetRole("custom")
	if !ok {
		t.Error("Expected custom role to exist")
	}
	if role.Label != "Custom Role" {
		t.Errorf("Expected role label 'Custom Role', got %s", role.Label)
	}
	if role.Persona != "Custom persona text" {
		t.Errorf("Expected role persona 'Custom persona text', got %s", role.Persona)
	}
}
