package storage

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"cloud.google.com/go/firestore"

	"job-scorer/models"
)

type FirestoreCheckpointService struct {
	client    *firestore.Client
	accountID string
	runFolder string
}

func NewFirestoreCheckpointService(client *firestore.Client, accountID string) *FirestoreCheckpointService {
	return &FirestoreCheckpointService{
		client:    client,
		accountID: accountID,
	}
}

func (cs *FirestoreCheckpointService) SetRunFolder(folder string) {
	cs.runFolder = strings.TrimSpace(folder)
}

func (cs *FirestoreCheckpointService) GetRunFolder() string {
	if cs.runFolder != "" {
		return cs.runFolder
	}
	return time.Now().Format("2006-01-02_15-04-05")
}

func (cs *FirestoreCheckpointService) SaveCheckpoint(jobs []*models.Job, stage string, metadata map[string]interface{}) error {
	jobsJSON, _ := json.Marshal(jobs)
	metaJSON, _ := json.Marshal(metadata)
	_, err := cs.client.Collection("runs").Doc(cs.GetRunFolder()).Collection("stages").Doc(stage).Set(
		context.Background(),
		map[string]interface{}{
			"run_id":     cs.GetRunFolder(),
			"account_id": cs.accountID,
			"stage":      stage,
			"job_count":  len(jobs),
			"metadata":   string(metaJSON),
			"jobs_json":  string(jobsJSON),
			"updated_at": time.Now().UTC(),
		},
	)
	return err
}

func (cs *FirestoreCheckpointService) SaveDailySnapshot(allJobs, promisingJobs, finalJobs []*models.Job) error {
	return cs.SaveCheckpoint(finalJobs, "daily_snapshot", map[string]interface{}{
		"all_jobs_count":       len(allJobs),
		"promising_jobs_count": len(promisingJobs),
		"final_jobs_count":     len(finalJobs),
	})
}

func (cs *FirestoreCheckpointService) ListCheckpoints() (map[string][]string, error) {
	return map[string][]string{}, nil
}

func (cs *FirestoreCheckpointService) LoadCheckpoint(dateFolder, filename string) ([]*models.Job, error) {
	stage := stageFromCheckpointFilename(filename)
	if stage == "" {
		return nil, nil
	}
	return cs.LoadCheckpointByStage(dateFolder, stage)
}

func (cs *FirestoreCheckpointService) LoadCheckpointByStage(runID, stage string) ([]*models.Job, error) {
	doc, err := cs.client.Collection("runs").Doc(runID).Collection("stages").Doc(stage).Get(context.Background())
	if err != nil {
		return nil, nil
	}
	if toString(doc.Data()["account_id"], "") != cs.accountID {
		return nil, nil
	}
	var jobs []*models.Job
	if err := json.Unmarshal([]byte(toString(doc.Data()["jobs_json"], "[]")), &jobs); err != nil {
		return nil, err
	}
	return jobs, nil
}

func (cs *FirestoreCheckpointService) LoadCheckpointLegacy(filename string) ([]*models.Job, error) {
	stage := stageFromCheckpointFilename(filename)
	if stage == "" {
		return nil, nil
	}
	return cs.LoadCheckpointByStage(cs.GetRunFolder(), stage)
}

func (cs *FirestoreCheckpointService) CleanupOldCheckpoints(daysToKeep int) error {
	if daysToKeep <= 0 {
		return nil
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

func toString(v interface{}, fallback string) string {
	if v == nil {
		return fallback
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fallback
}
