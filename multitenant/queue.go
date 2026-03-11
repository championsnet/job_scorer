package multitenant

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	cloudtasks "cloud.google.com/go/cloudtasks/apiv2"
	taskspb "cloud.google.com/go/cloudtasks/apiv2/cloudtaskspb"
)

type RunEnqueuer interface {
	EnqueueRun(ctx context.Context, runID string) error
	Close() error
}

type cloudTasksEnqueuer struct {
	client *cloudtasks.Client
	cfg    *RuntimeConfig
}

func NewRunEnqueuer(ctx context.Context, cfg *RuntimeConfig) (RunEnqueuer, error) {
	client, err := cloudtasks.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed creating Cloud Tasks client: %w", err)
	}
	return &cloudTasksEnqueuer{
		client: client,
		cfg:    cfg,
	}, nil
}

func (q *cloudTasksEnqueuer) EnqueueRun(ctx context.Context, runID string) error {
	queuePath := fmt.Sprintf("projects/%s/locations/%s/queues/%s", q.cfg.CloudTasksProjectID, q.cfg.CloudTasksLocation, q.cfg.CloudTasksQueue)

	payload, err := json.Marshal(map[string]string{"run_id": runID})
	if err != nil {
		return err
	}

	req := &taskspb.CreateTaskRequest{
		Parent: queuePath,
		Task: &taskspb.Task{
			MessageType: &taskspb.Task_HttpRequest{
				HttpRequest: &taskspb.HttpRequest{
					HttpMethod: taskspb.HttpMethod_POST,
					Url:        strings.TrimRight(q.cfg.CloudTasksWorkerURL, "/") + "/internal/tasks/run",
					Headers: map[string]string{
						"Content-Type":   "application/json",
						"X-Worker-Token": q.cfg.WorkerToken,
					},
					Body: payload,
				},
			},
		},
	}

	if q.cfg.CloudTasksServiceAcct != "" {
		req.Task.GetHttpRequest().AuthorizationHeader = &taskspb.HttpRequest_OidcToken{
			OidcToken: &taskspb.OidcToken{
				ServiceAccountEmail: q.cfg.CloudTasksServiceAcct,
				Audience:            strings.TrimRight(q.cfg.CloudTasksWorkerURL, "/"),
			},
		}
		delete(req.Task.GetHttpRequest().Headers, "X-Worker-Token")
	}

	_, err = q.client.CreateTask(ctx, req)
	if err != nil {
		return fmt.Errorf("failed creating cloud task for run %s: %w", runID, err)
	}
	return nil
}

func (q *cloudTasksEnqueuer) Close() error {
	if q.client == nil {
		return nil
	}
	return q.client.Close()
}

type directWorkerEnqueuer struct {
	workerURL   string
	workerToken string
	client      *http.Client
}

func NewDirectWorkerEnqueuer(workerURL, workerToken string) RunEnqueuer {
	return &directWorkerEnqueuer{
		workerURL:   strings.TrimRight(workerURL, "/"),
		workerToken: workerToken,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (q *directWorkerEnqueuer) EnqueueRun(ctx context.Context, runID string) error {
	body, err := json.Marshal(map[string]string{"run_id": runID})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, q.workerURL+"/internal/tasks/run", strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Worker-Token", q.workerToken)
	resp, err := q.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("direct worker enqueue failed with status %d", resp.StatusCode)
	}
	return nil
}

func (q *directWorkerEnqueuer) Close() error {
	return nil
}
