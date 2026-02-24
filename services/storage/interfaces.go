package storage

import (
	"job-scorer/models"
	"time"
)

// JobStorage interface for saving and loading job data
type JobStorage interface {
	SaveAllJobs(jobs []*models.Job) error
	SavePromisingJobs(jobs []*models.Job) error
	SaveFinalEvaluatedJobs(jobs []*models.Job) error
	LoadAllJobs() ([]*models.Job, error)
	LoadPromisingJobs() ([]*models.Job, error)
	LoadFinalEvaluatedJobs() ([]*models.Job, error)
	FileExists(filename string) bool
	DeleteFile(filename string) error
	GetOutputDir() string
	EnsureOutputDir() error
}

// JobTracker interface for tracking processed job IDs
type JobTrackerInterface interface {
	IsProcessed(jobID string) bool
	MarkAsProcessed(jobID string) error
	MarkMultipleAsProcessed(jobIDs []string) error
	FilterProcessedJobs(jobs []*models.Job) []*models.Job
	GetStats() map[string]interface{}
	CleanupOldEntries(olderThan time.Duration) error
}

// CheckpointStorage interface for saving and loading checkpoints
type CheckpointStorage interface {
	SetRunFolder(folder string)
	GetRunFolder() string
	SaveCheckpoint(jobs []*models.Job, stage string, metadata map[string]interface{}) error
	SaveDailySnapshot(allJobs, promisingJobs, finalJobs []*models.Job) error
	ListCheckpoints() (map[string][]string, error)
	LoadCheckpoint(dateFolder, filename string) ([]*models.Job, error)
	LoadCheckpointLegacy(filename string) ([]*models.Job, error)
	CleanupOldCheckpoints(daysToKeep int) error
} 