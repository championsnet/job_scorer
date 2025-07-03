package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"job-scorer/models"
	"job-scorer/utils"
)

type CheckpointService struct {
	checkpointDir string
	logger        *utils.Logger
}

func NewCheckpointService(checkpointDir string) (*CheckpointService, error) {
	// Create checkpoint directory if it doesn't exist
	if err := os.MkdirAll(checkpointDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create checkpoint directory: %w", err)
	}

	logger, err := utils.NewFileLogger("CheckpointService", checkpointDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create checkpoint logger: %w", err)
	}

	return &CheckpointService{
		checkpointDir: checkpointDir,
		logger:        logger,
	}, nil
}

// SaveCheckpoint saves jobs with timestamp and metadata in organized folders
func (cs *CheckpointService) SaveCheckpoint(jobs []*models.Job, stage string, metadata map[string]interface{}) error {
	now := time.Now()
	dateFolder := now.Format("2006-01-02")
	timestamp := now.Format("15-04-05")
	
	// Create date-based folder structure
	dateDir := filepath.Join(cs.checkpointDir, dateFolder)
	if err := os.MkdirAll(dateDir, 0755); err != nil {
		return fmt.Errorf("failed to create date directory %s: %w", dateDir, err)
	}
	
	// Create checkpoint data
	checkpoint := struct {
		Timestamp time.Time              `json:"timestamp"`
		Stage     string                 `json:"stage"`
		Metadata  map[string]interface{} `json:"metadata"`
		JobCount  int                    `json:"job_count"`
		Jobs      []*models.Job          `json:"jobs"`
	}{
		Timestamp: now,
		Stage:     stage,
		Metadata:  metadata,
		JobCount:  len(jobs),
		Jobs:      jobs,
	}

	// Create filename with timestamp
	filename := fmt.Sprintf("checkpoint_%s_%s.json", stage, timestamp)
	filepath := filepath.Join(dateDir, filename)

	// Marshal with indentation
	data, err := json.MarshalIndent(checkpoint, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal checkpoint data: %w", err)
	}

	// Write to file
	if err := os.WriteFile(filepath, data, 0644); err != nil {
		return fmt.Errorf("failed to write checkpoint file: %w", err)
	}

	cs.logger.Info("Saved checkpoint: %s/%s with %d jobs", dateFolder, filename, len(jobs))
	return nil
}

// SaveDailySnapshot saves a daily summary of all job stages in the date folder
func (cs *CheckpointService) SaveDailySnapshot(allJobs, promisingJobs, finalJobs []*models.Job) error {
	now := time.Now()
	dateFolder := now.Format("2006-01-02")
	
	// Create date-based folder structure
	dateDir := filepath.Join(cs.checkpointDir, dateFolder)
	if err := os.MkdirAll(dateDir, 0755); err != nil {
		return fmt.Errorf("failed to create date directory %s: %w", dateDir, err)
	}
	
	snapshot := struct {
		Date         string       `json:"date"`
		AllJobs      []*models.Job `json:"all_jobs"`
		PromisingJobs []*models.Job `json:"promising_jobs"`
		FinalJobs    []*models.Job `json:"final_jobs"`
		Summary      struct {
			TotalJobs      int `json:"total_jobs"`
			PromisingCount int `json:"promising_count"`
			FinalCount     int `json:"final_count"`
			EmailCount     int `json:"email_count"`
		} `json:"summary"`
	}{
		Date:         dateFolder,
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
		if job.ShouldSendEmail {
			emailCount++
		}
	}
	snapshot.Summary.EmailCount = emailCount

	// Create filename
	filename := "daily_snapshot.json"
	filepath := filepath.Join(dateDir, filename)

	// Marshal with indentation
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal daily snapshot: %w", err)
	}

	// Write to file
	if err := os.WriteFile(filepath, data, 0644); err != nil {
		return fmt.Errorf("failed to write daily snapshot: %w", err)
	}

	cs.logger.Info("Saved daily snapshot: %s/%s (Total: %d, Promising: %d, Final: %d, Email: %d)", 
		dateFolder, filename, snapshot.Summary.TotalJobs, snapshot.Summary.PromisingCount, 
		snapshot.Summary.FinalCount, snapshot.Summary.EmailCount)
	return nil
}

