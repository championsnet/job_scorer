package multitenant

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	_ "time/tzdata"

	"cloud.google.com/go/firestore"
	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
	"google.golang.org/api/iterator"

	"job-scorer/config"
	"job-scorer/models"
)

const (
	runStatusQueued  = "queued"
	runStatusRunning = "running"
	runStatusSuccess = "success"
	runStatusFailed  = "failed"
)

type Repository struct {
	fs     *firestore.Client
	config *RuntimeConfig
}

func NewRepository(fs *firestore.Client, cfg *RuntimeConfig) *Repository {
	return &Repository{fs: fs, config: cfg}
}

func (r *Repository) EnsureUserAccount(ctx context.Context, firebaseUID, email string, emailVerified bool) (*UserAccount, error) {
	firebaseUID = strings.TrimSpace(firebaseUID)
	email = strings.TrimSpace(strings.ToLower(email))
	if firebaseUID == "" || email == "" {
		return nil, fmt.Errorf("firebase UID and email are required")
	}

	userRef := r.fs.Collection("users").Doc(firebaseUID)
	userSnap, err := userRef.Get(ctx)
	if err == nil && userSnap.Exists() {
		accountID, _ := userSnap.Data()["account_id"].(string)
		_, err = userRef.Set(ctx, map[string]interface{}{
			"email":          email,
			"email_verified": emailVerified,
			"last_login_at":  time.Now().UTC(),
			"updated_at":     time.Now().UTC(),
		}, firestore.MergeAll)
		if err != nil {
			return nil, err
		}
		return &UserAccount{
			UserID:        firebaseUID,
			AccountID:     accountID,
			FirebaseUID:   firebaseUID,
			Email:         email,
			EmailVerified: emailVerified,
		}, nil
	}
	if err != nil && strings.Contains(err.Error(), "NotFound") == false {
		return nil, err
	}

	accountID := uuid.NewString()
	now := time.Now().UTC()
	accountName := strings.Split(email, "@")[0]
	if accountName == "" {
		accountName = "User account"
	}

	defaultPolicy := config.DefaultPolicy()
	policyJSON, _ := json.Marshal(defaultPolicy)

	batch := r.fs.Batch()
	batch.Set(r.fs.Collection("accounts").Doc(accountID), map[string]interface{}{
		"name":             accountName,
		"timezone":         "UTC",
		"schedule_cron":    "0 */1 * * *",
		"schedule_enabled": false,
		"next_run_at":      nil,
		"last_run_at":      nil,
		"active_run_id":    "",
		"credit_balance":   0,
		"created_at":       now,
		"updated_at":       now,
	})
	batch.Set(r.fs.Collection("account_settings").Doc(accountID), map[string]interface{}{
		"account_id":          accountID,
		"policy_json":         string(policyJSON),
		"max_jobs":            r.config.DefaultMaxJobs,
		"schedule_cron":       "0 */1 * * *",
		"schedule_timezone":   "UTC",
		"schedule_enabled":    false,
		"notification_emails": []string{email},
		"version":             1,
		"created_at":          now,
		"updated_at":          now,
	})
	batch.Set(r.fs.Collection("account_settings_versions").NewDoc(), map[string]interface{}{
		"account_id":          accountID,
		"version":             1,
		"policy_json":         string(policyJSON),
		"max_jobs":            r.config.DefaultMaxJobs,
		"schedule_cron":       "0 */1 * * *",
		"schedule_timezone":   "UTC",
		"schedule_enabled":    false,
		"notification_emails": []string{email},
		"changed_by_user_id":  nil,
		"created_at":          now,
	})
	batch.Set(userRef, map[string]interface{}{
		"id":             firebaseUID,
		"account_id":     accountID,
		"firebase_uid":   firebaseUID,
		"email":          email,
		"email_verified": emailVerified,
		"status":         "active",
		"last_login_at":  now,
		"created_at":     now,
		"updated_at":     now,
	})
	if _, err := batch.Commit(ctx); err != nil {
		return nil, err
	}

	return &UserAccount{
		UserID:        firebaseUID,
		AccountID:     accountID,
		FirebaseUID:   firebaseUID,
		Email:         email,
		EmailVerified: emailVerified,
	}, nil
}

