package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"job-scorer/models"
)

func TestLocalRunSummaryStore_SaveLoad(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "run_summary_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewLocalRunSummaryStore(tmpDir)
	if err != nil {
		t.Fatalf("NewLocalRunSummaryStore() error = %v", err)
	}

	summary := &models.RunSummary{
		RunID:      "2024-01-15_12-30-00",
		Status:     models.RunStatusSuccess,
		StartedAt:  time.Now().Add(-5 * time.Minute),
		DurationMs: 120000,
		StageCounts: models.RunStageCounts{
			AllJobs:        100,
			Prefiltered:    80,
			Evaluated:      60,
			Promising:      10,
			FinalEvaluated: 5,
			EmailSent:      3,
		},
		Config: models.RunConfigSnapshot{
			Locations: []string{"10000000"},
			MaxJobs:   100,
		},
	}

	if err := store.SaveRunSummary(summary); err != nil {
		t.Fatalf("SaveRunSummary() error = %v", err)
	}

	loaded, err := store.LoadRunSummary(summary.RunID)
	if err != nil {
		t.Fatalf("LoadRunSummary() error = %v", err)
	}
	if loaded == nil {
		t.Fatal("LoadRunSummary() returned nil")
	}
	if loaded.RunID != summary.RunID {
		t.Errorf("RunID = %s, want %s", loaded.RunID, summary.RunID)
	}
	if loaded.Status != summary.Status {
		t.Errorf("Status = %s, want %s", loaded.Status, summary.Status)
	}
	if loaded.StageCounts.AllJobs != summary.StageCounts.AllJobs {
		t.Errorf("AllJobs = %d, want %d", loaded.StageCounts.AllJobs, summary.StageCounts.AllJobs)
	}
}

func TestLocalRunSummaryStore_ListRuns(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "run_summary_list_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewLocalRunSummaryStore(tmpDir)
	if err != nil {
		t.Fatalf("NewLocalRunSummaryStore() error = %v", err)
	}

	// Create run folders with run_summary.json
	for _, runID := range []string{"2024-01-15_10-00-00", "2024-01-15_11-00-00", "2024-01-15_12-00-00"} {
		runDir := filepath.Join(tmpDir, runID)
		if err := os.MkdirAll(runDir, 0755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		s := &models.RunSummary{RunID: runID, Status: models.RunStatusSuccess, StartedAt: time.Now()}
		if err := store.SaveRunSummary(s); err != nil {
			t.Fatalf("SaveRunSummary: %v", err)
		}
	}

	list, err := store.ListRunSummaries(2)
	if err != nil {
		t.Fatalf("ListRunSummaries() error = %v", err)
	}
	if len(list) != 2 {
		t.Errorf("ListRunSummaries(2) returned %d, want 2", len(list))
	}
	// Should be sorted descending (newest first)
	if len(list) >= 2 && list[0].RunID < list[1].RunID {
		t.Errorf("List not sorted descending: %s before %s", list[0].RunID, list[1].RunID)
	}
}

func TestLocalRunSummaryStore_LoadMissing(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "run_summary_missing_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewLocalRunSummaryStore(tmpDir)
	if err != nil {
		t.Fatalf("NewLocalRunSummaryStore() error = %v", err)
	}

	loaded, err := store.LoadRunSummary("nonexistent")
	if err != nil {
		t.Fatalf("LoadRunSummary(nonexistent) error = %v", err)
	}
	if loaded != nil {
		t.Errorf("LoadRunSummary(nonexistent) = %v, want nil", loaded)
	}
}
