package storage

import (
	"fmt"
	"sync"
	"time"

	"job-scorer/models"
	"job-scorer/utils"
)

// GCSJobTracker manages processed job IDs using GCS storage
type GCSJobTracker struct {
	gcsStorage   *GCSStorage
	processedIDs map[string]time.Time // jobID -> timestamp when processed
	mutex        sync.RWMutex
	logger       *utils.Logger
}

// NewGCSJobTracker creates a new job tracker that uses GCS for persistence
func NewGCSJobTracker(gcsStorage *GCSStorage) (*GCSJobTracker, error) {
	tracker := &GCSJobTracker{
		gcsStorage:   gcsStorage,
		processedIDs: make(map[string]time.Time),
		logger:       utils.NewLogger("GCSJobTracker"),
	}
	
	// Load existing processed IDs from GCS
	if err := tracker.loadProcessedIDs(); err != nil {
		return nil, fmt.Errorf("failed to load processed job IDs: %w", err)
	}
	
	tracker.logger.Info("GCS job tracker initialized with %d existing processed job IDs", len(tracker.processedIDs))
	return tracker, nil
}

// IsProcessed checks if a job ID has already been processed
func (jt *GCSJobTracker) IsProcessed(jobID string) bool {
	if jobID == "" {
		return false // Empty job IDs are not considered processed
	}
	
	jt.mutex.RLock()
	defer jt.mutex.RUnlock()
	
	_, exists := jt.processedIDs[jobID]
	return exists
}

// MarkAsProcessed adds a job ID to the processed set
func (jt *GCSJobTracker) MarkAsProcessed(jobID string) error {
	if jobID == "" {
		return fmt.Errorf("cannot mark empty job ID as processed")
	}
	
	jt.mutex.Lock()
	defer jt.mutex.Unlock()
	
	jt.processedIDs[jobID] = time.Now()
	
	// Save to GCS after each addition
	return jt.saveProcessedIDs()
}

// MarkMultipleAsProcessed adds multiple job IDs to the processed set
func (jt *GCSJobTracker) MarkMultipleAsProcessed(jobIDs []string) error {
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
	
	// Save to GCS after batch addition
	return jt.saveProcessedIDs()
}

// FilterProcessedJobs removes jobs that have already been processed
func (jt *GCSJobTracker) FilterProcessedJobs(jobs []*models.Job) []*models.Job {
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
func (jt *GCSJobTracker) GetStats() map[string]interface{} {
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
		"storage_type":         "GCS",
		"gcs_enabled":          jt.gcsStorage.enabled,
	}
}

// CleanupOldEntries removes job IDs older than the specified duration
func (jt *GCSJobTracker) CleanupOldEntries(olderThan time.Duration) error {
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

// loadProcessedIDs loads the processed job IDs from GCS
func (jt *GCSJobTracker) loadProcessedIDs() error {
	processedIDs, err := jt.gcsStorage.LoadProcessedIDs()
	if err != nil {
		return fmt.Errorf("error loading processed job IDs from GCS: %w", err)
	}
	
	jt.processedIDs = processedIDs
	jt.logger.Info("Loaded %d processed job IDs from GCS", len(jt.processedIDs))
	return nil
}

// saveProcessedIDs saves the processed job IDs to GCS
func (jt *GCSJobTracker) saveProcessedIDs() error {
	if err := jt.gcsStorage.SaveProcessedIDs(jt.processedIDs); err != nil {
		return fmt.Errorf("error saving processed job IDs to GCS: %w", err)
	}
	
	jt.logger.Debug("Saved %d processed job IDs to GCS", len(jt.processedIDs))
	return nil
} 