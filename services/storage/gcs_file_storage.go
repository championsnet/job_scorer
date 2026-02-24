package storage

import (
	"fmt"

	"job-scorer/models"
	"job-scorer/utils"
)

// GCSFileStorage provides job data storage using GCS
type GCSFileStorage struct {
	gcsStorage *GCSStorage
	logger     *utils.Logger
}

// NewGCSFileStorage creates a new GCS file storage service
func NewGCSFileStorage(gcsStorage *GCSStorage) *GCSFileStorage {
	return &GCSFileStorage{
		gcsStorage: gcsStorage,
		logger:     utils.NewLogger("GCSFileStorage"),
	}
}

// SaveAllJobs saves all jobs to GCS
func (gfs *GCSFileStorage) SaveAllJobs(jobs []*models.Job) error {
	if err := gfs.gcsStorage.SaveJobData(jobs, "allJobs.json"); err != nil {
		return fmt.Errorf("failed to save all jobs: %w", err)
	}
	gfs.logger.Info("Saved %d jobs to GCS (allJobs.json)", len(jobs))
	return nil
}

// SavePromisingJobs saves promising jobs to GCS
func (gfs *GCSFileStorage) SavePromisingJobs(jobs []*models.Job) error {
	if err := gfs.gcsStorage.SaveJobData(jobs, "promisingJobs.json"); err != nil {
		return fmt.Errorf("failed to save promising jobs: %w", err)
	}
	gfs.logger.Info("Saved %d promising jobs to GCS", len(jobs))
	return nil
}

// SaveFinalEvaluatedJobs saves final evaluated jobs to GCS
func (gfs *GCSFileStorage) SaveFinalEvaluatedJobs(jobs []*models.Job) error {
	if err := gfs.gcsStorage.SaveJobData(jobs, "finalEvaluatedJobs.json"); err != nil {
		return fmt.Errorf("failed to save final evaluated jobs: %w", err)
	}
	gfs.logger.Info("Saved %d final evaluated jobs to GCS", len(jobs))
	return nil
}

// LoadAllJobs loads all jobs from GCS
func (gfs *GCSFileStorage) LoadAllJobs() ([]*models.Job, error) {
	jobs, err := gfs.gcsStorage.LoadJobData("allJobs.json")
	if err != nil {
		return []*models.Job{}, nil // Return empty slice if file doesn't exist
	}
	gfs.logger.Info("Loaded %d jobs from GCS (allJobs.json)", len(jobs))
	return jobs, nil
}

// LoadPromisingJobs loads promising jobs from GCS
func (gfs *GCSFileStorage) LoadPromisingJobs() ([]*models.Job, error) {
	jobs, err := gfs.gcsStorage.LoadJobData("promisingJobs.json")
	if err != nil {
		return []*models.Job{}, nil // Return empty slice if file doesn't exist
	}
	gfs.logger.Info("Loaded %d promising jobs from GCS", len(jobs))
	return jobs, nil
}

// LoadFinalEvaluatedJobs loads final evaluated jobs from GCS
func (gfs *GCSFileStorage) LoadFinalEvaluatedJobs() ([]*models.Job, error) {
	jobs, err := gfs.gcsStorage.LoadJobData("finalEvaluatedJobs.json")
	if err != nil {
		return []*models.Job{}, nil // Return empty slice if file doesn't exist
	}
	gfs.logger.Info("Loaded %d final evaluated jobs from GCS", len(jobs))
	return jobs, nil
}

// FileExists checks if a job data file exists in GCS
func (gfs *GCSFileStorage) FileExists(filename string) bool {
	gcsPath := fmt.Sprintf("job-data/%s", filename)
	return gfs.gcsStorage.FileExists(gcsPath)
}

// DeleteFile deletes a job data file from GCS
func (gfs *GCSFileStorage) DeleteFile(filename string) error {
	gcsPath := fmt.Sprintf("job-data/%s", filename)
	if err := gfs.gcsStorage.DeleteFile(gcsPath); err != nil {
		return fmt.Errorf("failed to delete file from GCS: %w", err)
	}
	gfs.logger.Info("Deleted file from GCS: %s", filename)
	return nil
}

// GetOutputDir returns the GCS bucket name (for compatibility)
func (gfs *GCSFileStorage) GetOutputDir() string {
	if gfs.gcsStorage.enabled {
		return fmt.Sprintf("gs://%s/job-data/", gfs.gcsStorage.bucketName)
	}
	return gfs.gcsStorage.fallbackDir
}

// EnsureOutputDir ensures the output directory exists (no-op for GCS)
func (gfs *GCSFileStorage) EnsureOutputDir() error {
	// No need to create directories in GCS, they're created automatically
	if !gfs.gcsStorage.enabled {
		gfs.logger.Info("GCS disabled, using fallback directory: %s", gfs.gcsStorage.fallbackDir)
	}
	return nil
} 