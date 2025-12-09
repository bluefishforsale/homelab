package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
	"gopkg.in/ini.v1"
)

// Config holds all application configuration
type Config struct {
	// Server
	Port     int
	Host     string
	LogLevel string

	// Database
	PostgresHost     string
	PostgresPort     int
	PostgresDB       string
	PostgresUser     string
	PostgresPassword string

	// Redis
	RedisAddr     string
	RedisPassword string
	RedisDB       int

	// Providers
	DefaultProvider  string
	EnableMCP        bool
	EnableMidjourney bool
	Providers        map[string]ProviderConfig

	// Midjourney
	MidjourneyURL        string
	MidjourneyAPIKey     string
	MidjourneyWebhookURL string

	// Roles
	Roles map[RoleName]Role

	// Workflows
	WorkflowsDir string

	// Limits
	MaxConcurrentWorkflows int
	MaxStepsPerWorkflow    int
	RateLimitRPM           int
	ContextCacheTTL        int

	// Storage
	StorageType             StorageType
	StorageBasePath         string
	S3Endpoint              string
	S3Bucket                string
	S3Region                string
	S3AccessKey             string
	S3SecretKey             string
	GCSBucket               string
	GCSCredentialsFile      string
	GoogleDriveFolderID     string
	GoogleDriveCredentials  string
}

// LoadConfig loads configuration from INI file and environment
func LoadConfig(path string) (*Config, error) {
	cfg := &Config{
		// Defaults
		Port:                   8088,
		Host:                   "0.0.0.0",
		LogLevel:               "info",
		PostgresHost:           "postgres",
		PostgresPort:           5432,
		PostgresDB:             "ai_corp",
		PostgresUser:           "ai_corp",
		RedisAddr:              "redis:6379",
		RedisDB:                0,
		DefaultProvider:        "local",
		WorkflowsDir:           "/app/workflows",
		MaxConcurrentWorkflows: 5,
		MaxStepsPerWorkflow:    50,
		RateLimitRPM:           60,
		ContextCacheTTL:        3600,
		Providers:              make(map[string]ProviderConfig),
		Roles:                  make(map[RoleName]Role),
		// Storage defaults (local for development)
		StorageType:            StorageLocal,
		StorageBasePath:        "/app/artifacts",
	}

	// Try to load INI file if it exists
	if _, err := os.Stat(path); err == nil {
		iniCfg, err := ini.Load(path)
		if err != nil {
			return nil, fmt.Errorf("failed to load config file: %w", err)
		}

		if err := cfg.parseINI(iniCfg); err != nil {
			return nil, fmt.Errorf("failed to parse config: %w", err)
		}

		log.Infof("Loaded configuration from %s", path)
	} else {
		log.Warnf("Config file not found at %s, using defaults", path)
	}

	// Override with environment variables
	cfg.applyEnvOverrides()

	// Validate
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}

