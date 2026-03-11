package storage

import (
	"context"
	"encoding/json"
	"time"

	"cloud.google.com/go/firestore"

	"job-scorer/models"
)

type FirestoreJobStorage struct {
	client    *firestore.Client
	accountID string
}

func NewFirestoreJobStorage(client *firestore.Client, accountID string) *FirestoreJobStorage {
	return &FirestoreJobStorage{client: client, accountID: accountID}
}

func (s *FirestoreJobStorage) SaveAllJobs(jobs []*models.Job) error {
	return s.save("all_jobs", jobs)
}

func (s *FirestoreJobStorage) SavePromisingJobs(jobs []*models.Job) error {
	return s.save("promising", jobs)
}

func (s *FirestoreJobStorage) SaveFinalEvaluatedJobs(jobs []*models.Job) error {
	return s.save("final_evaluated", jobs)
}

func (s *FirestoreJobStorage) LoadAllJobs() ([]*models.Job, error) {
	return s.load("all_jobs")
}

func (s *FirestoreJobStorage) LoadPromisingJobs() ([]*models.Job, error) {
	return s.load("promising")
}

func (s *FirestoreJobStorage) LoadFinalEvaluatedJobs() ([]*models.Job, error) {
	return s.load("final_evaluated")
}

func (s *FirestoreJobStorage) FileExists(_ string) bool {
	return true
}

func (s *FirestoreJobStorage) DeleteFile(_ string) error {
	return nil
}

func (s *FirestoreJobStorage) GetOutputDir() string {
	return "firestore://run_snapshots"
}

func (s *FirestoreJobStorage) EnsureOutputDir() error {
	return nil
}

func (s *FirestoreJobStorage) save(stage string, jobs []*models.Job) error {
	payload, _ := json.Marshal(jobs)
	_, err := s.client.Collection("account_latest_outputs").Doc(s.accountID+"_"+stage).Set(context.Background(), map[string]interface{}{
		"account_id": s.accountID,
		"stage":      stage,
		"jobs_json":  string(payload),
		"updated_at": time.Now().UTC(),
	})
	return err
}

func (s *FirestoreJobStorage) load(stage string) ([]*models.Job, error) {
	doc, err := s.client.Collection("account_latest_outputs").Doc(s.accountID + "_" + stage).Get(context.Background())
	if err != nil {
		return []*models.Job{}, nil
	}
	var jobs []*models.Job
	_ = json.Unmarshal([]byte(toString(doc.Data()["jobs_json"], "[]")), &jobs)
	return jobs, nil
}