func (r *Repository) GetAccountSettings(ctx context.Context, accountID string) (*AccountSettings, error) {
	snap, err := r.fs.Collection("account_settings").Doc(accountID).Get(ctx)
	if err != nil {
		return nil, err
	}
	data := snap.Data()
	settings := &AccountSettings{
		AccountID:        accountID,
		Version:          toInt(data["version"], 1),
		MaxJobs:          toInt(data["max_jobs"], r.config.DefaultMaxJobs),
		ScheduleCron:     toString(data["schedule_cron"], "0 */1 * * *"),
		ScheduleTimezone: toString(data["schedule_timezone"], "UTC"),
		ScheduleEnabled:  toBool(data["schedule_enabled"], false),
		UpdatedAt:        toTime(data["updated_at"]),
	}
	settings.NotificationEmails = toStringSlice(data["notification_emails"])
	policyText := toString(data["policy_json"], "")
	if policyText == "" {
		settings.Policy = config.DefaultPolicy()
		return settings, nil
	}
	if err := json.Unmarshal([]byte(policyText), &settings.Policy); err != nil {
		return nil, err
	}
	return settings, nil
}

func (r *Repository) UpsertAccountSettings(ctx context.Context, accountID, userID string, incoming *AccountSettings) (*AccountSettings, error) {
	if incoming == nil {
		return nil, fmt.Errorf("settings payload is required")
	}
	policyJSON, err := json.Marshal(incoming.Policy)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	nextRunAt, err := computeNextRunAt(incoming.ScheduleCron, incoming.ScheduleTimezone, now)
	if err != nil {
		return nil, err
	}
	if !incoming.ScheduleEnabled {
		nextRunAt = time.Time{}
	}

	setRef := r.fs.Collection("account_settings").Doc(accountID)
	version := 1
	err = r.fs.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		snap, err := tx.Get(setRef)
		if err == nil && snap.Exists() {
			version = toInt(snap.Data()["version"], 1) + 1
		}
		if err := tx.Set(setRef, map[string]interface{}{
			"account_id":          accountID,
			"policy_json":         string(policyJSON),
			"max_jobs":            incoming.MaxJobs,
			"schedule_cron":       incoming.ScheduleCron,
			"schedule_timezone":   incoming.ScheduleTimezone,
			"schedule_enabled":    incoming.ScheduleEnabled,
			"notification_emails": incoming.NotificationEmails,
			"version":             version,
			"updated_at":          now,
		}, firestore.MergeAll); err != nil {
			return err
		}
		if err := tx.Set(r.fs.Collection("account_settings_versions").NewDoc(), map[string]interface{}{
			"account_id":          accountID,
			"version":             version,
			"policy_json":         string(policyJSON),
			"max_jobs":            incoming.MaxJobs,
			"schedule_cron":       incoming.ScheduleCron,
			"schedule_timezone":   incoming.ScheduleTimezone,
			"schedule_enabled":    incoming.ScheduleEnabled,
			"notification_emails": incoming.NotificationEmails,
			"changed_by_user_id":  userID,
			"created_at":          now,
		}); err != nil {
			return err
		}
		accountUpdate := map[string]interface{}{
			"schedule_cron":    incoming.ScheduleCron,
			"timezone":         incoming.ScheduleTimezone,
			"schedule_enabled": incoming.ScheduleEnabled,
			"updated_at":       now,
		}
		if incoming.ScheduleEnabled {
			accountUpdate["next_run_at"] = nextRunAt
		} else {
			accountUpdate["next_run_at"] = nil
		}
		return tx.Set(r.fs.Collection("accounts").Doc(accountID), accountUpdate, firestore.MergeAll)
	})
	if err != nil {
		return nil, err
	}

	out := *incoming
	out.AccountID = accountID
	out.Version = version
	out.UpdatedAt = now
	return &out, nil
}

