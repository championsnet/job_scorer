package storage

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"job-scorer/models"
	"job-scorer/utils"
)

// GCSCheckpointService manages checkpoints using GCS storage
type GCSCheckpointService struct {
	gcsStorage *GCSStorage
	logger     *utils.Logger
	runFolder  string
}

// NewGCSCheckpointService creates a new GCS checkpoint service
func NewGCSCheckpointService(gcsStorage *GCSStorage) (*GCSCheckpointService, error) {
	logger := utils.NewLogger("GCSCheckpointService")

	return &GCSCheckpointService{
		gcsStorage: gcsStorage,
		logger:     logger,
	}, nil
}

// SetRunFolder sets the folder name for the current run/session
func (cs *GCSCheckpointService) SetRunFolder(folder string) {
	cs.runFolder = folder
	cs.logger.Info("Set run folder to: %s", folder)
}

// GetRunFolder returns the current run/session folder name, or generates one if not set
func (cs *GCSCheckpointService) GetRunFolder() string {
	if cs.runFolder != "" {
		return cs.runFolder
	}
	// Default: generate a new one
	now := time.Now()
	return now.Format("2006-01-02_15-04-05")
}

// SaveCheckpoint saves jobs with timestamp and metadata to GCS
func (cs *GCSCheckpointService) SaveCheckpoint(jobs []*models.Job, stage string, metadata map[string]interface{}) error {
	if err := cs.gcsStorage.SaveCheckpoint(jobs, stage, cs.GetRunFolder(), metadata); err != nil {
		return fmt.Errorf("failed to save checkpoint to GCS: %w", err)
	}

	cs.logger.Info("Saved checkpoint: %s/%s with %d jobs", cs.GetRunFolder(), stage, len(jobs))
	return nil
}

// SaveDailySnapshot saves a daily summary of all job stages to GCS
func (cs *GCSCheckpointService) SaveDailySnapshot(allJobs, promisingJobs, finalJobs []*models.Job) error {
	runFolder := cs.GetRunFolder()

	snapshot := struct {
		Date         string         `json:"date"`
		AllJobs      []*models.Job  `json:"all_jobs"`
		PromisingJobs []*models.Job `json:"promising_jobs"`
		FinalJobs    []*models.Job  `json:"final_jobs"`
		Summary      struct {
			TotalJobs      int `json:"total_jobs"`
			PromisingCount int `json:"promising_count"`
			FinalCount     int `json:"final_count"`
			EmailCount     int `json:"email_count"`
		} `json:"summary"`
	}{
		Date:         runFolder,
		AllJobs:      allJobs,
		PromisingJobs: promisingJobs,
		FinalJobs:    finalJobs,
	}

	// Calculate summary
	snapshot.Summary.TotalJobs = len(allJobs)
	snapshot.Summary.PromisingCount = len(promisingJobs)
	snapshot.Summary.FinalCount = len(finalJobs)

	emailCount := 0
	for _, job := range finalJobs {
		if job.FinalScore != nil {
			emailCount++
		}
	}
	snapshot.Summary.EmailCount = emailCount

	// Marshal to JSON
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal daily snapshot: %w", err)
	}

	// Save to GCS
	gcsPath := fmt.Sprintf("checkpoints/%s/daily_snapshot.json", runFolder)
	reader := strings.NewReader(string(data))
	if err := cs.gcsStorage.UploadData(reader, gcsPath); err != nil {
		return fmt.Errorf("failed to save daily snapshot to GCS: %w", err)
	}

	cs.logger.Info("Saved daily snapshot to GCS: %s (Total: %d, Promising: %d, Final: %d, Email: %d)",
		gcsPath, snapshot.Summary.TotalJobs, snapshot.Summary.PromisingCount,
		snapshot.Summary.FinalCount, snapshot.Summary.EmailCount)
	return nil
}

// ListCheckpoints returns all available checkpoints organized by date from GCS
func (cs *GCSCheckpointService) ListCheckpoints() (map[string][]string, error) {
	checkpoints := make(map[string][]string)
	
	// List all files in checkpoints/ directory
	files, err := cs.gcsStorage.ListFiles("checkpoints/")
	if err != nil {
		return nil, fmt.Errorf("failed to list checkpoint files from GCS: %w", err)
	}

	for _, file := range files {
		// Parse the file path: checkpoints/YYYY-MM-DD_HH-MM-SS/filename.json
		parts := strings.Split(file, "/")
		if len(parts) >= 3 && parts[0] == "checkpoints" {
			dateFolder := parts[1]
			filename := parts[2]
			
			if strings.HasSuffix(filename, ".json") {
				checkpoints[dateFolder] = append(checkpoints[dateFolder], filename)
			}
		}
	}

	cs.logger.Info("Listed checkpoints from GCS: %d date folders found", len(checkpoints))
	return checkpoints, nil
}

