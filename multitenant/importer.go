package multitenant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cloud.google.com/go/firestore"

	"job-scorer/config"
	"job-scorer/models"
)

func RunImport(ctx context.Context, runtimeCfg *RuntimeConfig) error {
	fs, err := OpenFirestore(ctx, runtimeCfg)
	if err != nil {
		return err
	}
	defer fs.Close()

	repo := NewRepository(fs, runtimeCfg)
	importEmail := strings.TrimSpace(getEnv("IMPORT_ACCOUNT_EMAIL", "imported-user@example.com"))
	importUID := strings.TrimSpace(getEnv("IMPORT_FIREBASE_UID", "import-user"))
	user, err := repo.EnsureUserAccount(ctx, importUID, importEmail, true)
	if err != nil {
		return fmt.Errorf("failed ensuring import user/account: %w", err)
	}

	baseCfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed loading local config for import: %w", err)
	}

	settingsPayload := &AccountSettings{
		Policy:             baseCfg.Policy,
		MaxJobs:            baseCfg.App.MaxJobs,
		ScheduleCron:       baseCfg.App.CronSchedule,
		ScheduleTimezone:   getEnv("IMPORT_SCHEDULE_TIMEZONE", "UTC"),
		ScheduleEnabled:    true,
		NotificationEmails: append([]string{}, baseCfg.SMTP.ToRecipients...),
	}
	if _, err := repo.UpsertAccountSettings(ctx, user.AccountID, user.UserID, settingsPayload); err != nil {
		return fmt.Errorf("failed importing settings: %w", err)
	}

	if err := importCV(runtimeCfg, repo, user.AccountID, baseCfg.App.CVPath); err != nil {
		return fmt.Errorf("failed importing CV: %w", err)
	}
	if err := importProcessedJobIDs(ctx, fs, user.AccountID, getEnv("IMPORT_PROCESSED_IDS_PATH", "data/processed_job_ids.json")); err != nil {
		return fmt.Errorf("failed importing processed job IDs: %w", err)
	}
	if err := importRunHistory(ctx, fs, user.AccountID, getEnv("IMPORT_CHECKPOINT_DIR", "data/checkpoints")); err != nil {
		return fmt.Errorf("failed importing run history: %w", err)
	}
	return nil
}

func stageFromCheckpointFilename(filename string) string {
	name := strings.TrimSuffix(filename, ".json")
	if !strings.HasPrefix(name, "checkpoint_") {
		return ""
	}
	rest := strings.TrimPrefix(name, "checkpoint_")
	parts := strings.Split(rest, "_")
	if len(parts) <= 1 {
		return rest
	}
	return strings.Join(parts[:len(parts)-1], "_")
}

func importCV(runtimeCfg *RuntimeConfig, repo *Repository, accountID, cvPath string) error {
	cvPath = strings.TrimSpace(cvPath)
	if cvPath == "" {
		return nil
	}
	body, err := os.ReadFile(cvPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if len(body) == 0 {
		return nil
	}

	store, err := NewObjectStorage(runtimeCfg)
	if err != nil {
		return err
	}
	defer store.Close()

	objectPath, sha, err := SaveAccountCV(store, runtimeCfg, accountID, filepath.Base(cvPath), body)
	if err != nil {
		return err
	}
	_, err = repo.SaveAccountCV(context.Background(), accountID, objectPath, "text/plain", sha, "", int64(len(body)))
	return err
}

func importProcessedJobIDs(ctx context.Context, fsClient *firestore.Client, accountID, filePath string) error {
	body, err := os.ReadFile(filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	var raw map[string]string
	if err := json.Unmarshal(body, &raw); err != nil {
		return err
	}
	batch := fsClient.Batch()
	now := time.Now().UTC()
	for id := range raw {
		if strings.TrimSpace(id) == "" {
			continue
		}
		docID := fmt.Sprintf("%s:linkedin:%s", accountID, id)
		batch.Set(fsClient.Collection("account_job_state").Doc(docID), map[string]interface{}{
			"account_id":      accountID,
			"source":          "linkedin",
			"external_job_id": id,
			"first_seen_at":   now,
			"last_seen_at":    now,
		})
	}
	_, err = batch.Commit(ctx)
	return err
}

func importRunHistory(ctx context.Context, fsClient *firestore.Client, accountID, checkpointsRoot string) error {
	entries, err := os.ReadDir(checkpointsRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		runID := entry.Name()
		runDir := filepath.Join(checkpointsRoot, runID)
		summaryPath := filepath.Join(runDir, "run_summary.json")
		body, err := os.ReadFile(summaryPath)
		if err != nil {
			continue
		}
		var summary models.RunSummary
		if err := json.Unmarshal(body, &summary); err != nil {
			continue
		}
		stageJSON, _ := json.Marshal(summary.StageCounts)
		usageJSON, _ := json.Marshal(summary.LLMUsage)
		configJSON, _ := json.Marshal(summary.Config)
		startedAt := summary.StartedAt
		if startedAt.IsZero() {
			startedAt = time.Now().UTC()
		}

		_, err = fsClient.Collection("runs").Doc(runID).Set(ctx, map[string]interface{}{
			"id":                runID,
			"account_id":        accountID,
			"trigger_type":      "import",
			"status":            string(summary.Status),
			"request_id":        "import-" + runID,
			"settings_version":  1,
			"settings_snapshot": "{}",
			"config_snapshot":   string(configJSON),
			"stage_counts":      string(stageJSON),
			"llm_usage":         string(usageJSON),
			"started_at":        startedAt,
			"completed_at":      summary.CompletedAt,
			"duration_ms":       summary.DurationMs,
			"error_message":     summary.ErrorMessage,
			"created_at":        time.Now().UTC(),
			"updated_at":        time.Now().UTC(),
		}, firestore.MergeAll)
		if err != nil {
			return err
		}

		files, err := os.ReadDir(runDir)
		if err != nil {
			return err
		}
		for _, file := range files {
			if file.IsDir() {
				continue
			}
			name := file.Name()
			if !strings.HasPrefix(name, "checkpoint_") || !strings.HasSuffix(name, ".json") {
				continue
			}
			stage := stageFromCheckpointFilename(name)
			if stage == "" {
				continue
			}
			fileBody, err := os.ReadFile(filepath.Join(runDir, name))
			if err != nil {
				continue
			}
			var payload struct {
				Metadata map[string]interface{} `json:"metadata"`
				Jobs     []*models.Job          `json:"jobs"`
			}
			if err := json.Unmarshal(fileBody, &payload); err != nil {
				continue
			}
			jobsJSON, _ := json.Marshal(payload.Jobs)
			metaJSON, _ := json.Marshal(payload.Metadata)
			_, err = fsClient.Collection("runs").Doc(runID).Collection("stages").Doc(stage).Set(ctx, map[string]interface{}{
				"run_id":     runID,
				"account_id": accountID,
				"stage":      stage,
				"job_count":  len(payload.Jobs),
				"metadata":   string(metaJSON),
				"jobs_json":  string(jobsJSON),
				"updated_at": time.Now().UTC(),
			})
			if err != nil {
				return err
			}
		}
	}
	return nil
}
