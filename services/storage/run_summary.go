package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"job-scorer/models"
	"job-scorer/utils"
)

const runSummaryFilename = "run_summary.json"

// LocalRunSummaryStore persists run summaries in the checkpoint directory
type LocalRunSummaryStore struct {
	checkpointDir string
	logger        *utils.Logger
}

// NewLocalRunSummaryStore creates a store that saves run_summary.json in each run folder
func NewLocalRunSummaryStore(checkpointDir string) (*LocalRunSummaryStore, error) {
	if err := os.MkdirAll(checkpointDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create checkpoint directory: %w", err)
	}
	logger, err := utils.NewFileLogger("RunSummaryStore", checkpointDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create logger: %w", err)
	}
	return &LocalRunSummaryStore{
		checkpointDir: checkpointDir,
		logger:        logger,
	}, nil
}

// SaveRunSummary writes run_summary.json to checkpoints/{runID}/run_summary.json
func (s *LocalRunSummaryStore) SaveRunSummary(summary *models.RunSummary) error {
	if summary == nil || summary.RunID == "" {
		return fmt.Errorf("run summary and run_id are required")
	}
	runDir := filepath.Join(s.checkpointDir, summary.RunID)
	if err := os.MkdirAll(runDir, 0755); err != nil {
		return fmt.Errorf("failed to create run directory: %w", err)
	}
	path := filepath.Join(runDir, runSummaryFilename)
	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal run summary: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write run summary: %w", err)
	}
	s.logger.Info("Saved run summary: %s", summary.RunID)
	return nil
}

// LoadRunSummary reads run_summary.json from checkpoints/{runID}/run_summary.json
func (s *LocalRunSummaryStore) LoadRunSummary(runID string) (*models.RunSummary, error) {
	if runID == "" {
		return nil, fmt.Errorf("run_id is required")
	}
	path := filepath.Join(s.checkpointDir, runID, runSummaryFilename)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read run summary: %w", err)
	}
	var summary models.RunSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		return nil, fmt.Errorf("failed to unmarshal run summary: %w", err)
	}
	return &summary, nil
}

// ListRunSummaries returns recent runs sorted by started_at descending, limited
func (s *LocalRunSummaryStore) ListRunSummaries(limit int) ([]*models.RunSummary, error) {
	entries, err := os.ReadDir(s.checkpointDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read checkpoint directory: %w", err)
	}
	var runFolders []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") && e.Name() != "legacy" {
			summaryPath := filepath.Join(s.checkpointDir, e.Name(), runSummaryFilename)
			if _, err := os.Stat(summaryPath); err == nil {
				runFolders = append(runFolders, e.Name())
			}
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(runFolders)))
	if limit > 0 && len(runFolders) > limit {
		runFolders = runFolders[:limit]
	}
	var summaries []*models.RunSummary
	for _, folder := range runFolders {
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