// parseINI parses the INI file into config
func (c *Config) parseINI(cfg *ini.File) error {
	// [server]
	if sec := cfg.Section("server"); sec != nil {
		c.Port = sec.Key("port").MustInt(c.Port)
		c.Host = sec.Key("host").MustString(c.Host)
		c.LogLevel = sec.Key("log_level").MustString(c.LogLevel)
	}

	// [database]
	if sec := cfg.Section("database"); sec != nil {
		c.PostgresHost = sec.Key("host").MustString(c.PostgresHost)
		c.PostgresPort = sec.Key("port").MustInt(c.PostgresPort)
		c.PostgresDB = sec.Key("name").MustString(c.PostgresDB)
		c.PostgresUser = sec.Key("user").MustString(c.PostgresUser)
	}

	// [redis]
	if sec := cfg.Section("redis"); sec != nil {
		c.RedisAddr = sec.Key("addr").MustString(c.RedisAddr)
		c.RedisDB = sec.Key("db").MustInt(c.RedisDB)
	}

	// [providers]
	if sec := cfg.Section("providers"); sec != nil {
		c.DefaultProvider = sec.Key("default").MustString(c.DefaultProvider)
		c.EnableMCP = sec.Key("enable_mcp").MustBool(false)
		c.EnableMidjourney = sec.Key("enable_midjourney").MustBool(false)
	}

	// Parse provider sections
	for _, sec := range cfg.Sections() {
		if strings.HasPrefix(sec.Name(), "providers.") {
			name := strings.TrimPrefix(sec.Name(), "providers.")
			apiKey := c.expandEnv(sec.Key("api_key").String())
			provider := ProviderConfig{
				Name:    name,
				Type:    sec.Key("type").MustString("openai_compatible"),
				URL:     c.expandEnv(sec.Key("url").String()),
				Model:   sec.Key("model").MustString("default"),
				APIKey:  apiKey,
				Enabled: true,
			}
			// Check for api_key to determine if enabled (except for local/openai_compatible which might not need auth)
			if apiKey == "" && provider.Type != "openai_compatible" && provider.Type != "local" {
				provider.Enabled = false
			}
			c.Providers[name] = provider
		}
	}

	// [midjourney]
	if sec := cfg.Section("midjourney"); sec != nil {
		c.EnableMidjourney = sec.Key("enabled").MustBool(c.EnableMidjourney)
		c.MidjourneyURL = c.expandEnv(sec.Key("api_url").String())
		c.MidjourneyAPIKey = c.expandEnv(sec.Key("api_key").String())
		c.MidjourneyWebhookURL = sec.Key("webhook_url").MustString("")
	}

	// Parse role sections
	for _, sec := range cfg.Sections() {
		if strings.HasPrefix(sec.Name(), "roles.") {
			name := RoleName(strings.TrimPrefix(sec.Name(), "roles."))
			role := Role{
				Name:     name,
				Label:    sec.Key("name").MustString(string(name)),
				Provider: sec.Key("provider").MustString(c.DefaultProvider),
				Persona:  sec.Key("persona").MustString(""),
				Model:    sec.Key("model").MustString(""),
			}
			c.Roles[name] = role
		}
	}

	// [workflows]
	if sec := cfg.Section("workflows"); sec != nil {
		c.WorkflowsDir = sec.Key("templates_dir").MustString(c.WorkflowsDir)
	}

	// [limits]
	if sec := cfg.Section("limits"); sec != nil {
		c.MaxConcurrentWorkflows = sec.Key("max_concurrent_workflows").MustInt(c.MaxConcurrentWorkflows)
		c.MaxStepsPerWorkflow = sec.Key("max_steps_per_workflow").MustInt(c.MaxStepsPerWorkflow)
		c.RateLimitRPM = sec.Key("rate_limit_rpm").MustInt(c.RateLimitRPM)
		c.ContextCacheTTL = sec.Key("context_cache_ttl").MustInt(c.ContextCacheTTL)
	}

	// [storage]
	if sec := cfg.Section("storage"); sec != nil {
		storageType := sec.Key("type").MustString(string(c.StorageType))
		c.StorageType = StorageType(storageType)
		c.StorageBasePath = sec.Key("base_path").MustString(c.StorageBasePath)
	}

	// [storage.s3]
	if sec := cfg.Section("storage.s3"); sec != nil {
		c.S3Endpoint = c.expandEnv(sec.Key("endpoint").String())
		c.S3Bucket = sec.Key("bucket").MustString("")
		c.S3Region = sec.Key("region").MustString("us-east-1")
		c.S3AccessKey = c.expandEnv(sec.Key("access_key").String())
		c.S3SecretKey = c.expandEnv(sec.Key("secret_key").String())
	}

	// [storage.gcs]
	if sec := cfg.Section("storage.gcs"); sec != nil {
		c.GCSBucket = sec.Key("bucket").MustString("")
		c.GCSCredentialsFile = c.expandEnv(sec.Key("credentials_file").String())
	}

	// [storage.google_drive]
	if sec := cfg.Section("storage.google_drive"); sec != nil {
		c.GoogleDriveFolderID = sec.Key("folder_id").MustString("")
		c.GoogleDriveCredentials = c.expandEnv(sec.Key("credentials").String())
	}

	return nil
}

// expandEnv expands ${VAR} syntax in strings
func (c *Config) expandEnv(s string) string {
	return os.ExpandEnv(s)
}

