package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalStorageCreation(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "storage-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	storage, err := NewLocalStorage(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create local storage: %v", err)
	}

	if storage.Name() != "local" {
		t.Errorf("Expected name 'local', got %s", storage.Name())
	}

	if storage.Type() != StorageLocal {
		t.Errorf("Expected type StorageLocal, got %v", storage.Type())
	}

	if !storage.IsAvailable() {
		t.Error("Expected storage to be available")
	}
}

func TestLocalStoragePutGet(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "storage-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	storage, err := NewLocalStorage(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create local storage: %v", err)
	}

	ctx := context.Background()
	key := "test/file.txt"
	content := "Hello, World!"

	// Put
	err = storage.Put(ctx, key, bytes.NewReader([]byte(content)), "text/plain")
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Exists
	exists, err := storage.Exists(ctx, key)
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if !exists {
		t.Error("Expected file to exist after Put")
	}

	// Get
	reader, err := storage.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}

	if string(data) != content {
		t.Errorf("Expected content '%s', got '%s'", content, string(data))
	}
}

func TestLocalStorageDelete(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "storage-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	storage, err := NewLocalStorage(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create local storage: %v", err)
	}

	ctx := context.Background()
	key := "to-delete.txt"

	// Put
	err = storage.Put(ctx, key, bytes.NewReader([]byte("delete me")), "text/plain")
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Delete
	err = storage.Delete(ctx, key)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify deleted
	exists, err := storage.Exists(ctx, key)
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if exists {
		t.Error("Expected file to not exist after Delete")
	}

	// Delete non-existent should not error
	err = storage.Delete(ctx, "nonexistent.txt")
	if err != nil {
		t.Errorf("Delete of non-existent file should not error: %v", err)
	}
}

func TestLocalStorageList(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "storage-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	storage, err := NewLocalStorage(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create local storage: %v", err)
	}

	ctx := context.Background()

	// Create some files
	files := []string{
		"prefix/file1.txt",
		"prefix/file2.txt",
		"prefix/subdir/file3.txt",
		"other/file4.txt",
	}

	for _, f := range files {
		err = storage.Put(ctx, f, bytes.NewReader([]byte("content")), "text/plain")
		if err != nil {
			t.Fatalf("Put failed for %s: %v", f, err)
		}
	}

	// List with prefix
	objects, err := storage.List(ctx, "prefix")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(objects) != 3 {
		t.Errorf("Expected 3 objects with prefix 'prefix', got %d", len(objects))
	}

	// List all
	objects, err = storage.List(ctx, "")
	if err != nil {
		t.Fatalf("List all failed: %v", err)
	}

	if len(objects) != 4 {
		t.Errorf("Expected 4 objects total, got %d", len(objects))
	}
}

func TestLocalStorageGetURL(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "storage-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	storage, err := NewLocalStorage(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create local storage: %v", err)
	}

	ctx := context.Background()
	key := "test.txt"

	url, err := storage.GetURL(ctx, key, 0)
	if err != nil {
		t.Fatalf("GetURL failed: %v", err)
	}

	expectedPath := filepath.Join(tmpDir, key)
	expectedURL := "file://" + expectedPath

	if url != expectedURL {
		t.Errorf("Expected URL '%s', got '%s'", expectedURL, url)
	}
}

func TestLocalStorageGetNotFound(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "storage-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	storage, err := NewLocalStorage(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create local storage: %v", err)
	}

	ctx := context.Background()
	_, err = storage.Get(ctx, "nonexistent.txt")
	if err == nil {
		t.Error("Expected error for non-existent file")
	}
}

func TestLocalStorageNestedDirectories(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "storage-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	storage, err := NewLocalStorage(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create local storage: %v", err)
	}

	ctx := context.Background()
	key := "deep/nested/path/to/file.txt"

	err = storage.Put(ctx, key, bytes.NewReader([]byte("nested")), "text/plain")
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	exists, err := storage.Exists(ctx, key)
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if !exists {
		t.Error("Expected nested file to exist")
	}
}

func TestStorageType(t *testing.T) {
	tests := []struct {
		storageType StorageType
		expected    string
	}{
		{StorageLocal, "local"},
		{StorageS3, "s3"},
		{StorageGCS, "gcs"},
		{StorageGoogleDrive, "google_drive"},
	}

	for _, tt := range tests {
		if string(tt.storageType) != tt.expected {
			t.Errorf("Expected %s, got %s", tt.expected, tt.storageType)
		}
	}
}

func TestS3StorageStub(t *testing.T) {
	storage, err := NewS3Storage(S3Config{
		Bucket: "test-bucket",
		Region: "us-east-1",
	})
	if err != nil {
		t.Fatalf("Failed to create S3 storage: %v", err)
	}

	if storage.Name() != "s3:test-bucket" {
		t.Errorf("Expected name 's3:test-bucket', got %s", storage.Name())
	}

	if storage.Type() != StorageS3 {
		t.Errorf("Expected type StorageS3, got %v", storage.Type())
	}

	// Should not be available (not implemented)
	if storage.IsAvailable() {
		t.Error("Expected S3 stub to not be available")
	}

	// Operations should return errors
	ctx := context.Background()
	err = storage.Put(ctx, "key", nil, "")
	if err == nil {
		t.Error("Expected Put to return error")
	}
}

func TestGCSStorageStub(t *testing.T) {
	storage, err := NewGCSStorage(GCSConfig{
		Bucket: "test-bucket",
	})
	if err != nil {
		t.Fatalf("Failed to create GCS storage: %v", err)
	}

	if storage.Name() != "gcs:test-bucket" {
		t.Errorf("Expected name 'gcs:test-bucket', got %s", storage.Name())
	}

	if storage.Type() != StorageGCS {
		t.Errorf("Expected type StorageGCS, got %v", storage.Type())
	}

	if storage.IsAvailable() {
		t.Error("Expected GCS stub to not be available")
	}
}

func TestGoogleDriveStorageStub(t *testing.T) {
	storage, err := NewGoogleDriveStorage(GoogleDriveConfig{
		FolderID: "test-folder",
	})
	if err != nil {
		t.Fatalf("Failed to create Google Drive storage: %v", err)
	}

	if storage.Name() != "google_drive:test-folder" {
		t.Errorf("Expected name 'google_drive:test-folder', got %s", storage.Name())
	}

	if storage.Type() != StorageGoogleDrive {
		t.Errorf("Expected type StorageGoogleDrive, got %v", storage.Type())
	}

	if storage.IsAvailable() {
		t.Error("Expected Google Drive stub to not be available")
	}
}

func TestStorageManagerCreation(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "storage-manager-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg, _ := LoadConfig("/nonexistent/config.ini")
	cfg.StorageType = StorageLocal
	cfg.StorageBasePath = tmpDir

	sm, err := NewStorageManager(cfg)
	if err != nil {
		t.Fatalf("Failed to create storage manager: %v", err)
	}

	// Primary should be local
	if sm.Primary().Type() != StorageLocal {
		t.Errorf("Expected primary to be local, got %v", sm.Primary().Type())
	}

	// Local backend should exist
	local, err := sm.Get("local")
	if err != nil {
		t.Errorf("Failed to get local backend: %v", err)
	}
	if local == nil {
		t.Error("Expected local backend to exist")
	}

	// List should return at least local
	backends := sm.List()
	if len(backends) < 1 {
		t.Error("Expected at least 1 backend")
	}
}
