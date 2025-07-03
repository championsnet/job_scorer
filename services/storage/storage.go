package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"job-scorer/models"
	"job-scorer/utils"
)

type FileStorage struct {
	outputDir string
	logger    *utils.Logger
}

func NewFileStorage(outputDir string) *FileStorage {
	return &FileStorage{
		outputDir: outputDir,
		logger:    utils.NewLogger("FileStorage"),
	}
}

func (fs *FileStorage) SaveAllJobs(jobs []*models.Job) error {
	filename := filepath.Join(fs.outputDir, "allJobs.json")
	return fs.saveJobsToFile(jobs, filename)
}

func (fs *FileStorage) SavePromisingJobs(jobs []*models.Job) error {
	filename := filepath.Join(fs.outputDir, "promisingJobs.json")
	return fs.saveJobsToFile(jobs, filename)
}

func (fs *FileStorage) SaveFinalEvaluatedJobs(jobs []*models.Job) error {
	filename := filepath.Join(fs.outputDir, "finalEvaluatedJobs.json")
	return fs.saveJobsToFile(jobs, filename)
}

func (fs *FileStorage) saveJobsToFile(jobs []*models.Job, filename string) error {
	// Create directory if it doesn't exist
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("error creating directory %s: %w", dir, err)
	}

	// Marshal jobs to JSON with indentation
	jsonData, err := json.MarshalIndent(jobs, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling jobs to JSON: %w", err)
	}

	// Write to file
	if err := os.WriteFile(filename, jsonData, 0644); err != nil {
		return fmt.Errorf("error writing file %s: %w", filename, err)
	}

	fs.logger.Info("Saved %d jobs to %s", len(jobs), filename)
	return nil
}

func (fs *FileStorage) LoadAllJobs() ([]*models.Job, error) {
	filename := filepath.Join(fs.outputDir, "allJobs.json")
	return fs.loadJobsFromFile(filename)
}

func (fs *FileStorage) LoadPromisingJobs() ([]*models.Job, error) {
	filename := filepath.Join(fs.outputDir, "promisingJobs.json")
	return fs.loadJobsFromFile(filename)
}

func (fs *FileStorage) LoadFinalEvaluatedJobs() ([]*models.Job, error) {
	filename := filepath.Join(fs.outputDir, "finalEvaluatedJobs.json")
	return fs.loadJobsFromFile(filename)
}

func (fs *FileStorage) loadJobsFromFile(filename string) ([]*models.Job, error) {
	// Check if file exists
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		fs.logger.Info("File %s does not exist, returning empty slice", filename)
		return []*models.Job{}, nil
	}

	// Read file
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("error reading file %s: %w", filename, err)
	}

	// Unmarshal JSON
	var jobs []*models.Job
	if err := json.Unmarshal(data, &jobs); err != nil {
		return nil, fmt.Errorf("error unmarshaling JSON from %s: %w", filename, err)
	}

	fs.logger.Info("Loaded %d jobs from %s", len(jobs), filename)
	return jobs, nil
}

func (fs *FileStorage) FileExists(filename string) bool {
	fullPath := filepath.Join(fs.outputDir, filename)
	_, err := os.Stat(fullPath)
	return !os.IsNotExist(err)
}

func (fs *FileStorage) DeleteFile(filename string) error {
	fullPath := filepath.Join(fs.outputDir, filename)
	if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("error deleting file %s: %w", fullPath, err)
	}
	fs.logger.Info("Deleted file %s", fullPath)
	return nil
}

func (fs *FileStorage) GetOutputDir() string {
	return fs.outputDir
}

func (fs *FileStorage) EnsureOutputDir() error {
	if err := os.MkdirAll(fs.outputDir, 0755); err != nil {
		return fmt.Errorf("error creating output directory %s: %w", fs.outputDir, err)
	}
	return nil
} 