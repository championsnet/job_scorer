package storage

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"job-scorer/models"
	"job-scorer/utils"
)

// GCSRunSummaryStore persists run summaries in GCS checkpoints/{runID}/run_summary.json
type GCSRunSummaryStore struct {
	gcsStorage *GCSStorage
	logger     *utils.Logger
}

// NewGCSRunSummaryStore creates a GCS-backed run summary store
func NewGCSRunSummaryStore(gcsStorage *GCSStorage) *GCSRunSummaryStore {
	return &GCSRunSummaryStore{
		gcsStorage: gcsStorage,
		logger:     utils.NewLogger("GCSRunSummaryStore"),
	}
}

// SaveRunSummary writes run_summary.json to checkpoints/{runID}/run_summary.json
func (s *GCSRunSummaryStore) SaveRunSummary(summary *models.RunSummary) error {
	if summary == nil || summary.RunID == "" {
		return fmt.Errorf("run summary and run_id are required")
	}
	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal run summary: %w", err)
	}
	gcsPath := fmt.Sprintf("checkpoints/%s/%s", summary.RunID, runSummaryFilename)
	reader := strings.NewReader(string(data))
	if err := s.gcsStorage.UploadData(reader, gcsPath); err != nil {
		return fmt.Errorf("failed to save run summary to GCS: %w", err)
	}
	s.logger.Info("Saved run summary to GCS: %s", summary.RunID)
	return nil
}

// LoadRunSummary reads run_summary.json from GCS
func (s *GCSRunSummaryStore) LoadRunSummary(runID string) (*models.RunSummary, error) {
	if runID == "" {
		return nil, fmt.Errorf("run_id is required")
	}
	gcsPath := fmt.Sprintf("checkpoints/%s/%s", runID, runSummaryFilename)
	data, err := s.gcsStorage.DownloadData(gcsPath)
	if err != nil {
		if strings.Contains(err.Error(), "doesn't exist") || strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to load run summary from GCS: %w", err)
	}
	var summary models.RunSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		return nil, fmt.Errorf("failed to unmarshal run summary: %w", err)
	}
	return &summary, nil
}

// ListRunSummaries returns recent runs by listing checkpoints/ and loading run_summary.json from each folder
func (s *GCSRunSummaryStore) ListRunSummaries(limit int) ([]*models.RunSummary, error) {
	files, err := s.gcsStorage.ListFiles("checkpoints/")
	if err != nil {
		return nil, fmt.Errorf("failed to list checkpoints from GCS: %w", err)
	}
	runFolders := make(map[string]bool)
	for _, f := range files {
		parts := strings.Split(f, "/")
		if len(parts) >= 3 && parts[0] == "checkpoints" {
			runFolder := parts[1]
			filename := parts[2]
			if filename == runSummaryFilename {
				runFolders[runFolder] = true
			}
		}
	}
	var folders []string
	for f := range runFolders {
		folders = append(folders, f)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(folders)))
	if limit > 0 && len(folders) > limit {
		folders = folders[:limit]
	}
	var summaries []*models.RunSummary
	for _, folder := range folders {
		summary, err := s.LoadRunSummary(folder)
		if err != nil {
			s.logger.Warning("Failed to load run summary %s: %v", folder, err)
			continue
		}
		if summary != nil {
			summaries = append(summaries, summary)
		}
	}
	return summaries, nil
}
