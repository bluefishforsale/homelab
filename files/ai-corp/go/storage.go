package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

// StorageType represents the type of storage backend
type StorageType string

const (
	StorageLocal       StorageType = "local"
	StorageS3          StorageType = "s3"
	StorageGCS         StorageType = "gcs"
	StorageGoogleDrive StorageType = "google_drive"
)

// StorageObject represents a stored object
type StorageObject struct {
	Key          string    `json:"key"`
	Size         int64     `json:"size"`
	ContentType  string    `json:"content_type"`
	LastModified time.Time `json:"last_modified"`
	ETag         string    `json:"etag,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// StorageBackend defines the interface for storage backends
type StorageBackend interface {
	// Name returns the backend name
	Name() string

	// Type returns the storage type
	Type() StorageType

	// Put stores data at the given key
	Put(ctx context.Context, key string, data io.Reader, contentType string) error

	// Get retrieves data from the given key
	Get(ctx context.Context, key string) (io.ReadCloser, error)

	// Delete removes the object at the given key
	Delete(ctx context.Context, key string) error

	// Exists checks if an object exists
	Exists(ctx context.Context, key string) (bool, error)

	// List lists objects with the given prefix
	List(ctx context.Context, prefix string) ([]StorageObject, error)

	// GetURL returns a URL for accessing the object (may be signed/temporary)
	GetURL(ctx context.Context, key string, expiry time.Duration) (string, error)

	// IsAvailable checks if the backend is accessible
	IsAvailable() bool
}

// StorageConfig holds storage backend configuration
type StorageConfig struct {
	Type     StorageType
	BasePath string // For local storage

	// S3 config
	S3Endpoint  string
	S3Bucket    string
	S3Region    string
	S3AccessKey string
	S3SecretKey string

	// GCS config
	GCSBucket          string
	GCSCredentialsFile string

	// Google Drive config
	GoogleDriveFolderID     string
	GoogleDriveCredentials  string
}

// StorageManager manages storage backends
type StorageManager struct {
	backends map[string]StorageBackend
	primary  StorageBackend
	config   *Config
}

// NewStorageManager creates a new storage manager
func NewStorageManager(config *Config) (*StorageManager, error) {
	sm := &StorageManager{
		backends: make(map[string]StorageBackend),
		config:   config,
	}

	// Initialize configured backends
	if err := sm.initBackends(); err != nil {
		return nil, err
	}

	return sm, nil
}

// initBackends initializes storage backends based on config
func (sm *StorageManager) initBackends() error {
	// Always initialize local storage as fallback
	localBackend, err := NewLocalStorage(sm.config.StorageBasePath)
	if err != nil {
		return fmt.Errorf("failed to initialize local storage: %w", err)
	}
	sm.backends["local"] = localBackend

	// Set primary backend
	switch sm.config.StorageType {
	case StorageS3:
		if sm.config.S3Bucket != "" {
			s3Backend, err := NewS3Storage(S3Config{
				Endpoint:  sm.config.S3Endpoint,
				Bucket:    sm.config.S3Bucket,
				Region:    sm.config.S3Region,
				AccessKey: sm.config.S3AccessKey,
				SecretKey: sm.config.S3SecretKey,
			})
			if err != nil {
				log.Warnf("Failed to initialize S3 storage: %v, falling back to local", err)
				sm.primary = localBackend
			} else {
				sm.backends["s3"] = s3Backend
				sm.primary = s3Backend
			}
		} else {
			sm.primary = localBackend
		}

	case StorageGCS:
		if sm.config.GCSBucket != "" {
			gcsBackend, err := NewGCSStorage(GCSConfig{
				Bucket:          sm.config.GCSBucket,
				CredentialsFile: sm.config.GCSCredentialsFile,
			})
			if err != nil {
				log.Warnf("Failed to initialize GCS storage: %v, falling back to local", err)
				sm.primary = localBackend
			} else {
				sm.backends["gcs"] = gcsBackend
				sm.primary = gcsBackend
			}
		} else {
			sm.primary = localBackend
		}

	case StorageGoogleDrive:
		if sm.config.GoogleDriveFolderID != "" {
			gdBackend, err := NewGoogleDriveStorage(GoogleDriveConfig{
				FolderID:        sm.config.GoogleDriveFolderID,
				CredentialsJSON: sm.config.GoogleDriveCredentials,
			})
			if err != nil {
				log.Warnf("Failed to initialize Google Drive storage: %v, falling back to local", err)
				sm.primary = localBackend
			} else {
				sm.backends["google_drive"] = gdBackend
				sm.primary = gdBackend
			}
		} else {
			sm.primary = localBackend
		}

	default:
		sm.primary = localBackend
	}

	log.Infof("Storage initialized: primary=%s, backends=%d", sm.primary.Name(), len(sm.backends))
	return nil
}

// Primary returns the primary storage backend
func (sm *StorageManager) Primary() StorageBackend {
	return sm.primary
}

// Get returns a specific backend by name
func (sm *StorageManager) Get(name string) (StorageBackend, error) {
	backend, ok := sm.backends[name]
	if !ok {
		return nil, fmt.Errorf("storage backend not found: %s", name)
	}
	return backend, nil
}

// List returns all available backends
func (sm *StorageManager) List() []StorageBackend {
	backends := make([]StorageBackend, 0, len(sm.backends))
	for _, b := range sm.backends {
		backends = append(backends, b)
	}
	return backends
}

// ============================================================================
// Local Storage Implementation
// ============================================================================

// LocalStorage implements StorageBackend for local filesystem
type LocalStorage struct {
	basePath string
}

// NewLocalStorage creates a new local storage backend
func NewLocalStorage(basePath string) (*LocalStorage, error) {
	// Ensure base path exists
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %w", err)
	}

	return &LocalStorage{basePath: basePath}, nil
}

func (s *LocalStorage) Name() string {
	return "local"
}

func (s *LocalStorage) Type() StorageType {
	return StorageLocal
}

func (s *LocalStorage) Put(ctx context.Context, key string, data io.Reader, contentType string) error {
	path := s.keyToPath(key)

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Create file
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	// Copy data
	if _, err := io.Copy(file, data); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

func (s *LocalStorage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	path := s.keyToPath(key)

	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("object not found: %s", key)
		}
		return nil, fmt.Errorf("failed to open file: %w", err)
	}

	return file, nil
}

func (s *LocalStorage) Delete(ctx context.Context, key string) error {
	path := s.keyToPath(key)

	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return nil // Already deleted
		}
		return fmt.Errorf("failed to delete file: %w", err)
	}

	return nil
}

func (s *LocalStorage) Exists(ctx context.Context, key string) (bool, error) {
	path := s.keyToPath(key)
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func (s *LocalStorage) List(ctx context.Context, prefix string) ([]StorageObject, error) {
	var objects []StorageObject

	prefixPath := s.keyToPath(prefix)
	baseLen := len(s.basePath) + 1

	err := filepath.Walk(prefixPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}
		if info.IsDir() {
			return nil
		}

		// Convert path back to key
		key := path[baseLen:]
		key = strings.ReplaceAll(key, string(os.PathSeparator), "/")

		objects = append(objects, StorageObject{
			Key:          key,
			Size:         info.Size(),
			LastModified: info.ModTime(),
		})

		return nil
	})

	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	return objects, nil
}

func (s *LocalStorage) GetURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	// For local storage, return a file:// URL
	path := s.keyToPath(key)
	return "file://" + path, nil
}

func (s *LocalStorage) IsAvailable() bool {
	// Check if base path is accessible
	_, err := os.Stat(s.basePath)
	return err == nil
}

func (s *LocalStorage) keyToPath(key string) string {
	// Sanitize key and convert to filesystem path
	key = strings.TrimPrefix(key, "/")
	return filepath.Join(s.basePath, filepath.FromSlash(key))
}

// ============================================================================
// S3 Storage Implementation (Stub - requires AWS SDK)
// ============================================================================

type S3Config struct {
	Endpoint  string
	Bucket    string
	Region    string
	AccessKey string
	SecretKey string
}

type S3Storage struct {
	config S3Config
}

func NewS3Storage(config S3Config) (*S3Storage, error) {
	// TODO: Initialize AWS S3 client
	// For now, return a stub that can be implemented later
	return &S3Storage{config: config}, nil
}

func (s *S3Storage) Name() string {
	return "s3:" + s.config.Bucket
}

func (s *S3Storage) Type() StorageType {
	return StorageS3
}

func (s *S3Storage) Put(ctx context.Context, key string, data io.Reader, contentType string) error {
	// TODO: Implement S3 put
	return fmt.Errorf("S3 storage not implemented - add AWS SDK dependency")
}

func (s *S3Storage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	return nil, fmt.Errorf("S3 storage not implemented")
}

func (s *S3Storage) Delete(ctx context.Context, key string) error {
	return fmt.Errorf("S3 storage not implemented")
}

func (s *S3Storage) Exists(ctx context.Context, key string) (bool, error) {
	return false, fmt.Errorf("S3 storage not implemented")
}

func (s *S3Storage) List(ctx context.Context, prefix string) ([]StorageObject, error) {
	return nil, fmt.Errorf("S3 storage not implemented")
}

func (s *S3Storage) GetURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	return "", fmt.Errorf("S3 storage not implemented")
}

func (s *S3Storage) IsAvailable() bool {
	return false // Not implemented yet
}

// ============================================================================
// GCS Storage Implementation (Stub - requires GCS SDK)
// ============================================================================

type GCSConfig struct {
	Bucket          string
	CredentialsFile string
}

type GCSStorage struct {
	config GCSConfig
}

func NewGCSStorage(config GCSConfig) (*GCSStorage, error) {
	// TODO: Initialize GCS client
	return &GCSStorage{config: config}, nil
}

func (s *GCSStorage) Name() string {
	return "gcs:" + s.config.Bucket
}

func (s *GCSStorage) Type() StorageType {
	return StorageGCS
}

func (s *GCSStorage) Put(ctx context.Context, key string, data io.Reader, contentType string) error {
	return fmt.Errorf("GCS storage not implemented - add GCS SDK dependency")
}

func (s *GCSStorage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	return nil, fmt.Errorf("GCS storage not implemented")
}

func (s *GCSStorage) Delete(ctx context.Context, key string) error {
	return fmt.Errorf("GCS storage not implemented")
}

func (s *GCSStorage) Exists(ctx context.Context, key string) (bool, error) {
	return false, fmt.Errorf("GCS storage not implemented")
}

func (s *GCSStorage) List(ctx context.Context, prefix string) ([]StorageObject, error) {
	return nil, fmt.Errorf("GCS storage not implemented")
}

func (s *GCSStorage) GetURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	return "", fmt.Errorf("GCS storage not implemented")
}

func (s *GCSStorage) IsAvailable() bool {
	return false
}

// ============================================================================
// Google Drive Storage Implementation (Stub - requires Google Drive API)
// ============================================================================

type GoogleDriveConfig struct {
	FolderID        string
	CredentialsJSON string
}

type GoogleDriveStorage struct {
	config GoogleDriveConfig
}

func NewGoogleDriveStorage(config GoogleDriveConfig) (*GoogleDriveStorage, error) {
	// TODO: Initialize Google Drive client
	return &GoogleDriveStorage{config: config}, nil
}

func (s *GoogleDriveStorage) Name() string {
	return "google_drive:" + s.config.FolderID
}

func (s *GoogleDriveStorage) Type() StorageType {
	return StorageGoogleDrive
}

func (s *GoogleDriveStorage) Put(ctx context.Context, key string, data io.Reader, contentType string) error {
	return fmt.Errorf("Google Drive storage not implemented - add Google Drive API dependency")
}

func (s *GoogleDriveStorage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	return nil, fmt.Errorf("Google Drive storage not implemented")
}

func (s *GoogleDriveStorage) Delete(ctx context.Context, key string) error {
	return fmt.Errorf("Google Drive storage not implemented")
}

func (s *GoogleDriveStorage) Exists(ctx context.Context, key string) (bool, error) {
	return false, fmt.Errorf("Google Drive storage not implemented")
}

func (s *GoogleDriveStorage) List(ctx context.Context, prefix string) ([]StorageObject, error) {
	return nil, fmt.Errorf("Google Drive storage not implemented")
}

func (s *GoogleDriveStorage) GetURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	return "", fmt.Errorf("Google Drive storage not implemented")
}

func (s *GoogleDriveStorage) IsAvailable() bool {
	return false
}
