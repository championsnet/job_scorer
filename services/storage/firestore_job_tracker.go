package storage

import (
	"context"
	"fmt"
	"strings"
	"time"

	"cloud.google.com/go/firestore"

	"job-scorer/models"
)

type FirestoreJobTracker struct {
	client    *firestore.Client
	accountID string
}

func NewFirestoreJobTracker(client *firestore.Client, accountID string) *FirestoreJobTracker {
	return &FirestoreJobTracker{client: client, accountID: accountID}
}

func (jt *FirestoreJobTracker) IsProcessed(jobID string) bool {
	if strings.TrimSpace(jobID) == "" {
		return false
	}
	docID := fmt.Sprintf("%s:linkedin:%s", jt.accountID, jobID)
	doc, err := jt.client.Collection("account_job_state").Doc(docID).Get(context.Background())
	return err == nil && doc.Exists()
}

func (jt *FirestoreJobTracker) MarkAsProcessed(jobID string) error {
	return jt.MarkMultipleAsProcessed([]string{jobID})
}

func (jt *FirestoreJobTracker) MarkMultipleAsProcessed(jobIDs []string) error {
	now := time.Now().UTC()
	batch := jt.client.Batch()
	for _, jobID := range jobIDs {
		if strings.TrimSpace(jobID) == "" {
			continue
		}
		docID := fmt.Sprintf("%s:linkedin:%s", jt.accountID, jobID)
		batch.Set(jt.client.Collection("account_job_state").Doc(docID), map[string]interface{}{
			"account_id":      jt.accountID,
			"source":          "linkedin",
			"external_job_id": jobID,
			"first_seen_at":   now,
			"last_seen_at":    now,
		}, firestore.MergeAll)
	}
	_, err := batch.Commit(context.Background())
	return err
}

func (jt *FirestoreJobTracker) FilterProcessedJobs(jobs []*models.Job) []*models.Job {
	filtered := make([]*models.Job, 0, len(jobs))
	for _, job := range jobs {
		if job == nil {
			continue
		}
		if job.JobID == "" || !jt.IsProcessed(job.JobID) {
			filtered = append(filtered, job)
		}
	}
	return filtered
}

func (jt *FirestoreJobTracker) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"storage_type": "firestore",
	}
}

func (jt *FirestoreJobTracker) CleanupOldEntries(_ time.Duration) error {
	return nil
}
