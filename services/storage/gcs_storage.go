package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"job-scorer/models"
	"job-scorer/utils"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
)

type GCSStorage struct {
	client      *storage.Client
	bucketName  string
	logger      *utils.Logger
	ctx         context.Context
	enabled     bool
	fallbackDir string
}

type GCSConfig struct {
	BucketName    string
	ProjectID     string
	Enabled       bool
	FallbackDir   string
}

// NewGCSStorage creates a new GCS storage service
func NewGCSStorage(config GCSConfig) (*GCSStorage, error) {
	logger := utils.NewLogger("GCSStorage")
	
	gcs := &GCSStorage{
		bucketName:  config.BucketName,
		logger:      logger,
		ctx:         context.Background(),
		enabled:     config.Enabled,
		fallbackDir: config.FallbackDir,
	}

	if !config.Enabled {
		logger.Info("GCS storage disabled, using local fallback directory: %s", config.FallbackDir)
		return gcs, nil
	}

	// Create GCS client
	client, err := storage.NewClient(gcs.ctx)
	if err != nil {
		logger.Error("Failed to create GCS client: %v", err)
		logger.Warning("Falling back to local storage")
		gcs.enabled = false
		return gcs, nil
	}

	gcs.client = client
	
	// Test bucket access
	bucket := client.Bucket(config.BucketName)
	_, err = bucket.Attrs(gcs.ctx)
	if err != nil {
		logger.Error("Failed to access GCS bucket '%s': %v", config.BucketName, err)
		logger.Warning("Falling back to local storage")
		gcs.enabled = false
		client.Close()
		gcs.client = nil
		return gcs, nil
	}

	logger.Info("Successfully connected to GCS bucket: %s", config.BucketName)
	return gcs, nil
}

// Close closes the GCS client
func (gcs *GCSStorage) Close() error {
	if gcs.client != nil {
		return gcs.client.Close()
	}
	return nil
}

// IsEnabled returns true if GCS storage is enabled and working
func (gcs *GCSStorage) IsEnabled() bool {
	return gcs.enabled
}

// UploadFile uploads a local file to GCS
func (gcs *GCSStorage) UploadFile(localPath, gcsPath string) error {
	if !gcs.enabled {
		return gcs.copyToFallback(localPath, gcsPath)
	}

	file, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("failed to open local file %s: %w", localPath, err)
	}
	defer file.Close()

	return gcs.UploadData(file, gcsPath)
}

// UploadData uploads data from a reader to GCS
func (gcs *GCSStorage) UploadData(reader io.Reader, gcsPath string) error {
	if !gcs.enabled {
		return gcs.saveToFallback(reader, gcsPath)
	}

	obj := gcs.client.Bucket(gcs.bucketName).Object(gcsPath)
	writer := obj.NewWriter(gcs.ctx)
	
	// Set metadata
	writer.Metadata = map[string]string{
		"uploaded-by": "job-scorer",
		"upload-time": time.Now().Format(time.RFC3339),
	}

	// Copy data
	_, err := io.Copy(writer, reader)
	if err != nil {
		writer.Close()
		return fmt.Errorf("failed to upload data to GCS: %w", err)
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf("failed to finalize GCS upload: %w", err)
	}

	gcs.logger.Info("Successfully uploaded to GCS: %s", gcsPath)
	return nil
}

// DownloadFile downloads a file from GCS to local path
func (gcs *GCSStorage) DownloadFile(gcsPath, localPath string) error {
	if !gcs.enabled {
		return gcs.copyFromFallback(gcsPath, localPath)
	}

	obj := gcs.client.Bucket(gcs.bucketName).Object(gcsPath)
	reader, err := obj.NewReader(gcs.ctx)
	if err != nil {
		return fmt.Errorf("failed to create GCS reader: %w", err)
	}
	defer reader.Close()

	// Create local directory if needed
	dir := filepath.Dir(localPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create local directory: %w", err)
	}

	file, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("failed to create local file: %w", err)
	}
	defer file.Close()

	_, err = io.Copy(file, reader)
	if err != nil {
		return fmt.Errorf("failed to download from GCS: %w", err)
	}

	gcs.logger.Info("Successfully downloaded from GCS: %s -> %s", gcsPath, localPath)
	return nil
}

// DownloadData downloads data from GCS and returns it as bytes
func (gcs *GCSStorage) DownloadData(gcsPath string) ([]byte, error) {
	if !gcs.enabled {
		return gcs.readFromFallback(gcsPath)
	}

	obj := gcs.client.Bucket(gcs.bucketName).Object(gcsPath)
	reader, err := obj.NewReader(gcs.ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCS reader: %w", err)
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read from GCS: %w", err)
	}

	gcs.logger.Debug("Successfully downloaded data from GCS: %s (%d bytes)", gcsPath, len(data))
	return data, nil
}

