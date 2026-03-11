package storage

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"

	"job-scorer/models"
)

type FirestoreRunSummaryStore struct {
	client    *firestore.Client
	accountID string
}

func NewFirestoreRunSummaryStore(client *firestore.Client, accountID string) *FirestoreRunSummaryStore {
	return &FirestoreRunSummaryStore{client: client, accountID: accountID}
}

func (s *FirestoreRunSummaryStore) SaveRunSummary(summary *models.RunSummary) error {
	stageJSON, _ := json.Marshal(summary.StageCounts)
	usageJSON, _ := json.Marshal(summary.LLMUsage)
	configJSON, _ := json.Marshal(summary.Config)
	_, err := s.client.Collection("runs").Doc(summary.RunID).Set(context.Background(), map[string]interface{}{
		"status":          string(summary.Status),
		"started_at":      summary.StartedAt,
		"completed_at":    summary.CompletedAt,
		"duration_ms":     summary.DurationMs,
		"stage_counts":    string(stageJSON),
		"llm_usage":       string(usageJSON),
		"config_snapshot": string(configJSON),
		"error_message":   summary.ErrorMessage,
		"updated_at":      time.Now().UTC(),
	}, firestore.MergeAll)
	return err
}

func (s *FirestoreRunSummaryStore) LoadRunSummary(runID string) (*models.RunSummary, error) {
	doc, err := s.client.Collection("runs").Doc(runID).Get(context.Background())
	if err != nil {
		return nil, nil
	}
	if toString(doc.Data()["account_id"], "") != s.accountID {
		return nil, nil
	}
	return summaryFromDoc(runID, doc.Data()), nil
}

func (s *FirestoreRunSummaryStore) ListRunSummaries(limit int) ([]*models.RunSummary, error) {
	if limit <= 0 {
		limit = 50
	}
	iter := s.client.Collection("runs").
		Where("account_id", "==", s.accountID).
		OrderBy("created_at", firestore.Desc).
		Limit(limit).
		Documents(context.Background())
	out := make([]*models.RunSummary, 0, limit)
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		out = append(out, summaryFromDoc(doc.Ref.ID, doc.Data()))
	}
	return out, nil
}

func summaryFromDoc(runID string, data map[string]interface{}) *models.RunSummary {
	summary := &models.RunSummary{
		RunID:        runID,
		Status:       models.RunStatus(toString(data["status"], "queued")),
		DurationMs:   toInt64(data["duration_ms"]),
		ErrorMessage: toString(data["error_message"], ""),
		StartedAt:    toTime(data["started_at"]),
	}
	completed := toTime(data["completed_at"])
	if !completed.IsZero() {
		summary.CompletedAt = &completed
	}
	_ = json.Unmarshal([]byte(toString(data["stage_counts"], "{}")), &summary.StageCounts)
	_ = json.Unmarshal([]byte(toString(data["llm_usage"], "{}")), &summary.LLMUsage)
	_ = json.Unmarshal([]byte(toString(data["config_snapshot"], "{}")), &summary.Config)
	return summary
}

func toInt64(v interface{}) int64 {
	switch t := v.(type) {
	case int64:
		return t
	case int:
		return int64(t)
	case float64:
		return int64(t)
	default:
		return 0
	}
}

func toTime(v interface{}) time.Time {
	switch t := v.(type) {
	case time.Time:
		return t
	case *time.Time:
		if t != nil {
			return *t
		}
	case string:
		if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(t)); err == nil {
			return parsed
		}
	}
	return time.Time{}
}
