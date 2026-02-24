package utils

import (
	"fmt"
	"os"
)

// GCSLogger extends Logger with GCS upload capabilities
type GCSLogger struct {
	*Logger
	gcsStorage   GCSUploader
	uploadOnClose bool
}

// GCSUploader interface for uploading files to GCS
type GCSUploader interface {
	SaveLogFile(localLogPath string) error
}

// NewGCSFileLogger creates a logger that writes to both console and file, and uploads to GCS
func NewGCSFileLogger(serviceName, logDir string, gcsStorage GCSUploader, uploadOnClose bool) (*GCSLogger, error) {
	// Create base file logger
	baseLogger, err := NewFileLogger(serviceName, logDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create base logger: %w", err)
	}

	return &GCSLogger{
		Logger:        baseLogger,
		gcsStorage:    gcsStorage,
		uploadOnClose: uploadOnClose,
	}, nil
}

// Close closes the log file and optionally uploads it to GCS
func (gl *GCSLogger) Close() error {
	// Get the file path before closing the file
	var logFilePath string
	if gl.file != nil {
		logFilePath = gl.file.Name()
	}

	// Close the base logger file
	if err := gl.Logger.Close(); err != nil {
		return fmt.Errorf("failed to close base logger: %w", err)
	}

	// Upload to GCS if enabled and file exists
	if gl.uploadOnClose && gl.gcsStorage != nil && logFilePath != "" {
		if _, err := os.Stat(logFilePath); err == nil {
			gl.Info("Uploading log file to GCS: %s", logFilePath)
			if err := gl.gcsStorage.SaveLogFile(logFilePath); err != nil {
				gl.Error("Failed to upload log file to GCS: %v", err)
				return fmt.Errorf("failed to upload log file to GCS: %w", err)
			}
			gl.Info("Successfully uploaded log file to GCS")
		}
	}

	return nil
}

// UploadLogNow immediately uploads the current log file to GCS (without closing it)
func (gl *GCSLogger) UploadLogNow() error {
	if gl.gcsStorage == nil {
		return fmt.Errorf("GCS storage not configured")
	}

	if gl.file == nil {
		return fmt.Errorf("no log file open")
	}

	logFilePath := gl.file.Name()
	
	// Flush any pending writes
	if gl.file != nil {
		gl.file.Sync()
	}

	gl.Info("Uploading current log file to GCS: %s", logFilePath)
	if err := gl.gcsStorage.SaveLogFile(logFilePath); err != nil {
		gl.Error("Failed to upload current log file to GCS: %v", err)
		return fmt.Errorf("failed to upload log file to GCS: %w", err)
	}
	
	gl.Info("Successfully uploaded current log file to GCS")
	return nil
} 