func (r *Repository) SaveAccountCV(ctx context.Context, accountID, storagePath, mimeType, sha256, extractedText string, sizeBytes int64) (*AccountCV, error) {
	now := time.Now().UTC()
	iter := r.fs.Collection("account_files").
		Where("account_id", "==", accountID).
		Where("file_kind", "==", "cv").
		Where("is_active", "==", true).
		Documents(ctx)
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		_, _ = doc.Ref.Set(ctx, map[string]interface{}{"is_active": false, "updated_at": now}, firestore.MergeAll)
	}
	doc := r.fs.Collection("account_files").NewDoc()
	data := map[string]interface{}{
		"id":             doc.ID,
		"account_id":     accountID,
		"file_kind":      "cv",
		"storage_path":   storagePath,
		"mime_type":      mimeType,
		"size_bytes":     sizeBytes,
		"sha256":         sha256,
		"extracted_text": extractedText,
		"is_active":      true,
		"created_at":     now,
		"updated_at":     now,
	}
	if _, err := doc.Set(ctx, data); err != nil {
		return nil, err
	}
	return &AccountCV{
		ID:            doc.ID,
		AccountID:     accountID,
		StoragePath:   storagePath,
		MimeType:      mimeType,
		SizeBytes:     sizeBytes,
		SHA256:        sha256,
		ExtractedText: extractedText,
		CreatedAt:     now,
	}, nil
}

func (r *Repository) GetActiveCV(ctx context.Context, accountID string) (*AccountCV, error) {
	iter := r.fs.Collection("account_files").
		Where("account_id", "==", accountID).
		Where("file_kind", "==", "cv").
		Where("is_active", "==", true).
		Limit(20).
		Documents(ctx)
	var latest *AccountCV
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		data := doc.Data()
		current := &AccountCV{
			ID:            doc.Ref.ID,
			AccountID:     accountID,
			StoragePath:   toString(data["storage_path"], ""),
			MimeType:      toString(data["mime_type"], ""),
			SizeBytes:     toInt64(data["size_bytes"]),
			SHA256:        toString(data["sha256"], ""),
			ExtractedText: toString(data["extracted_text"], ""),
			CreatedAt:     toTime(data["created_at"]),
		}
		if latest == nil || current.CreatedAt.After(latest.CreatedAt) {
			latest = current
		}
	}
	return latest, nil
}

