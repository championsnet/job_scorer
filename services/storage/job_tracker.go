package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"job-scorer/models"
	"job-scorer/utils"
)

// JobTracker manages a set of processed job IDs to avoid reprocessing
type JobTracker struct {
	filePath    string
	processedIDs map[string]time.Time // jobID -> timestamp when processed
	mutex       sync.RWMutex
	logger      *utils.Logger
}

// NewJobTracker creates a new job tracker that saves to a JSON file
func NewJobTracker(dataDir string) (*JobTracker, error) {
	filePath := filepath.Join(dataDir, "processed_job_ids.json")
	
	tracker := &JobTracker{
		filePath:     filePath,
		processedIDs: make(map[string]time.Time),
		logger:       utils.NewLogger("JobTracker"),
	}
	
	// Load existing processed IDs if file exists
	if err := tracker.loadProcessedIDs(); err != nil {
		return nil, fmt.Errorf("failed to load processed job IDs: %w", err)
	}
	
	tracker.logger.Info("Job tracker initialized with %d existing processed job IDs", len(tracker.processedIDs))
	return tracker, nil
}

// IsProcessed checks if a job ID has already been processed
func (jt *JobTracker) IsProcessed(jobID string) bool {
	if jobID == "" {
		return false // Empty job IDs are not considered processed
	}
	
	jt.mutex.RLock()
	defer jt.mutex.RUnlock()
	
	_, exists := jt.processedIDs[jobID]
	return exists
}

// MarkAsProcessed adds a job ID to the processed set
func (jt *JobTracker) MarkAsProcessed(jobID string) error {
	if jobID == "" {
		return fmt.Errorf("cannot mark empty job ID as processed")
	}
	
	jt.mutex.Lock()
	defer jt.mutex.Unlock()
	
	jt.processedIDs[jobID] = time.Now()
	
	// Save to file after each addition
	return jt.saveProcessedIDs()
}

// MarkMultipleAsProcessed adds multiple job IDs to the processed set
func (jt *JobTracker) MarkMultipleAsProcessed(jobIDs []string) error {
	if len(jobIDs) == 0 {
		return nil
	}
	
	jt.mutex.Lock()
	defer jt.mutex.Unlock()
	
	now := time.Now()
	for _, jobID := range jobIDs {
		if jobID != "" {
			jt.processedIDs[jobID] = now
		}
	}
	
	// Save to file after batch addition
	return jt.saveProcessedIDs()
}

// FilterProcessedJobs removes jobs that have already been processed
func (jt *JobTracker) FilterProcessedJobs(jobs []*models.Job) []*models.Job {
	if len(jobs) == 0 {
		return jobs
	}
	
	var filteredJobs []*models.Job
	var processedCount int
	
	for _, job := range jobs {
		if jt.IsProcessed(job.JobID) {
			processedCount++
			jt.logger.Debug("Skipping already processed job: %s (ID: %s)", job.Position, job.JobID)
		} else {
			filteredJobs = append(filteredJobs, job)
		}
	}
	
	if processedCount > 0 {
		jt.logger.Info("Filtered out %d already processed jobs, %d new jobs remaining", processedCount, len(filteredJobs))
	}
	
	return filteredJobs
}

// GetStats returns statistics about processed jobs
func (jt *JobTracker) GetStats() map[string]interface{} {
	jt.mutex.RLock()
	defer jt.mutex.RUnlock()
	
	// Calculate some basic stats
	var totalJobs int
	var recentJobs int
	oneWeekAgo := time.Now().AddDate(0, 0, -7)
	
	for _, timestamp := range jt.processedIDs {
		totalJobs++
		if timestamp.After(oneWeekAgo) {
			recentJobs++
		}
	}
	
	return map[string]interface{}{
		"total_processed_jobs": totalJobs,
		"recent_jobs_7_days":   recentJobs,
		"tracking_file":        jt.filePath,
	}
}

// CleanupOldEntries removes job IDs older than the specified duration
func (jt *JobTracker) CleanupOldEntries(olderThan time.Duration) error {
	jt.mutex.Lock()
	defer jt.mutex.Unlock()
	
	cutoff := time.Now().Add(-olderThan)
	var removedCount int
	
	for jobID, timestamp := range jt.processedIDs {
		if timestamp.Before(cutoff) {
			delete(jt.processedIDs, jobID)
			removedCount++
		}
	}
	
	if removedCount > 0 {
		jt.logger.Info("Cleaned up %d old job IDs (older than %v)", removedCount, olderThan)
		return jt.saveProcessedIDs()
	}
	
	return nil
}

// loadProcessedIDs loads the processed job IDs from the JSON file
func (jt *JobTracker) loadProcessedIDs() error {
	// Check if file exists
	if _, err := os.Stat(jt.filePath); os.IsNotExist(err) {
		jt.logger.Info("No existing processed job IDs file found, starting fresh")
		return nil
	}
	
	// Read file
	data, err := os.ReadFile(jt.filePath)
	if err != nil {
		return fmt.Errorf("error reading processed job IDs file: %w", err)
	}
	
	// Unmarshal JSON
	var processedData map[string]string
	if err := json.Unmarshal(data, &processedData); err != nil {
		return fmt.Errorf("error unmarshaling processed job IDs: %w", err)
	}
	
	// Convert string timestamps back to time.Time
	for jobID, timestampStr := range processedData {
		if timestamp, err := time.Parse(time.RFC3339, timestampStr); err == nil {
			jt.processedIDs[jobID] = timestamp
		} else {
			jt.logger.Warning("Invalid timestamp for job ID %s: %s", jobID, timestampStr)
		}
	}
	
	jt.logger.Info("Loaded %d processed job IDs from %s", len(jt.processedIDs), jt.filePath)
	return nil
}

// saveProcessedIDs saves the processed job IDs to the JSON file
func (jt *JobTracker) saveProcessedIDs() error {
	// Create directory if it doesn't exist
	dir := filepath.Dir(jt.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("error creating directory %s: %w", dir, err)
	}
	
	// Convert time.Time to string for JSON serialization
	processedData := make(map[string]string)
	for jobID, timestamp := range jt.processedIDs {
		processedData[jobID] = timestamp.Format(time.RFC3339)
	}
	
	// Marshal to JSON with indentation
	jsonData, err := json.MarshalIndent(processedData, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling processed job IDs to JSON: %w", err)
	}
	
	// Write to file
	if err := os.WriteFile(jt.filePath, jsonData, 0644); err != nil {
		return fmt.Errorf("error writing processed job IDs file: %w", err)
	}
	
	jt.logger.Debug("Saved %d processed job IDs to %s", len(jt.processedIDs), jt.filePath)
	return nil
} 