// FileExists checks if a file exists in GCS
func (gcs *GCSStorage) FileExists(gcsPath string) bool {
	if !gcs.enabled {
		return gcs.fallbackFileExists(gcsPath)
	}

	obj := gcs.client.Bucket(gcs.bucketName).Object(gcsPath)
	_, err := obj.Attrs(gcs.ctx)
	return err == nil
}

// DeleteFile deletes a file from GCS
func (gcs *GCSStorage) DeleteFile(gcsPath string) error {
	if !gcs.enabled {
		return gcs.deleteFromFallback(gcsPath)
	}

	obj := gcs.client.Bucket(gcs.bucketName).Object(gcsPath)
	if err := obj.Delete(gcs.ctx); err != nil {
		return fmt.Errorf("failed to delete from GCS: %w", err)
	}

	gcs.logger.Info("Successfully deleted from GCS: %s", gcsPath)
	return nil
}

// ListFiles lists files in a GCS directory (prefix)
func (gcs *GCSStorage) ListFiles(prefix string) ([]string, error) {
	if !gcs.enabled {
		return gcs.listFallbackFiles(prefix)
	}

	bucket := gcs.client.Bucket(gcs.bucketName)
	query := &storage.Query{Prefix: prefix}
	
	var files []string
	it := bucket.Objects(gcs.ctx, query)
	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to list GCS objects: %w", err)
		}
		files = append(files, attrs.Name)
	}

	gcs.logger.Debug("Listed %d files with prefix: %s", len(files), prefix)
	return files, nil
}

// SaveJobData saves job data to GCS with automatic JSON marshaling
func (gcs *GCSStorage) SaveJobData(jobs []*models.Job, filename string) error {
	data, err := json.MarshalIndent(jobs, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal jobs: %w", err)
	}

	reader := strings.NewReader(string(data))
	path := fmt.Sprintf("job-data/%s", filename)
	
	if err := gcs.UploadData(reader, path); err != nil {
		return fmt.Errorf("failed to save job data: %w", err)
	}

	gcs.logger.Info("Saved %d jobs to %s", len(jobs), path)
	return nil
}

// LoadJobData loads job data from GCS with automatic JSON unmarshaling
func (gcs *GCSStorage) LoadJobData(filename string) ([]*models.Job, error) {
	path := fmt.Sprintf("job-data/%s", filename)
	
	data, err := gcs.DownloadData(path)
	if err != nil {
		return nil, fmt.Errorf("failed to load job data: %w", err)
	}

	var jobs []*models.Job
	if err := json.Unmarshal(data, &jobs); err != nil {
		return nil, fmt.Errorf("failed to unmarshal job data: %w", err)
	}

	gcs.logger.Info("Loaded %d jobs from %s", len(jobs), path)
	return jobs, nil
}

// SaveProcessedIDs saves processed job IDs to GCS
func (gcs *GCSStorage) SaveProcessedIDs(processedIDs map[string]time.Time) error {
	// Convert time.Time to string for JSON serialization
	processedData := make(map[string]string)
	for jobID, timestamp := range processedIDs {
		processedData[jobID] = timestamp.Format(time.RFC3339)
	}

	data, err := json.MarshalIndent(processedData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal processed IDs: %w", err)
	}

	reader := strings.NewReader(string(data))
	path := "data/processed_job_ids.json"
	
	if err := gcs.UploadData(reader, path); err != nil {
		return fmt.Errorf("failed to save processed IDs: %w", err)
	}

	gcs.logger.Info("Saved %d processed job IDs to GCS", len(processedIDs))
	return nil
}

// LoadProcessedIDs loads processed job IDs from GCS
func (gcs *GCSStorage) LoadProcessedIDs() (map[string]time.Time, error) {
	path := "data/processed_job_ids.json"
	
	data, err := gcs.DownloadData(path)
	if err != nil {
		// File might not exist yet, return empty map
		if strings.Contains(err.Error(), "object doesn't exist") {
			gcs.logger.Info("No existing processed job IDs found in GCS, starting fresh")
			return make(map[string]time.Time), nil
		}
		return nil, fmt.Errorf("failed to load processed IDs: %w", err)
	}

	var processedData map[string]string
	if err := json.Unmarshal(data, &processedData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal processed IDs: %w", err)
	}

	// Convert string timestamps back to time.Time
	processedIDs := make(map[string]time.Time)
	for jobID, timestampStr := range processedData {
		if timestamp, err := time.Parse(time.RFC3339, timestampStr); err == nil {
			processedIDs[jobID] = timestamp
		} else {
			gcs.logger.Warning("Invalid timestamp for job ID %s: %s", jobID, timestampStr)
		}
	}

	gcs.logger.Info("Loaded %d processed job IDs from GCS", len(processedIDs))
	return processedIDs, nil
}