func (r *Repository) CreateQueuedRun(ctx context.Context, accountID string, requestedByUserID *string, triggerType, runKey string, settings *AccountSettings, creditCost int, forceReeval bool) (*RunRequest, error) {
	if creditCost <= 0 {
		creditCost = 1
	}
	runID := uuid.NewString()
	requestID := uuid.NewString()
	now := time.Now().UTC()
	settingsJSON, _ := json.Marshal(map[string]interface{}{
		"version":             settings.Version,
		"max_jobs":            settings.MaxJobs,
		"schedule_cron":       settings.ScheduleCron,
		"schedule_timezone":   settings.ScheduleTimezone,
		"schedule_enabled":    settings.ScheduleEnabled,
		"notification_emails": settings.NotificationEmails,
		"policy":              settings.Policy,
	})

	err := r.fs.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		accountRef := r.fs.Collection("accounts").Doc(accountID)
		accountSnap, err := tx.Get(accountRef)
		if err != nil {
			return err
		}
		balance := toInt(accountSnap.Data()["credit_balance"], 0)
		activeRunID := toString(accountSnap.Data()["active_run_id"], "")
		if activeRunID != "" {
			activeRunRef := r.fs.Collection("runs").Doc(activeRunID)
			activeRunSnap, activeErr := tx.Get(activeRunRef)
			if activeErr == nil {
				activeStatus := toString(activeRunSnap.Data()["status"], runStatusQueued)
				if activeStatus == runStatusQueued || activeStatus == runStatusRunning {
					return fmt.Errorf("an active run already exists for this account")
				}
				// Stale lock: account points to a completed run; clear it in this transaction.
				tx.Set(accountRef, map[string]interface{}{
					"active_run_id": "",
					"updated_at":    now,
				}, firestore.MergeAll)
			} else if !strings.Contains(strings.ToLower(activeErr.Error()), "notfound") {
				return activeErr
			}
		}
		if balance < creditCost {
			return fmt.Errorf("insufficient credits: available=%d required=%d", balance, creditCost)
		}

		runRef := r.fs.Collection("runs").Doc(runID)
		tx.Set(runRef, map[string]interface{}{
			"id":                    runID,
			"account_id":            accountID,
			"trigger_type":          triggerType,
			"status":                runStatusQueued,
			"requested_by_user_id":  safePtrValue(requestedByUserID),
			"run_key":               runKey,
			"request_id":            requestID,
			"settings_version":      settings.Version,
			"settings_snapshot":     string(settingsJSON),
			"config_snapshot":       "{}",
			"stage_counts":          "{}",
			"llm_usage":             "{}",
			"started_at":            nil,
			"completed_at":          nil,
			"duration_ms":           int64(0),
			"error_message":         "",
			"force_reeval":          forceReeval,
			"credit_reservation_id": runID,
			"reserved_credits":      creditCost,
			"reservation_status":    "reserved",
			"created_at":            now,
			"updated_at":            now,
		})

		tx.Set(accountRef, map[string]interface{}{
			"credit_balance": balance - creditCost,
			"active_run_id":  runID,
			"updated_at":     now,
		}, firestore.MergeAll)

		tx.Set(r.fs.Collection("credit_ledger").NewDoc(), map[string]interface{}{
			"account_id": accountID,
			"event_type": "reserve",
			"credits":    -creditCost,
			"reference":  "run:" + runID,
			"run_id":     runID,
			"created_at": now,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &RunRequest{RunID: runID, RequestID: requestID, Status: runStatusQueued}, nil
}

func (r *Repository) MarkRunRunning(ctx context.Context, runID string) error {
	_, err := r.fs.Collection("runs").Doc(runID).Set(ctx, map[string]interface{}{
		"status":     runStatusRunning,
		"started_at": time.Now().UTC(),
		"updated_at": time.Now().UTC(),
	}, firestore.MergeAll)
	return err
}

func (r *Repository) SaveRunSummary(ctx context.Context, accountID string, summary *models.RunSummary) error {
	now := time.Now().UTC()
	stageJSON, _ := json.Marshal(summary.StageCounts)
	usageJSON, _ := json.Marshal(summary.LLMUsage)
	configJSON, _ := json.Marshal(summary.Config)
	status := string(summary.Status)
	if status == "" {
		status = runStatusFailed
	}
	return r.fs.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		runRef := r.fs.Collection("runs").Doc(summary.RunID)
		runSnap, err := tx.Get(runRef)
		if err != nil {
			return err
		}
		runData := runSnap.Data()
		reserved := toInt(runData["reserved_credits"], 0)
		reservationStatus := toString(runData["reservation_status"], "")

		tx.Set(runRef, map[string]interface{}{
			"status":          status,
			"started_at":      summary.StartedAt,
			"completed_at":    summary.CompletedAt,
			"duration_ms":     summary.DurationMs,
			"stage_counts":    string(stageJSON),
			"llm_usage":       string(usageJSON),
			"config_snapshot": string(configJSON),
			"error_message":   summary.ErrorMessage,
			"updated_at":      now,
		}, firestore.MergeAll)

		accountRef := r.fs.Collection("accounts").Doc(accountID)
		accountUpdates := map[string]interface{}{
			"last_run_at":   now,
			"active_run_id": "",
			"updated_at":    now,
		}

		if status == runStatusSuccess {
			tx.Set(runRef, map[string]interface{}{"reservation_status": "consumed"}, firestore.MergeAll)
		} else if reserved > 0 && reservationStatus == "reserved" {
			accountSnap, err := tx.Get(accountRef)
			if err == nil {
				balance := toInt(accountSnap.Data()["credit_balance"], 0)
				accountUpdates["credit_balance"] = balance + reserved
				tx.Set(r.fs.Collection("credit_ledger").NewDoc(), map[string]interface{}{
					"account_id": accountID,
					"event_type": "refund",
					"credits":    reserved,
					"reference":  "run-failure-refund:" + summary.RunID,
					"run_id":     summary.RunID,
					"created_at": now,
				})
				tx.Set(runRef, map[string]interface{}{"reservation_status": "refunded"}, firestore.MergeAll)
			}
		}
		tx.Set(accountRef, accountUpdates, firestore.MergeAll)
		return nil
	})
}

