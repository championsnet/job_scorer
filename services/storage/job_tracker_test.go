package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"job-scorer/models"
)

func TestNewJobTracker(t *testing.T) {
	// Create temporary directory
	tempDir := t.TempDir()
	
	tracker, err := NewJobTracker(tempDir)
	if err != nil {
		t.Fatalf("NewJobTracker() error = %v", err)
	}
	
	if tracker == nil {
		t.Fatal("NewJobTracker() returned nil")
	}
	
	// Check that the file path is set correctly
	expectedPath := filepath.Join(tempDir, "processed_job_ids.json")
	if tracker.filePath != expectedPath {
		t.Errorf("Expected file path %s, got %s", expectedPath, tracker.filePath)
	}
}

func TestJobTracker_IsProcessed(t *testing.T) {
	tempDir := t.TempDir()
	tracker, err := NewJobTracker(tempDir)
	if err != nil {
		t.Fatalf("NewJobTracker() error = %v", err)
	}
	
	// Test empty job ID
	if tracker.IsProcessed("") {
		t.Error("Empty job ID should not be considered processed")
	}
	
	// Test non-existent job ID
	if tracker.IsProcessed("nonexistent") {
		t.Error("Non-existent job ID should not be considered processed")
	}
	
	// Mark a job as processed
	err = tracker.MarkAsProcessed("test123")
	if err != nil {
		t.Fatalf("MarkAsProcessed() error = %v", err)
	}
	
	// Test that it's now considered processed
	if !tracker.IsProcessed("test123") {
		t.Error("Job ID should be considered processed after marking")
	}
}

func TestJobTracker_MarkAsProcessed(t *testing.T) {
	tempDir := t.TempDir()
	tracker, err := NewJobTracker(tempDir)
	if err != nil {
		t.Fatalf("NewJobTracker() error = %v", err)
	}
	
	// Test marking empty job ID
	err = tracker.MarkAsProcessed("")
	if err == nil {
		t.Error("MarkAsProcessed() should return error for empty job ID")
	}
	
	// Test marking valid job ID
	err = tracker.MarkAsProcessed("valid123")
	if err != nil {
		t.Errorf("MarkAsProcessed() error = %v", err)
	}
	
	// Verify it's marked as processed
	if !tracker.IsProcessed("valid123") {
		t.Error("Job ID should be marked as processed")
	}
}

func TestJobTracker_MarkMultipleAsProcessed(t *testing.T) {
	tempDir := t.TempDir()
	tracker, err := NewJobTracker(tempDir)
	if err != nil {
		t.Fatalf("NewJobTracker() error = %v", err)
	}
	
	// Test empty slice
	err = tracker.MarkMultipleAsProcessed([]string{})
	if err != nil {
		t.Errorf("MarkMultipleAsProcessed() with empty slice error = %v", err)
	}
	
	// Test with valid job IDs
	jobIDs := []string{"job1", "job2", "job3", ""} // Empty ID should be ignored
	err = tracker.MarkMultipleAsProcessed(jobIDs)
	if err != nil {
		t.Errorf("MarkMultipleAsProcessed() error = %v", err)
	}
	
	// Verify all valid job IDs are marked as processed
	for _, jobID := range jobIDs {
		if jobID != "" && !tracker.IsProcessed(jobID) {
			t.Errorf("Job ID %s should be marked as processed", jobID)
		}
	}
}

func TestJobTracker_FilterProcessedJobs(t *testing.T) {
	tempDir := t.TempDir()
	tracker, err := NewJobTracker(tempDir)
	if err != nil {
		t.Fatalf("NewJobTracker() error = %v", err)
	}
	
	// Create test jobs
	jobs := []*models.Job{
		{JobID: "job1", Position: "Engineer", Company: "Tech Corp"},
		{JobID: "job2", Position: "Manager", Company: "Biz Corp"},
		{JobID: "", Position: "No ID", Company: "No ID Corp"}, // Empty ID
		{JobID: "job3", Position: "Designer", Company: "Design Corp"},
	}
	
	// Mark some jobs as processed
	err = tracker.MarkMultipleAsProcessed([]string{"job1", "job3"})
	if err != nil {
		t.Fatalf("MarkMultipleAsProcessed() error = %v", err)
	}
	
	// Filter processed jobs
	filtered := tracker.FilterProcessedJobs(jobs)
	
	// Should have 2 jobs remaining (job2 and the one with empty ID)
	expectedCount := 2
	if len(filtered) != expectedCount {
		t.Errorf("Expected %d jobs after filtering, got %d", expectedCount, len(filtered))
	}
	
	// Check that the right jobs remain
	expectedJobIDs := map[string]bool{"job2": true, "": true}
	for _, job := range filtered {
		if !expectedJobIDs[job.JobID] {
			t.Errorf("Unexpected job ID in filtered results: %s", job.JobID)
		}
	}
}