// SaveLogFile uploads a log file to GCS
func (gcs *GCSStorage) SaveLogFile(localLogPath string) error {
	// Extract filename from path for GCS path
	filename := filepath.Base(localLogPath)
	gcsPath := fmt.Sprintf("logs/%s", filename)
	
	return gcs.UploadFile(localLogPath, gcsPath)
}

// SaveCheckpoint saves checkpoint data to GCS
func (gcs *GCSStorage) SaveCheckpoint(jobs []*models.Job, stage, runFolder string, metadata map[string]interface{}) error {
	timestamp := time.Now().Format("15-04-05")
	
	checkpoint := struct {
		Timestamp time.Time              `json:"timestamp"`
		Stage     string                 `json:"stage"`
		Metadata  map[string]interface{} `json:"metadata"`
		JobCount  int                    `json:"job_count"`
		Jobs      []*models.Job          `json:"jobs"`
	}{
		Timestamp: time.Now(),
		Stage:     stage,
		Metadata:  metadata,
		JobCount:  len(jobs),
		Jobs:      jobs,
	}

	data, err := json.MarshalIndent(checkpoint, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal checkpoint: %w", err)
	}

	filename := fmt.Sprintf("checkpoint_%s_%s.json", stage, timestamp)
	gcsPath := fmt.Sprintf("checkpoints/%s/%s", runFolder, filename)
	
	reader := strings.NewReader(string(data))
	if err := gcs.UploadData(reader, gcsPath); err != nil {
		return fmt.Errorf("failed to save checkpoint: %w", err)
	}

	gcs.logger.Info("Saved checkpoint to GCS: %s", gcsPath)
	return nil
}

// Fallback methods for when GCS is disabled
func (gcs *GCSStorage) copyToFallback(localPath, gcsPath string) error {
	fallbackPath := filepath.Join(gcs.fallbackDir, gcsPath)
	
	// Create directory if needed
	dir := filepath.Dir(fallbackPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create fallback directory: %w", err)
	}

	// Copy file
	src, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer src.Close()

	dst, err := os.Create(fallbackPath)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	if err != nil {
		return fmt.Errorf("failed to copy file: %w", err)
	}

	gcs.logger.Debug("Copied to fallback: %s -> %s", localPath, fallbackPath)
	return nil
}

func (gcs *GCSStorage) saveToFallback(reader io.Reader, gcsPath string) error {
	fallbackPath := filepath.Join(gcs.fallbackDir, gcsPath)
	
	// Create directory if needed
	dir := filepath.Dir(fallbackPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create fallback directory: %w", err)
	}

	file, err := os.Create(fallbackPath)
	if err != nil {
		return fmt.Errorf("failed to create fallback file: %w", err)
	}
	defer file.Close()

	_, err = io.Copy(file, reader)
	if err != nil {
		return fmt.Errorf("failed to save to fallback: %w", err)
	}

	gcs.logger.Debug("Saved to fallback: %s", fallbackPath)
	return nil
}

func (gcs *GCSStorage) copyFromFallback(gcsPath, localPath string) error {
	fallbackPath := filepath.Join(gcs.fallbackDir, gcsPath)
	
	src, err := os.Open(fallbackPath)
	if err != nil {
		return fmt.Errorf("failed to open fallback file: %w", err)
	}
	defer src.Close()

	// Create local directory if needed
	dir := filepath.Dir(localPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create local directory: %w", err)
	}

	dst, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("failed to create local file: %w", err)
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	return err
}

func (gcs *GCSStorage) readFromFallback(gcsPath string) ([]byte, error) {
	fallbackPath := filepath.Join(gcs.fallbackDir, gcsPath)
	return os.ReadFile(fallbackPath)
}

func (gcs *GCSStorage) fallbackFileExists(gcsPath string) bool {
	fallbackPath := filepath.Join(gcs.fallbackDir, gcsPath)
	_, err := os.Stat(fallbackPath)
	return err == nil
}

func (gcs *GCSStorage) deleteFromFallback(gcsPath string) error {
	fallbackPath := filepath.Join(gcs.fallbackDir, gcsPath)
	return os.Remove(fallbackPath)
}

func (gcs *GCSStorage) listFallbackFiles(prefix string) ([]string, error) {
	fallbackPrefix := filepath.Join(gcs.fallbackDir, prefix)
	var files []string
	
	err := filepath.Walk(fallbackPrefix, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			// Convert back to relative path
			relPath, err := filepath.Rel(gcs.fallbackDir, path)
			if err != nil {
				return err
			}
			files = append(files, relPath)
		}
		return nil
	})
	
	return files, err
} 