func (r *Repository) SaveStageSnapshot(ctx context.Context, accountID, runID, stage string, jobs []*models.Job, metadata map[string]interface{}) error {
	jobsJSON, _ := json.Marshal(jobs)
	metaJSON, _ := json.Marshal(metadata)
	_, err := r.fs.Collection("runs").Doc(runID).Collection("stages").Doc(stage).Set(ctx, map[string]interface{}{
		"run_id":     runID,
		"account_id": accountID,
		"stage":      stage,
		"job_count":  len(jobs),
		"metadata":   string(metaJSON),
		"jobs_json":  string(jobsJSON),
		"updated_at": time.Now().UTC(),
	})
	return err
}

func (r *Repository) LoadStageSnapshot(ctx context.Context, accountID, runID, stage string) ([]*models.Job, error) {
	doc, err := r.fs.Collection("runs").Doc(runID).Collection("stages").Doc(stage).Get(ctx)
	if err != nil {
		return nil, nil
	}
	if toString(doc.Data()["account_id"], "") != accountID {
		return nil, nil
	}
	var jobs []*models.Job
	if err := json.Unmarshal([]byte(toString(doc.Data()["jobs_json"], "[]")), &jobs); err != nil {
		return nil, err
	}
	return jobs, nil
}

func (r *Repository) ListRuns(ctx context.Context, accountID string, limit int) ([]*models.RunSummary, error) {
	if limit <= 0 {
		limit = 50
	}
	iter := r.fs.Collection("runs").
		Where("account_id", "==", accountID).
		OrderBy("created_at", firestore.Desc).
		Limit(limit).
		Documents(ctx)
	return collectRunSummaries(iter)
}

func (r *Repository) GetRun(ctx context.Context, accountID, runID string) (*models.RunSummary, error) {
	doc, err := r.fs.Collection("runs").Doc(runID).Get(ctx)
	if err != nil {
		return nil, nil
	}
	if toString(doc.Data()["account_id"], "") != accountID {
		return nil, nil
	}
	return runSummaryFromMap(doc.Ref.ID, doc.Data()), nil
}

func (r *Repository) GetRunByID(ctx context.Context, runID string) (*models.RunSummary, string, error) {
	doc, err := r.fs.Collection("runs").Doc(runID).Get(ctx)
	if err != nil {
		return nil, "", nil
	}
	accountID := toString(doc.Data()["account_id"], "")
	return runSummaryFromMap(runID, doc.Data()), accountID, nil
}

func (r *Repository) GetRunByRequestID(ctx context.Context, accountID, requestID string) (*RunRequest, error) {
	iter := r.fs.Collection("runs").
		Where("account_id", "==", accountID).
		Where("request_id", "==", requestID).
		Limit(1).
		Documents(ctx)
	doc, err := iter.Next()
	if err != nil {
		return nil, nil
	}
	return &RunRequest{
		RunID:     doc.Ref.ID,
		RequestID: requestID,
		Status:    toString(doc.Data()["status"], runStatusQueued),
	}, nil
}

