package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"job-scorer/models"
)

func TestNewFileStorage(t *testing.T) {
	storage := NewFileStorage("test_output")
	if storage == nil {
		t.Errorf("NewFileStorage() returned nil")
	}
}

func TestFileStorage_SaveAndLoadJobs(t *testing.T) {
	// Create temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "storage_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	storage := NewFileStorage(tmpDir)

	// Create test jobs
	jobs := []*models.Job{
		{
			Position:  "Software Engineer",
			Company:   "Tech Corp",
			Location:  "Basel",
			CreatedAt: time.Now(),
		},
		{
			Position:  "DevOps Engineer",
			Company:   "Cloud Co",
			Location:  "Zurich",
			CreatedAt: time.Now(),
		},
	}

	// Test SaveAllJobs
	err = storage.SaveAllJobs(jobs)
	if err != nil {
		t.Errorf("SaveAllJobs() error = %v", err)
	}

	// Verify file was created
	filePath := filepath.Join(tmpDir, "allJobs.json")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Errorf("SaveAllJobs() file was not created: %s", filePath)
	}

	// Test LoadAllJobs
	loadedJobs, err := storage.LoadAllJobs()
	if err != nil {
		t.Errorf("LoadAllJobs() error = %v", err)
	}

	if len(loadedJobs) != len(jobs) {
		t.Errorf("LoadAllJobs() returned %d jobs, expected %d", len(loadedJobs), len(jobs))
	}

	// Verify job content
	if loadedJobs[0].Position != jobs[0].Position {
		t.Errorf("LoadAllJobs() first job position = %s, expected %s", loadedJobs[0].Position, jobs[0].Position)
	}
}

func TestFileStorage_SavePromisingJobs(t *testing.T) {
	// Create temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "storage_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	storage := NewFileStorage(tmpDir)

	score := 8.0
	jobs := []*models.Job{
		{
			Position:  "Software Engineer",
			Company:   "Tech Corp",
			Location:  "Basel",
			Score:     &score,
			CreatedAt: time.Now(),
		},
	}

	// Test SavePromisingJobs
	err = storage.SavePromisingJobs(jobs)
	if err != nil {
		t.Errorf("SavePromisingJobs() error = %v", err)
	}

	// Verify file was created
	filePath := filepath.Join(tmpDir, "promisingJobs.json")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Errorf("SavePromisingJobs() file was not created: %s", filePath)
	}

	// Test LoadPromisingJobs
	loadedJobs, err := storage.LoadPromisingJobs()
	if err != nil {
		t.Errorf("LoadPromisingJobs() error = %v", err)
	}

	if len(loadedJobs) != len(jobs) {
		t.Errorf("LoadPromisingJobs() returned %d jobs, expected %d", len(loadedJobs), len(jobs))
	}
}

func TestFileStorage_SaveFinalEvaluatedJobs(t *testing.T) {
	// Create temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "storage_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	storage := NewFileStorage(tmpDir)

	score := 9.0
	jobs := []*models.Job{
		{
			Position:        "Software Engineer",
			Company:         "Tech Corp",
			Location:        "Basel",
			Score:           &score,
			FinalScore:      &score,
			ShouldSendEmail: true,
			CreatedAt:       time.Now(),
		},
	}

	// Test SaveFinalEvaluatedJobs
	err = storage.SaveFinalEvaluatedJobs(jobs)
	if err != nil {
		t.Errorf("SaveFinalEvaluatedJobs() error = %v", err)
	}

	// Verify file was created
	filePath := filepath.Join(tmpDir, "finalEvaluatedJobs.json")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Errorf("SaveFinalEvaluatedJobs() file was not created: %s", filePath)
	}

	// Test LoadFinalEvaluatedJobs
	loadedJobs, err := storage.LoadFinalEvaluatedJobs()
	if err != nil {
		t.Errorf("LoadFinalEvaluatedJobs() error = %v", err)
	}

	if len(loadedJobs) != len(jobs) {
		t.Errorf("LoadFinalEvaluatedJobs() returned %d jobs, expected %d", len(loadedJobs), len(jobs))
	}
}

func TestFileStorage_LoadNonExistentFile(t *testing.T) {
	// Create temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "storage_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	storage := NewFileStorage(tmpDir)

	// Test loading non-existent file
	jobs, err := storage.LoadAllJobs()
	if err != nil {
		t.Errorf("LoadAllJobs() should not return error for non-existent file: %v", err)
	}

	if len(jobs) != 0 {
		t.Errorf("LoadAllJobs() should return empty slice for non-existent file, got %d jobs", len(jobs))
	}
}

func TestFileStorage_EnsureOutputDir(t *testing.T) {
	// Create temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "storage_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a subdirectory that doesn't exist yet
	subDir := filepath.Join(tmpDir, "new_subdir")
	storage := NewFileStorage(subDir)

	// Test EnsureOutputDir
	err = storage.EnsureOutputDir()
	if err != nil {
		t.Errorf("EnsureOutputDir() error = %v", err)
	}

	// Verify directory was created
	if _, err := os.Stat(subDir); os.IsNotExist(err) {
		t.Errorf("EnsureOutputDir() directory was not created: %s", subDir)
	}
} 