// LoadCheckpoint loads a specific checkpoint file from GCS
func (cs *GCSCheckpointService) LoadCheckpoint(dateFolder, filename string) ([]*models.Job, error) {
	gcsPath := fmt.Sprintf("checkpoints/%s/%s", dateFolder, filename)

	data, err := cs.gcsStorage.DownloadData(gcsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load checkpoint from GCS: %w", err)
	}

	var checkpoint struct {
		Jobs []*models.Job `json:"jobs"`
	}

	if err := json.Unmarshal(data, &checkpoint); err != nil {
		return nil, fmt.Errorf("failed to unmarshal checkpoint data: %w", err)
	}

	cs.logger.Info("Loaded checkpoint from GCS: %s with %d jobs", gcsPath, len(checkpoint.Jobs))
	return checkpoint.Jobs, nil
}

// LoadCheckpointByStage finds and loads the most recent checkpoint for a stage in a run folder
func (cs *GCSCheckpointService) LoadCheckpointByStage(runID, stage string) ([]*models.Job, error) {
	if runID == "" || stage == "" {
		return nil, fmt.Errorf("run_id and stage are required")
	}
	prefix := fmt.Sprintf("checkpoints/%s/", runID)
	files, err := cs.gcsStorage.ListFiles(prefix)
	if err != nil {
		return nil, fmt.Errorf("failed to list checkpoints from GCS: %w", err)
	}
	stagePrefix := "checkpoint_" + stage + "_"
	var bestPath string
	var bestFilename string
	for _, f := range files {
		pathParts := strings.Split(f, "/")
		if len(pathParts) < 3 {
			continue
		}
		filename := pathParts[len(pathParts)-1]
		if !strings.HasSuffix(filename, ".json") || len(filename) < len(stagePrefix) || filename[:len(stagePrefix)] != stagePrefix {
			continue
		}
		if bestPath == "" || filename > bestFilename {
			bestPath = f
			bestFilename = filename
		}
	}
	if bestPath == "" {
		return nil, nil
	}
	return cs.LoadCheckpoint(runID, bestFilename)
}

// LoadCheckpointLegacy loads a legacy checkpoint file (compatibility method)
func (cs *GCSCheckpointService) LoadCheckpointLegacy(filename string) ([]*models.Job, error) {
	// For GCS, we'll look in the checkpoints root directory
	gcsPath := fmt.Sprintf("checkpoints/%s", filename)
	
	data, err := cs.gcsStorage.DownloadData(gcsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load legacy checkpoint from GCS: %w", err)
	}

	var checkpoint struct {
		Jobs []*models.Job `json:"jobs"`
	}
	
	if err := json.Unmarshal(data, &checkpoint); err != nil {
		return nil, fmt.Errorf("failed to unmarshal legacy checkpoint data: %w", err)
	}

	cs.logger.Info("Loaded legacy checkpoint from GCS: %s with %d jobs", gcsPath, len(checkpoint.Jobs))
	return checkpoint.Jobs, nil
}

// CleanupOldCheckpoints removes checkpoints older than specified days from GCS
func (cs *GCSCheckpointService) CleanupOldCheckpoints(daysToKeep int) error {
	files, err := cs.gcsStorage.ListFiles("checkpoints/")
	if err != nil {
		return fmt.Errorf("failed to list checkpoint files for cleanup: %w", err)
	}

	cutoffTime := time.Now().AddDate(0, 0, -daysToKeep)
	deletedCount := 0

	for _, file := range files {
		// Parse date from checkpoint path
		parts := strings.Split(file, "/")
		if len(parts) >= 2 && parts[0] == "checkpoints" {
			dateFolder := parts[1]
			
			// Try to parse the date folder name
			if folderTime, err := time.Parse("2006-01-02_15-04-05", dateFolder); err == nil {
				if folderTime.Before(cutoffTime) {
					if err := cs.gcsStorage.DeleteFile(file); err != nil {
						cs.logger.Error("Failed to delete old checkpoint %s: %v", file, err)
					} else {
						deletedCount++
					}
				}
			}
		}
	}

	cs.logger.Info("Cleaned up %d old checkpoint files from GCS (older than %d days)", deletedCount, daysToKeep)
	return nil
} 