func (r *Repository) GetAnalyticsOverview(ctx context.Context, accountID string) (*DashboardOverview, error) {
	runs, err := r.ListRuns(ctx, accountID, 100)
	if err != nil {
		return nil, err
	}
	data := &DashboardOverview{RecentRuns: runs}
	var totalDuration int64
	for _, run := range runs {
		data.Runs.Total++
		if run.Status == models.RunStatusSuccess {
			data.Runs.Success++
		}
		if run.Status == models.RunStatusFailed {
			data.Runs.Failed++
		}
		totalDuration += run.DurationMs
		data.Funnel.AllJobs += run.StageCounts.AllJobs
		data.Funnel.Prefiltered += run.StageCounts.Prefiltered
		data.Funnel.Evaluated += run.StageCounts.Evaluated
		data.Funnel.Promising += run.StageCounts.Promising
		data.Funnel.FinalEvaluated += run.StageCounts.FinalEvaluated
		data.Funnel.Notification += run.StageCounts.Notification
		data.Funnel.Validated += run.StageCounts.Validated
		data.Funnel.EmailSent += run.StageCounts.EmailSent
		data.LLMUsage.Calls += run.LLMUsage.Calls
		data.LLMUsage.InputTokens += run.LLMUsage.InputTokens
		data.LLMUsage.OutputTokens += run.LLMUsage.OutputTokens
	}
	if data.Runs.Total > 0 {
		data.Runs.AvgDurationMs = totalDuration / int64(data.Runs.Total)
	}
	return data, nil
}

func (r *Repository) GetCreditBalance(ctx context.Context, accountID string) (int, error) {
	doc, err := r.fs.Collection("accounts").Doc(accountID).Get(ctx)
	if err != nil {
		return 0, err
	}
	return toInt(doc.Data()["credit_balance"], 0), nil
}

func (r *Repository) GetBillingSummary(ctx context.Context, accountID string) (*BillingSummary, error) {
	balance, err := r.GetCreditBalance(ctx, accountID)
	if err != nil {
		return nil, err
	}
	return &BillingSummary{
		Balance:       balance,
		Packages:      r.config.BillingPackages,
		LastUpdatedAt: time.Now().UTC(),
	}, nil
}

func (r *Repository) RecordCreditsPurchase(ctx context.Context, accountID, providerEventID string, payload []byte, amountCents int, currency string, credits int) error {
	eventRef := r.fs.Collection("payment_events").Doc(providerEventID)
	now := time.Now().UTC()
	return r.fs.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		snap, err := tx.Get(eventRef)
		if err == nil && snap.Exists() {
			return nil
		}
		accountRef := r.fs.Collection("accounts").Doc(accountID)
		accountSnap, err := tx.Get(accountRef)
		if err != nil {
			return err
		}
		balance := toInt(accountSnap.Data()["credit_balance"], 0)
		tx.Set(accountRef, map[string]interface{}{
			"credit_balance": balance + credits,
			"updated_at":     now,
		}, firestore.MergeAll)
		tx.Set(eventRef, map[string]interface{}{
			"id":                providerEventID,
			"account_id":        accountID,
			"provider":          "stripe",
			"provider_event_id": providerEventID,
			"payload":           string(payload),
			"amount_cents":      amountCents,
			"currency":          currency,
			"credits_granted":   credits,
			"status":            "completed",
			"created_at":        now,
		})
		tx.Set(r.fs.Collection("credit_ledger").NewDoc(), map[string]interface{}{
			"account_id":       accountID,
			"event_type":       "purchase",
			"credits":          credits,
			"reference":        "stripe:" + providerEventID,
			"payment_event_id": providerEventID,
			"created_at":       now,
		})
		return nil
	})
}