func TestJobTracker_GetStats(t *testing.T) {
	tempDir := t.TempDir()
	tracker, err := NewJobTracker(tempDir)
	if err != nil {
		t.Fatalf("NewJobTracker() error = %v", err)
	}
	
	// Add some test jobs
	jobIDs := []string{"job1", "job2", "job3"}
	err = tracker.MarkMultipleAsProcessed(jobIDs)
	if err != nil {
		t.Fatalf("MarkMultipleAsProcessed() error = %v", err)
	}
	
	// Get stats
	stats := tracker.GetStats()
	
	// Check that stats contain expected fields
	if stats["total_processed_jobs"] != 3 {
		t.Errorf("Expected 3 total processed jobs, got %v", stats["total_processed_jobs"])
	}
	
	if stats["tracking_file"] == "" {
		t.Error("Tracking file path should not be empty")
	}
	
	// Check that recent jobs count is reasonable (should be 3 since we just added them)
	recentJobs := stats["recent_jobs_7_days"]
	if recentJobs != 3 {
		t.Errorf("Expected 3 recent jobs, got %v", recentJobs)
	}
}

func TestJobTracker_CleanupOldEntries(t *testing.T) {
	tempDir := t.TempDir()
	tracker, err := NewJobTracker(tempDir)
	if err != nil {
		t.Fatalf("NewJobTracker() error = %v", err)
	}
	
	// Add some test jobs
	jobIDs := []string{"job1", "job2", "job3"}
	err = tracker.MarkMultipleAsProcessed(jobIDs)
	if err != nil {
		t.Fatalf("MarkMultipleAsProcessed() error = %v", err)
	}
	
	// Verify all jobs are processed
	for _, jobID := range jobIDs {
		if !tracker.IsProcessed(jobID) {
			t.Errorf("Job ID %s should be marked as processed", jobID)
		}
	}
	
	// Clean up entries older than 1 second (should remove all since they were just added)
	err = tracker.CleanupOldEntries(time.Second)
	if err != nil {
		t.Errorf("CleanupOldEntries() error = %v", err)
	}
	
	// Verify all jobs are still processed (they shouldn't be cleaned up since they're recent)
	for _, jobID := range jobIDs {
		if !tracker.IsProcessed(jobID) {
			t.Errorf("Job ID %s should still be marked as processed", jobID)
		}
	}
	
	// Clean up entries older than 0 seconds (should remove all)
	err = tracker.CleanupOldEntries(0)
	if err != nil {
		t.Errorf("CleanupOldEntries() error = %v", err)
	}
	
	// Verify all jobs are now removed
	for _, jobID := range jobIDs {
		if tracker.IsProcessed(jobID) {
			t.Errorf("Job ID %s should have been cleaned up", jobID)
		}
	}
}

func TestJobTracker_Persistence(t *testing.T) {
	tempDir := t.TempDir()
	
	// Create first tracker and add some jobs
	tracker1, err := NewJobTracker(tempDir)
	if err != nil {
		t.Fatalf("NewJobTracker() error = %v", err)
	}
	
	jobIDs := []string{"persistent1", "persistent2"}
	err = tracker1.MarkMultipleAsProcessed(jobIDs)
	if err != nil {
		t.Fatalf("MarkMultipleAsProcessed() error = %v", err)
	}
	
	// Create second tracker (should load the same file)
	tracker2, err := NewJobTracker(tempDir)
	if err != nil {
		t.Fatalf("NewJobTracker() error = %v", err)
	}
	
	// Verify that the second tracker sees the jobs as processed
	for _, jobID := range jobIDs {
		if !tracker2.IsProcessed(jobID) {
			t.Errorf("Job ID %s should be marked as processed in second tracker", jobID)
		}
	}
}

func TestJobTracker_FileCreation(t *testing.T) {
	tempDir := t.TempDir()
	
	// Verify file doesn't exist initially
	filePath := filepath.Join(tempDir, "processed_job_ids.json")
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Fatal("File should not exist initially")
	}
	
	// Create tracker
	tracker, err := NewJobTracker(tempDir)
	if err != nil {
		t.Fatalf("NewJobTracker() error = %v", err)
	}
	
	// Add a job (this should create the file)
	err = tracker.MarkAsProcessed("test123")
	if err != nil {
		t.Fatalf("MarkAsProcessed() error = %v", err)
	}
	
	// Verify file now exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Fatal("File should exist after marking job as processed")
	}
} 