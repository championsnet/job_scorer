package multitenant

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"

	"job-scorer/services/storage"
)

func NewObjectStorage(cfg *RuntimeConfig) (*storage.GCSStorage, error) {
	return storage.NewGCSStorage(storage.GCSConfig{
		BucketName:  cfg.GCSBucketName,
		ProjectID:   cfg.GCSProjectID,
		Enabled:     cfg.GCSEnabled,
		FallbackDir: cfg.GCSFallbackDir,
	})
}

func SaveAccountCV(store *storage.GCSStorage, cfg *RuntimeConfig, accountID, originalFilename string, data []byte) (string, string, error) {
	ext := strings.ToLower(filepath.Ext(strings.TrimSpace(originalFilename)))
	if ext == "" {
		ext = ".txt"
	}
	objectPath := fmt.Sprintf("%s/%s/cv/%s%s", cfg.GCSCVPrefix, accountID, uuid.NewString(), ext)
	if err := store.UploadData(bytes.NewReader(data), objectPath); err != nil {
		return "", "", fmt.Errorf("failed uploading CV object: %w", err)
	}

	sum := sha256.Sum256(data)
	return objectPath, hex.EncodeToString(sum[:]), nil
}

func MaterializeAccountCV(store *storage.GCSStorage, objectPath string) (string, func(), error) {
	objectPath = strings.TrimSpace(objectPath)
	if objectPath == "" {
		return "", nil, fmt.Errorf("empty CV object path")
	}

	tmpDir, err := os.MkdirTemp("", "job-scorer-cv-*")
	if err != nil {
		return "", nil, err
	}
	localPath := filepath.Join(tmpDir, "cv"+filepath.Ext(objectPath))
	if err := store.DownloadFile(objectPath, localPath); err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", nil, fmt.Errorf("failed downloading CV object: %w", err)
	}
	cleanup := func() {
		_ = os.RemoveAll(tmpDir)
	}
	return localPath, cleanup, nil
}