type DueSchedule struct {
	AccountID    string
	ScheduleCron string
	Timezone     string
}

func (r *Repository) ListDueSchedules(ctx context.Context, now time.Time, limit int) ([]DueSchedule, error) {
	if limit <= 0 {
		limit = 100
	}
	iter := r.fs.Collection("accounts").
		Where("schedule_enabled", "==", true).
		Where("next_run_at", "<=", now).
		Limit(limit).
		Documents(ctx)

	out := make([]DueSchedule, 0, limit)
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		data := doc.Data()
		out = append(out, DueSchedule{
			AccountID:    doc.Ref.ID,
			ScheduleCron: toString(data["schedule_cron"], "0 */1 * * *"),
			Timezone:     toString(data["timezone"], "UTC"),
		})
	}
	return out, nil
}

func (r *Repository) AdvanceSchedule(ctx context.Context, accountID, cronExpr, timezone string, now time.Time) error {
	next, err := computeNextRunAt(cronExpr, timezone, now)
	if err != nil {
		return err
	}
	_, err = r.fs.Collection("accounts").Doc(accountID).Set(ctx, map[string]interface{}{
		"last_run_at": now,
		"next_run_at": next,
		"updated_at":  now,
	}, firestore.MergeAll)
	return err
}

func computeNextRunAt(cronExpr, timezone string, from time.Time) (time.Time, error) {
	cronExpr = strings.TrimSpace(cronExpr)
	if cronExpr == "" {
		cronExpr = "0 */1 * * *"
	}
	tz := strings.TrimSpace(timezone)
	if tz == "" {
		tz = "UTC"
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid timezone %q: %w", tz, err)
	}
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	schedule, err := parser.Parse(cronExpr)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid cron expression %q: %w", cronExpr, err)
	}
	return schedule.Next(from.In(loc)).UTC(), nil
}

func collectRunSummaries(iter *firestore.DocumentIterator) ([]*models.RunSummary, error) {
	out := make([]*models.RunSummary, 0)
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		out = append(out, runSummaryFromMap(doc.Ref.ID, doc.Data()))
	}
	return out, nil
}

func runSummaryFromMap(runID string, data map[string]interface{}) *models.RunSummary {
	summary := &models.RunSummary{
		RunID:        runID,
		Status:       models.RunStatus(toString(data["status"], runStatusQueued)),
		DurationMs:   toInt64(data["duration_ms"]),
		ErrorMessage: toString(data["error_message"], ""),
	}
	summary.StartedAt = toTime(data["started_at"])
	completed := toTime(data["completed_at"])
	if !completed.IsZero() {
		summary.CompletedAt = &completed
	}
	_ = json.Unmarshal([]byte(toString(data["stage_counts"], "{}")), &summary.StageCounts)
	_ = json.Unmarshal([]byte(toString(data["llm_usage"], "{}")), &summary.LLMUsage)
	_ = json.Unmarshal([]byte(toString(data["config_snapshot"], "{}")), &summary.Config)
	if summary.Config.Locations == nil {
		summary.Config.Locations = []string{}
	}
	summary.ForceReeval = toBool(data["force_reeval"], false)
	return summary
}

func safePtrValue(v *string) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(*v)
}

func toInt(v interface{}, fallback int) int {
	switch t := v.(type) {
	case int:
		return t
	case int64:
		return int(t)
	case float64:
		return int(t)
	default:
		return fallback
	}
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

func toString(v interface{}, fallback string) string {
	if v == nil {
		return fallback
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fallback
}

func toBool(v interface{}, fallback bool) bool {
	if v == nil {
		return fallback
	}
	if b, ok := v.(bool); ok {
		return b
	}
	return fallback
}

func toStringSlice(v interface{}) []string {
	switch t := v.(type) {
	case []string:
		return t
	case []interface{}:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return []string{}
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
	}
	return time.Time{}
}
