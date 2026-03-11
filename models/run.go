package models

import "time"

// RunStatus represents the lifecycle state of a pipeline run
type RunStatus string

const (
	RunStatusRunning RunStatus = "running"
	RunStatusSuccess RunStatus = "success"
	RunStatusFailed  RunStatus = "failed"
)

// RunStageCounts holds per-stage job counts for funnel analytics
type RunStageCounts struct {
	AllJobs           int `json:"all_jobs"`
	Prefiltered       int `json:"prefiltered"`
	Evaluated         int `json:"evaluated"`
	Promising         int `json:"promising"`
	FinalEvaluated    int `json:"final_evaluated"`
	Notification      int `json:"notification"`
	Validated         int `json:"validated"`
	EmailSent         int `json:"email_sent"`
	DuplicatesRemoved int `json:"duplicates_removed,omitempty"`
}

// LLMUsageSnapshot captures token usage at the end of a run
type LLMUsageSnapshot struct {
	Calls                 int `json:"calls"`
	InputTokens           int `json:"input_tokens"`
	CachedInputTokens     int `json:"cached_input_tokens"`
	NonCachedInputTokens  int `json:"non_cached_input_tokens"`
	BillableInputTokens   int `json:"billable_input_tokens"`
	OutputTokens          int `json:"output_tokens"`
	TotalTokens           int `json:"total_tokens"`
}

// RunConfigSnapshot captures config used for this run (for display/audit)
type RunConfigSnapshot struct {
	Locations []string `json:"locations"`
	MaxJobs   int      `json:"max_jobs"`
}

// NotificationResult records which jobs were emailed and to whom
type NotificationResult struct {
	RunID             string   `json:"run_id"`
	JobIDs            []string `json:"job_ids"`
	RecipientsCount   int      `json:"recipients_count"`
	SuccessCount      int      `json:"success_count"`
	FailedCount       int      `json:"failed_count"`
	CompletedAt       string   `json:"completed_at"`
	ErrorMessage      string   `json:"error_message,omitempty"`
}

// RunSummary is the persisted manifest for a pipeline run
type RunSummary struct {
	RunID        string              `json:"run_id"`
	Status       RunStatus           `json:"status"`
	StartedAt    time.Time           `json:"started_at"`
	CompletedAt  *time.Time          `json:"completed_at,omitempty"`
	DurationMs   int64               `json:"duration_ms,omitempty"`
	StageCounts  RunStageCounts      `json:"stage_counts"`
	Config       RunConfigSnapshot   `json:"config"`
	LLMUsage     LLMUsageSnapshot    `json:"llm_usage"`
	Notification *NotificationResult `json:"notification,omitempty"`
	ErrorMessage string              `json:"error_message,omitempty"`
	ForceReeval  bool                `json:"force_reeval,omitempty"`
}