// ListCheckpoints returns all available checkpoints organized by date
func (cs *CheckpointService) ListCheckpoints() (map[string][]string, error) {
	checkpoints := make(map[string][]string)
	
	// Read the main checkpoint directory
	files, err := os.ReadDir(cs.checkpointDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read checkpoint directory: %w", err)
	}

	for _, file := range files {
		if file.IsDir() {
			// This is a date folder, read its contents
			dateDir := filepath.Join(cs.checkpointDir, file.Name())
			dateFiles, err := os.ReadDir(dateDir)
			if err != nil {
				cs.logger.Error("Failed to read date directory %s: %v", file.Name(), err)
				continue
			}
			
			var dateCheckpoints []string
			for _, dateFile := range dateFiles {
				if !dateFile.IsDir() && filepath.Ext(dateFile.Name()) == ".json" {
					dateCheckpoints = append(dateCheckpoints, dateFile.Name())
				}
			}
			
			if len(dateCheckpoints) > 0 {
				checkpoints[file.Name()] = dateCheckpoints
			}
		} else if filepath.Ext(file.Name()) == ".json" {
			// Legacy files in root directory
			checkpoints["legacy"] = append(checkpoints["legacy"], file.Name())
		}
	}

	return checkpoints, nil
}

// LoadCheckpoint loads a specific checkpoint file from a date folder
func (cs *CheckpointService) LoadCheckpoint(dateFolder, filename string) ([]*models.Job, error) {
	filepath := filepath.Join(cs.checkpointDir, dateFolder, filename)
	
	data, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to read checkpoint file: %w", err)
	}

	var checkpoint struct {
		Jobs []*models.Job `json:"jobs"`
	}
	
	if err := json.Unmarshal(data, &checkpoint); err != nil {
		return nil, fmt.Errorf("failed to unmarshal checkpoint data: %w", err)
	}

	cs.logger.Info("Loaded checkpoint: %s/%s with %d jobs", dateFolder, filename, len(checkpoint.Jobs))
	return checkpoint.Jobs, nil
}

// LoadCheckpointLegacy loads a legacy checkpoint file from root directory
func (cs *CheckpointService) LoadCheckpointLegacy(filename string) ([]*models.Job, error) {
	filepath := filepath.Join(cs.checkpointDir, filename)
	
	data, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to read checkpoint file: %w", err)
	}

	var checkpoint struct {
		Jobs []*models.Job `json:"jobs"`
	}
	
	if err := json.Unmarshal(data, &checkpoint); err != nil {
		return nil, fmt.Errorf("failed to unmarshal checkpoint data: %w", err)
	}

	cs.logger.Info("Loaded legacy checkpoint: %s with %d jobs", filename, len(checkpoint.Jobs))
	return checkpoint.Jobs, nil
}

// CleanupOldCheckpoints removes checkpoints older than specified days
func (cs *CheckpointService) CleanupOldCheckpoints(daysToKeep int) error {
	files, err := os.ReadDir(cs.checkpointDir)
	if err != nil {
		return fmt.Errorf("failed to read checkpoint directory: %w", err)
	}

	cutoffTime := time.Now().AddDate(0, 0, -daysToKeep)
	deletedCount := 0

	for _, file := range files {
		if !file.IsDir() {
			// Handle legacy files in root directory
			if filepath.Ext(file.Name()) == ".json" {
				fileInfo, err := file.Info()
				if err != nil {
					continue
				}

				if fileInfo.ModTime().Before(cutoffTime) {
					filepath := filepath.Join(cs.checkpointDir, file.Name())
					if err := os.Remove(filepath); err != nil {
						cs.logger.Error("Failed to delete old legacy checkpoint %s: %v", file.Name(), err)
					} else {
						deletedCount++
					}
				}
			}
			continue
		}

		// Handle date folders
		fileInfo, err := file.Info()
		if err != nil {
			continue
		}

		if fileInfo.ModTime().Before(cutoffTime) {
			dateDir := filepath.Join(cs.checkpointDir, file.Name())
			if err := os.RemoveAll(dateDir); err != nil {
				cs.logger.Error("Failed to delete old date directory %s: %v", file.Name(), err)
			} else {
				deletedCount++
			}
		}
	}

	cs.logger.Info("Cleaned up %d old checkpoints/date folders (older than %d days)", deletedCount, daysToKeep)
	return nil
} 