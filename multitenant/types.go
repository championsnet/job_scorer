package multitenant

import (
	"time"

	"job-scorer/config"
	"job-scorer/models"
)

type UserAccount struct {
	UserID        string `json:"userID"`
	AccountID     string `json:"accountID"`
	FirebaseUID   string `json:"firebaseUID"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"emailVerified"`
}

type AccountSettings struct {
	AccountID          string        `json:"accountID"`
	Version            int           `json:"version"`
	Policy             config.Policy `json:"policy"`
	MaxJobs            int           `json:"maxJobs"`
	ScheduleCron       string        `json:"scheduleCron"`
	ScheduleTimezone   string        `json:"scheduleTimezone"`
	ScheduleEnabled    bool          `json:"scheduleEnabled"`
	NotificationEmails []string      `json:"notificationEmails"`
	UpdatedAt          time.Time     `json:"updatedAt"`
}

type AccountCV struct {
	ID            string
	AccountID     string
	StoragePath   string
	MimeType      string
	SizeBytes     int64
	SHA256        string
	ExtractedText string
	CreatedAt     time.Time
}

type RunRequest struct {
	RunID     string `json:"run_id"`
	RequestID string `json:"request_id"`
	Status    string `json:"status"`
}

type BillingSummary struct {
	Balance       int             `json:"balance"`
	Packages      []CreditPackage `json:"packages"`
	LastUpdatedAt time.Time       `json:"last_updated_at"`
}

type DashboardOverview struct {
	Runs struct {
		Total         int   `json:"total"`
		Success       int   `json:"success"`
		Failed        int   `json:"failed"`
		AvgDurationMs int64 `json:"avg_duration_ms"`
	} `json:"runs"`
	Funnel struct {
		AllJobs        int `json:"all_jobs"`
		Prefiltered    int `json:"prefiltered"`
		Evaluated      int `json:"evaluated"`
		Promising      int `json:"promising"`
		FinalEvaluated int `json:"final_evaluated"`
		Notification   int `json:"notification"`
		Validated      int `json:"validated"`
		EmailSent      int `json:"email_sent"`
	} `json:"funnel"`
	LLMUsage struct {
		Calls        int `json:"calls"`
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"llm_usage"`
	RecentRuns []*models.RunSummary `json:"recent_runs"`
}