// applyEnvOverrides applies environment variable overrides
func (c *Config) applyEnvOverrides() {
	if v := os.Getenv("PORT"); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			c.Port = i
		}
	}
	if v := os.Getenv("HOST"); v != "" {
		c.Host = v
	}
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		c.LogLevel = v
	}
	if v := os.Getenv("POSTGRES_HOST"); v != "" {
		c.PostgresHost = v
	}
	if v := os.Getenv("POSTGRES_PORT"); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			c.PostgresPort = i
		}
	}
	if v := os.Getenv("POSTGRES_DB"); v != "" {
		c.PostgresDB = v
	}
	if v := os.Getenv("POSTGRES_USER"); v != "" {
		c.PostgresUser = v
	}
	if v := os.Getenv("POSTGRES_PASSWORD"); v != "" {
		c.PostgresPassword = v
	}
	if v := os.Getenv("REDIS_ADDR"); v != "" {
		c.RedisAddr = v
	}
	if v := os.Getenv("REDIS_PASSWORD"); v != "" {
		c.RedisPassword = v
	}
	if v := os.Getenv("WORKFLOWS_DIR"); v != "" {
		c.WorkflowsDir = v
	}
	// Storage overrides
	if v := os.Getenv("STORAGE_TYPE"); v != "" {
		c.StorageType = StorageType(v)
	}
	if v := os.Getenv("STORAGE_BASE_PATH"); v != "" {
		c.StorageBasePath = v
	}
	if v := os.Getenv("S3_BUCKET"); v != "" {
		c.S3Bucket = v
	}
	if v := os.Getenv("S3_REGION"); v != "" {
		c.S3Region = v
	}
	if v := os.Getenv("S3_ACCESS_KEY"); v != "" {
		c.S3AccessKey = v
	}
	if v := os.Getenv("S3_SECRET_KEY"); v != "" {
		c.S3SecretKey = v
	}
	if v := os.Getenv("GCS_BUCKET"); v != "" {
		c.GCSBucket = v
	}
	if v := os.Getenv("GCS_CREDENTIALS_FILE"); v != "" {
		c.GCSCredentialsFile = v
	}
	if v := os.Getenv("GOOGLE_DRIVE_FOLDER_ID"); v != "" {
		c.GoogleDriveFolderID = v
	}
}

// validate checks that required configuration is present
func (c *Config) validate() error {
	if c.Port <= 0 || c.Port > 65535 {
		return fmt.Errorf("invalid port: %d", c.Port)
	}

	// Ensure at least one provider is configured
	if len(c.Providers) == 0 {
		// Add default local provider
		c.Providers["local"] = ProviderConfig{
			Name:    "local",
			Type:    "openai_compatible",
			URL:     "http://192.168.1.143:8080",
			Model:   "default",
			Enabled: true,
		}
	}

	// Ensure default roles exist
	c.ensureDefaultRoles()

	return nil
}

// ensureDefaultRoles creates default role configurations if not specified
func (c *Config) ensureDefaultRoles() {
	defaultRoles := map[RoleName]Role{
		RoleBoard: {
			Name:     RoleBoard,
			Label:    "Board of Directors",
			Provider: c.DefaultProvider,
			Persona:  "You are the Board of Directors. You set strategic direction, evaluate major decisions, and approve or reject proposals. Focus on long-term value, risk assessment, and market positioning.",
		},
		RoleCEO: {
			Name:     RoleCEO,
			Label:    "CEO",
			Provider: c.DefaultProvider,
			Persona:  "You are the CEO. You make executive decisions, prioritize initiatives, and coordinate between departments. Balance innovation with practicality.",
		},
		RoleCTO: {
			Name:     RoleCTO,
			Label:    "CTO",
			Provider: c.DefaultProvider,
			Persona:  "You are the CTO. You evaluate technical feasibility, estimate effort, identify risks, and propose architecture. Be specific about technologies and timelines.",
		},
		RoleMarketing: {
			Name:     RoleMarketing,
			Label:    "Marketing Director",
			Provider: c.DefaultProvider,
			Persona:  "You are the Marketing Director. You analyze market fit, craft positioning, write copy, and plan go-to-market strategies. Focus on audience and differentiation.",
		},
		RoleArtist: {
			Name:     RoleArtist,
			Label:    "Creative Director",
			Provider: "midjourney",
			Persona:  "You create visual assets. Translate concepts into image prompts and manage visual brand identity.",
		},
		RoleWorker: {
			Name:     RoleWorker,
			Label:    "Task Worker",
			Provider: c.DefaultProvider,
			Persona:  "You execute specific tasks as assigned. Be thorough, follow instructions precisely, and report results clearly.",
		},
	}

	for name, role := range defaultRoles {
		if _, exists := c.Roles[name]; !exists {
			c.Roles[name] = role
		}
	}
}

// GetRole returns the configuration for a role
func (c *Config) GetRole(name RoleName) (Role, bool) {
	role, ok := c.Roles[name]
	return role, ok
}

// GetProvider returns the configuration for a provider
func (c *Config) GetProvider(name string) (ProviderConfig, bool) {
	provider, ok := c.Providers[name]
	return provider, ok
}
