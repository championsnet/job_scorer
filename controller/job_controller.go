package controller

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"job-scorer/config"
	"job-scorer/models"
	"job-scorer/services/cv"
	"job-scorer/services/evaluator"
	"job-scorer/services/filter"
	"job-scorer/services/notification"
	"job-scorer/services/scraper"
	"job-scorer/services/storage"
	"job-scorer/utils"
)

type JobController struct {
	config         *config.Config
	scraper        *scraper.LinkedInScraper
	filter         *filter.Filter
	evaluator      *evaluator.Evaluator
	cvReader       *cv.CVReader
	notifier       *notification.EmailNotifier
	storage        storage.JobStorage
	checkpoint     storage.CheckpointStorage
	runSummaryStore storage.RunSummaryStore
	jobTracker     storage.JobTrackerInterface
	rateLimiter    *utils.RateLimiter
	logger         *utils.Logger
	gcsStorage     *storage.GCSStorage // Optional GCS storage
}

func NewJobController(cfg *config.Config) (*JobController, error) {
	// Create log directory
	logDir := filepath.Join(cfg.App.DataDir, "logs")

	// Initialize GCS storage if enabled
	var gcsStorage *storage.GCSStorage
	var sharedLogger *utils.Logger

	if cfg.GCS.Enabled {
		var err error
		gcsStorage, err = storage.NewGCSStorage(storage.GCSConfig{
			BucketName:  cfg.GCS.BucketName,
			ProjectID:   cfg.GCS.ProjectID,
			Enabled:     cfg.GCS.Enabled,
			FallbackDir: cfg.GCS.FallbackDir,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create GCS storage: %w", err)
		}

		// Create GCS-enabled logger
		gcsLogger, err := utils.NewGCSFileLogger("JobScorer", logDir, gcsStorage, true)
		if err != nil {
			return nil, fmt.Errorf("failed to create GCS logger: %w", err)
		}
		sharedLogger = gcsLogger.Logger
	} else {
		// Create regular file logger
		var err error
		sharedLogger, err = utils.NewFileLogger("JobScorer", logDir)
		if err != nil {
			return nil, fmt.Errorf("failed to create shared logger: %w", err)
		}
	}

	// Create storage services (GCS or local)
	var jobStorage storage.JobStorage
	var checkpointService storage.CheckpointStorage
	var runSummaryStore storage.RunSummaryStore
	var jobTracker storage.JobTrackerInterface

	if cfg.GCS.Enabled && gcsStorage != nil && gcsStorage.IsEnabled() {
		// Use GCS storage services
		jobStorage = storage.NewGCSFileStorage(gcsStorage)

		gcsCheckpointService, err := storage.NewGCSCheckpointService(gcsStorage)
		if err != nil {
			return nil, fmt.Errorf("failed to create GCS checkpoint service: %w", err)
		}
		checkpointService = gcsCheckpointService

		runSummaryStore = storage.NewGCSRunSummaryStore(gcsStorage)

		gcsJobTracker, err := storage.NewGCSJobTracker(gcsStorage)
		if err != nil {
			return nil, fmt.Errorf("failed to create GCS job tracker: %w", err)
		}
		jobTracker = gcsJobTracker

		sharedLogger.Info("Using Google Cloud Storage for data persistence")
	} else {
		// Use local storage services
		jobStorage = storage.NewFileStorage(cfg.App.OutputDir)

		checkpointDir := filepath.Join(cfg.App.DataDir, "checkpoints")
		localCheckpointService, err := storage.NewCheckpointService(checkpointDir)
		if err != nil {
			return nil, fmt.Errorf("failed to create checkpoint service: %w", err)
		}
		checkpointService = localCheckpointService

		localRunSummaryStore, err := storage.NewLocalRunSummaryStore(checkpointDir)
		if err != nil {
			return nil, fmt.Errorf("failed to create run summary store: %w", err)
		}
		runSummaryStore = localRunSummaryStore

		localJobTracker, err := storage.NewJobTracker(cfg.App.DataDir)
		if err != nil {
			return nil, fmt.Errorf("failed to create job tracker: %w", err)
		}
		jobTracker = localJobTracker

		sharedLogger.Info("Using local file storage for data persistence")
	}

	// Initialize services with shared logger
	linkedInScraper := scraper.NewLinkedInScraper(cfg.Policy.Scraper)
	filterService := filter.NewFilter(cfg.Policy.Filters, cfg.Policy.Notification, sharedLogger)
	cvReader := cv.NewCVReader(cfg.App.CVPath, cfg.Policy.CV)
	notifier := notification.NewEmailNotifier(cfg)

	// Token-aware rate limiter for LLM provider
	// Based on conservative API limits:
	// - 14400 requests per day = 10 requests per minute (conservative)
	// - 18000 tokens per minute (using 17000 for safety margin)
	tokenRateLimiter := utils.NewTokenRateLimiter(
		10, // Conservative: 14400 RPD / 1440 minutes = 10 RPM
		cfg.RateLimit.TimeWindow,
		cfg.RateLimit.MaxTokensPerMinute,
		time.Minute,
	)

	// Initialize LLM client and evaluator with shared logger
	openAIClient := evaluator.NewOpenAIClient(cfg, tokenRateLimiter)
	evaluatorService := evaluator.NewEvaluator(openAIClient, cvReader, linkedInScraper, tokenRateLimiter, cfg.Policy, sharedLogger)

	// Ensure output directory exists
	if err := jobStorage.EnsureOutputDir(); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	controller := &JobController{
		config:          cfg,
		scraper:         linkedInScraper,
		filter:          filterService,
		evaluator:       evaluatorService,
		cvReader:        cvReader,
		notifier:        notifier,
		storage:         jobStorage,
		checkpoint:      checkpointService,
		runSummaryStore: runSummaryStore,
		jobTracker:      jobTracker,
		rateLimiter:     tokenRateLimiter,
		logger:          sharedLogger,
		gcsStorage:      gcsStorage,
	}

	return controller, nil
}

func (jc *JobController) SearchAndFilterJobs() error {
	return jc.SearchAndFilterJobsWithRunID("")
}

func (jc *JobController) SearchAndFilterJobsWithRunID(runID string) error {
	startTime := time.Now()
	jc.evaluator.ResetLLMUsageTotals()
	jc.logger.Info("🎯 Starting Job Scorer Pipeline - %s", startTime.Format("2006-01-02 15:04:05"))

	// Set a single run/session folder for all checkpoints in this run
	runFolder := runID
	if runFolder == "" {
		runFolder = startTime.Format("2006-01-02_15-04-05")
	}
	jc.checkpoint.SetRunFolder(runFolder)

	metadata := map[string]interface{}{
		"locations": jc.config.App.Locations,
		"max_jobs":  jc.config.App.MaxJobs,
	}

	// Build run summary on exit (success or failure)
	stageCounts := models.RunStageCounts{}
	configSnapshot := models.RunConfigSnapshot{
		Locations: jc.config.App.Locations,
		MaxJobs:   jc.config.App.MaxJobs,
	}
	saveRunSummary := func(status models.RunStatus, errMsg string, notif *models.NotificationResult) {
		completedAt := time.Now()
		durationMs := completedAt.Sub(startTime).Milliseconds()
		usage := jc.evaluator.GetLLMUsageTotals()
		summary := &models.RunSummary{
			RunID:       runFolder,
			Status:      status,
			StartedAt:   startTime,
			CompletedAt: &completedAt,
			DurationMs:  durationMs,
			StageCounts: stageCounts,
			Config:      configSnapshot,
			LLMUsage: models.LLMUsageSnapshot{
				Calls:                 usage.Calls,
				InputTokens:           usage.InputTokens,
				CachedInputTokens:     usage.CachedInputTokens,
				NonCachedInputTokens:  usage.NonCachedInputTokens,
				BillableInputTokens:   usage.BillableInputTokens,
				OutputTokens:          usage.OutputTokens,
				TotalTokens:           usage.TotalTokens,
			},
			Notification: notif,
			ErrorMessage: errMsg,
		}
		if saveErr := jc.runSummaryStore.SaveRunSummary(summary); saveErr != nil {
			jc.logger.Error("Failed to save run summary: %v", saveErr)
		}
	}

	// STEP 1: FETCH JOBS
	jc.logger.PipelineStart("JOB FETCHING", "Scraping LinkedIn for job postings from all configured locations")
	allJobs, err := jc.fetchJobsFromAllLocations()
	if err != nil {
		saveRunSummary(models.RunStatusFailed, err.Error(), nil)
		return fmt.Errorf("failed to fetch jobs: %w", err)
	}
	stageCounts.AllJobs = len(allJobs)
	jc.logger.PipelineResult("FETCHING", 0, len(allJobs), "total jobs scraped")
	jc.logJobSummary("All Fetched Jobs", allJobs)

	// Save all jobs and checkpoint
	if err := jc.storage.SaveAllJobs(allJobs); err != nil {
		jc.logger.Error("Failed to save all jobs: %v", err)
	}
	if err := jc.checkpoint.SaveCheckpoint(allJobs, "all_jobs", metadata); err != nil {
		jc.logger.Error("Failed to save all jobs checkpoint: %v", err)
	}

	// STEP 2: FILTER ALREADY PROCESSED
	jc.logger.PipelineStart("DUPLICATE FILTERING", "Removing jobs that have already been processed")
	var newJobs []*models.Job
	if jc.config.App.ForceReeval {
		jc.logger.Info("ForceReeval is enabled — skipping duplicate filter, all %d jobs will be re-evaluated", len(allJobs))
		newJobs = allJobs
	} else {
		newJobs = jc.jobTracker.FilterProcessedJobs(allJobs)
	}
	processedCount := len(allJobs) - len(newJobs)
	stageCounts.DuplicatesRemoved = processedCount
	jc.logger.PipelineResult("DUPLICATE FILTERING", len(allJobs), len(newJobs), fmt.Sprintf("(%d duplicates removed)", processedCount))

	if processedCount > 0 {
		jc.logger.JobDetail("Skipped %d already processed jobs", processedCount)
	}

	// STEP 3: PREFILTER JOBS
	jc.logger.PipelineStart("PREFILTERING", "Applying location, language and seniority filters")
	prefilteredJobs := jc.filter.PrefilterJobs(newJobs)
	filteredOutCount := len(newJobs) - len(prefilteredJobs)
	prefilterExcludedJobs := buildPrefilterExcludedJobs(allJobs, newJobs, prefilteredJobs, jc.filter)
	jc.logger.PipelineResult("PREFILTERING", len(newJobs), len(prefilteredJobs), fmt.Sprintf("(%d filtered out)", filteredOutCount))

	// Log details of filtered out jobs
	if filteredOutCount > 0 {
		jc.logFilteredJobs(newJobs, prefilteredJobs)
	}

	// Mark prefiltered jobs as processed
	if len(prefilteredJobs) > 0 {
		var jobIDs []string
		for _, job := range prefilteredJobs {
			if job.JobID != "" {
				jobIDs = append(jobIDs, job.JobID)
			}
		}
		if err := jc.jobTracker.MarkMultipleAsProcessed(jobIDs); err != nil {
			jc.logger.Error("Failed to mark jobs as processed: %v", err)
		}
	}

	stageCounts.Prefiltered = len(prefilteredJobs)
	if err := jc.checkpoint.SaveCheckpoint(prefilteredJobs, "prefiltered", metadata); err != nil {
		jc.logger.Error("Failed to save prefiltered jobs checkpoint: %v", err)
	}
	jc.saveExcludedCheckpoint(prefilterExcludedJobs, "prefiltered", metadata)

	// STEP 4: LLM INITIAL EVALUATION
	jc.logger.PipelineStart("LLM INITIAL EVALUATION", "AI scoring of jobs for basic suitability")
	evaluatedJobs, err := jc.evaluateJobsWithRateLimit(prefilteredJobs)
	if err != nil {
		saveRunSummary(models.RunStatusFailed, err.Error(), nil)
		return fmt.Errorf("failed to evaluate jobs: %w", err)
	}

	stageCounts.Evaluated = jc.countSuccessfulEvaluations(evaluatedJobs)
	successCount := stageCounts.Evaluated
	jc.logger.PipelineResult("LLM EVALUATION", len(prefilteredJobs), successCount, "successfully scored")
	jc.logEvaluationSummary("Initial Evaluation", evaluatedJobs)

	if err := jc.checkpoint.SaveCheckpoint(evaluatedJobs, "evaluated", metadata); err != nil {
		jc.logger.Error("Failed to save evaluated jobs checkpoint: %v", err)
	}

	// STEP 5: FILTER PROMISING JOBS
	jc.logger.PipelineStart("PROMISING FILTER", "Selecting jobs with policy score threshold for detailed evaluation")
	promisingThreshold := jc.config.Policy.Pipeline.PromisingScoreThreshold
	promisingJobs := jc.filter.FilterPromisingJobs(evaluatedJobs, promisingThreshold)
	promisingExcludedJobs := buildPromisingExcludedJobs(evaluatedJobs, promisingJobs, jc.filter, promisingThreshold)
	stageCounts.Promising = len(promisingJobs)
	jc.logger.PipelineResult("PROMISING FILTER", len(evaluatedJobs), len(promisingJobs), "passed initial threshold")
	jc.logJobSummary("Promising Jobs", promisingJobs)

	if err := jc.storage.SavePromisingJobs(promisingJobs); err != nil {
		jc.logger.Error("Failed to save promising jobs: %v", err)
	}
	if err := jc.checkpoint.SaveCheckpoint(promisingJobs, "promising", metadata); err != nil {
		jc.logger.Error("Failed to save promising jobs checkpoint: %v", err)
	}
	jc.saveExcludedCheckpoint(promisingExcludedJobs, "promising", metadata)

	// STEP 6: CV-BASED EVALUATION
	jc.logger.PipelineStart("CV MATCHING", "Detailed AI evaluation with CV comparison")
	finalEvaluatedJobs, err := jc.evaluateJobsWithCV(promisingJobs)
	if err != nil {
		saveRunSummary(models.RunStatusFailed, err.Error(), nil)
		return fmt.Errorf("failed to perform CV evaluation: %w", err)
	}

	stageCounts.FinalEvaluated = jc.countSuccessfulFinalEvaluations(finalEvaluatedJobs)
	finalSuccessCount := stageCounts.FinalEvaluated
	jc.logger.PipelineResult("CV MATCHING", len(promisingJobs), finalSuccessCount, "with final recommendations")
	jc.logFinalEvaluationSummary("CV-Based Evaluation", finalEvaluatedJobs)

	if err := jc.storage.SaveFinalEvaluatedJobs(finalEvaluatedJobs); err != nil {
		jc.logger.Error("Failed to save final evaluated jobs: %v", err)
	}
	if err := jc.checkpoint.SaveCheckpoint(finalEvaluatedJobs, "final_evaluated", metadata); err != nil {
		jc.logger.Error("Failed to save final evaluated jobs checkpoint: %v", err)
	}

	// STEP 7: NOTIFICATION FILTERING
	jc.logger.PipelineStart("NOTIFICATION FILTER", "Selecting jobs that should trigger email alerts")
	notificationJobs := jc.filter.FilterNotificationJobs(finalEvaluatedJobs)
	notificationExcludedJobs := buildNotificationExcludedJobs(finalEvaluatedJobs, notificationJobs, jc.filter)
	stageCounts.Notification = len(notificationJobs)
	jc.logger.PipelineResult("NOTIFICATION FILTER", len(finalEvaluatedJobs), len(notificationJobs), "selected for notification")

	if err := jc.checkpoint.SaveCheckpoint(notificationJobs, "notification", metadata); err != nil {
		jc.logger.Error("Failed to save notification jobs checkpoint: %v", err)
	}
	jc.saveExcludedCheckpoint(notificationExcludedJobs, "notification", metadata)

	// STEP 7.5: LLM VALIDATION (TWO-STAGE) - Optional
	var validatedNotificationJobs []*models.Job
	var validationExcludedJobs []*models.Job
	if jc.config.Policy.Pipeline.EnableFinalValidation {
		jc.logger.PipelineStart("LLM VALIDATION", "Validating notification jobs with second LLM pass")
		var validationRejected int
		redFlagPhrases := jc.config.Policy.Pipeline.RedFlagPhrases
		for _, job := range notificationJobs {
			// Programmatic rejection: missing/placeholder description
			desc := strings.ToLower(strings.TrimSpace(job.JobDescription))
			if (jc.config.Policy.Pipeline.RejectEmptyDescriptions && desc == "") ||
				(jc.config.Policy.Pipeline.RejectPlaceholderDescription && (desc == "job description not available" || desc == "not available")) {
				validationRejected++
				rejected := cloneJob(job)
				rejected.Excluded = true
				rejected.ExclusionReason = "Excluded by validation because the job description was missing or placeholder text"
				validationExcludedJobs = append(validationExcludedJobs, rejected)
				jc.logger.JobDetail("❌ Programmatic validation rejected: %s at %s (Reason: missing or placeholder job description)", job.Position, job.Company)
				continue
			}

			valid, reason, err := jc.evaluator.ValidateFinalEvaluation(job)
			if err != nil {
				jc.logger.Warning("LLM validation failed for %s at %s: %v", job.Position, job.Company, err)
				rejected := cloneJob(job)
				rejected.Excluded = true
				rejected.ExclusionReason = "Excluded because validation failed and no reliable approval could be established"
				validationExcludedJobs = append(validationExcludedJobs, rejected)
				continue
			}

			// Programmatic rejection: red-flag phrases in LLM reason
			reasonLower := strings.ToLower(reason)
			flagged := false
			for _, phrase := range redFlagPhrases {
				if strings.Contains(reasonLower, phrase) {
					flagged = true
					break
				}
			}
			if flagged {
				validationRejected++
				rejected := cloneJob(job)
				rejected.Excluded = true
				rejected.ExclusionReason = "Excluded by validation because the validation reason contained a configured red-flag phrase"
				validationExcludedJobs = append(validationExcludedJobs, rejected)
				jc.logger.JobDetail("❌ Programmatic validation rejected: %s at %s (Reason: red-flag in LLM reason: %s)", job.Position, job.Company, reason)
				continue
			}

			if valid {
				validatedNotificationJobs = append(validatedNotificationJobs, job)
				jc.logger.JobDetail("✅ LLM validation passed: %s at %s (Reason: %s)", job.Position, job.Company, reason)
			} else {
				validationRejected++
				rejected := cloneJob(job)
				rejected.Excluded = true
				rejected.ExclusionReason = reason
				validationExcludedJobs = append(validationExcludedJobs, rejected)
				jc.logger.JobDetail("❌ LLM validation rejected: %s at %s (Reason: %s)", job.Position, job.Company, reason)
			}
		}
		jc.logger.PipelineResult("LLM VALIDATION", len(notificationJobs), len(validatedNotificationJobs), "passed validation")
		if validationRejected > 0 {
			jc.logger.Warning("%d jobs rejected by LLM validation", validationRejected)
		}
	} else {
		// Skip LLM validation - use all notification jobs
		validatedNotificationJobs = notificationJobs
		jc.logger.Info("LLM validation disabled - proceeding with all %d notification jobs", len(notificationJobs))
	}

	stageCounts.Validated = len(validatedNotificationJobs)
	if err := jc.checkpoint.SaveCheckpoint(validatedNotificationJobs, "validated_notification", metadata); err != nil {
		jc.logger.Error("Failed to save validated notification jobs checkpoint: %v", err)
	}
	jc.saveExcludedCheckpoint(validationExcludedJobs, "validated_notification", metadata)

	// STEP 8: SEND NOTIFICATIONS
	jc.logger.PipelineStart("EMAIL NOTIFICATION", "Sending email alerts for selected jobs")
	if len(validatedNotificationJobs) > 0 {
		for i, job := range validatedNotificationJobs {
			jc.logger.Progress(i+1, len(validatedNotificationJobs), "Preparing notification for: %s at %s", job.Position, job.Company)
		}
		if err := jc.notifier.SendJobNotification(validatedNotificationJobs); err != nil {
			jc.logger.Error("Failed to send email notification: %v", err)
			notifResult := &models.NotificationResult{
				RunID:           runFolder,
				JobIDs:          jobIDsFromJobs(validatedNotificationJobs),
				RecipientsCount: len(jc.config.SMTP.ToRecipients),
				SuccessCount:    0,
				FailedCount:     len(jc.config.SMTP.ToRecipients),
				CompletedAt:     time.Now().Format(time.RFC3339),
				ErrorMessage:    err.Error(),
			}
			saveRunSummary(models.RunStatusFailed, err.Error(), notifResult)
			return fmt.Errorf("failed to send email notification: %w", err)
		}
		stageCounts.EmailSent = len(validatedNotificationJobs)
		if err := jc.checkpoint.SaveCheckpoint(validatedNotificationJobs, "email_sent", metadata); err != nil {
			jc.logger.Error("Failed to save email sent jobs checkpoint: %v", err)
		}
		notifResult := &models.NotificationResult{
			RunID:           runFolder,
			JobIDs:          jobIDsFromJobs(validatedNotificationJobs),
			RecipientsCount: len(jc.config.SMTP.ToRecipients),
			SuccessCount:    len(jc.config.SMTP.ToRecipients),
			FailedCount:     0,
			CompletedAt:     time.Now().Format(time.RFC3339),
		}
		jc.logger.Success("Email notification sent for %d jobs", len(validatedNotificationJobs))
		saveRunSummary(models.RunStatusSuccess, "", notifResult)
	} else {
		jc.logger.Skip("No jobs qualified for email notification after LLM validation")
		if err := jc.checkpoint.SaveCheckpoint([]*models.Job{}, "email_sent", metadata); err != nil {
			jc.logger.Error("Failed to save empty email sent jobs checkpoint: %v", err)
		}
		saveRunSummary(models.RunStatusSuccess, "", nil)
	}

	// Save daily snapshot
	if err := jc.checkpoint.SaveDailySnapshot(allJobs, promisingJobs, finalEvaluatedJobs); err != nil {
		jc.logger.Error("Failed to save daily snapshot: %v", err)
	}

	// FINAL SUMMARY
	jc.logger.Info("")
	jc.logger.Success("🎉 Pipeline completed successfully!")
	jc.logger.Info("📈 FINAL STATS:")
	jc.logger.Info("   • Total Fetched: %d jobs", len(allJobs))
	jc.logger.Info("   • New Jobs: %d jobs", len(newJobs))
	jc.logger.Info("   • After Prefiltering: %d jobs", len(prefilteredJobs))
	jc.logger.Info("   • Successfully Evaluated: %d jobs", successCount)
	jc.logger.Info("   • Promising Jobs: %d jobs", len(promisingJobs))
	jc.logger.Info("   • Final Recommendations: %d jobs", finalSuccessCount)
	jc.logger.Info("   • Email Notifications: %d jobs", len(validatedNotificationJobs))
	llmUsage := jc.evaluator.GetLLMUsageTotals()
	jc.logger.Info("   • LLM Token Usage: input=%d (non_cached=%d, cached=%d, billable=%d), output=%d, total=%d, calls=%d",
		llmUsage.InputTokens, llmUsage.NonCachedInputTokens, llmUsage.CachedInputTokens, llmUsage.BillableInputTokens,
		llmUsage.OutputTokens, llmUsage.TotalTokens, llmUsage.Calls)
	jc.logger.Info("")

	return nil
}

func (jc *JobController) fetchJobsFromAllLocations() ([]*models.Job, error) {
	var allJobs []*models.Job
	totalErrors := 0
	seenJobIDs := make(map[string]bool) // Track unique job IDs
	var duplicateCount int

	for i, location := range jc.config.App.Locations {
		jc.logger.Progress(i+1, len(jc.config.App.Locations), "Scraping location: %s", location)

		options := scraper.QueryOptions{
			Location:        location,
			DateSincePosted: jc.config.Policy.Scraper.DateSincePosted,
			Limit:           jc.config.App.MaxJobs,
		}

		jobs, err := jc.scraper.Query(options)
		if err != nil {
			totalErrors++
			jc.logger.JobDetail("❌ Failed to fetch from location %s: %v", location, err)
			// Continue to next location even if one fails
			continue
		}

		// Deduplicate jobs based on JobID
		var uniqueJobs []*models.Job
		for _, job := range jobs {
			if job.JobID != "" && seenJobIDs[job.JobID] {
				duplicateCount++
				continue
			}
			if job.JobID != "" {
				seenJobIDs[job.JobID] = true
			}
			uniqueJobs = append(uniqueJobs, job)
		}

		jc.logger.JobDetail("✅ Found %d unique jobs from location %s (filtered %d duplicates)",
			len(uniqueJobs), location, duplicateCount)
		allJobs = append(allJobs, uniqueJobs...)
	}

	if totalErrors > 0 {
		jc.logger.Warning("⚠️  %d location(s) failed to fetch", totalErrors)
	}
	if duplicateCount > 0 {
		jc.logger.Info("🔍 Filtered out %d duplicate job postings", duplicateCount)
	}

	return allJobs, nil
}

func (jc *JobController) evaluateJobsWithRateLimit(jobs []*models.Job) ([]*models.Job, error) {
	if len(jobs) == 0 {
		return jobs, nil
	}

	// Use batch processing for initial evaluations (3 jobs at a time)
	batchSize := jc.config.Policy.Pipeline.BatchSize

	// If we have a small number of jobs, fall back to individual processing
	if len(jobs) <= jc.config.Policy.Pipeline.IndividualFallbackMaxJobs {
		return jc.evaluateJobsIndividually(jobs)
	}

	jc.logger.Info("Using batch processing for %d jobs (batch size: %d)", len(jobs), batchSize)
	evaluatedJobs, err := jc.evaluator.BatchEvaluateJobs(jobs, batchSize)
	if err != nil {
		jc.logger.Warning("Batch evaluation failed, falling back to individual evaluation: %v", err)
		return jc.evaluateJobsIndividually(jobs)
	}

	// Count successful evaluations
	successCount := 0
	for _, job := range evaluatedJobs {
		if job.Score != nil {
			successCount++
		}
	}

	jc.logger.Info("Batch evaluation completed: %d/%d jobs successfully evaluated", successCount, len(jobs))
	return evaluatedJobs, nil
}

// evaluateJobsIndividually processes jobs one by one (fallback method)
func (jc *JobController) evaluateJobsIndividually(jobs []*models.Job) ([]*models.Job, error) {
	var evaluatedJobs []*models.Job
	errorCount := 0

	// Process jobs sequentially to respect rate limit
	for i, job := range jobs {
		jc.logger.Progress(i+1, len(jobs), "Evaluating: %s at %s", job.Position, job.Company)

		// Wait for rate limiter before making request
		if err := jc.rateLimiter.Acquire(); err != nil {
			return nil, fmt.Errorf("rate limiter error: %w", err)
		}

		evaluatedJob, err := jc.evaluator.EvaluateJob(job)
		if err != nil {
			errorCount++
			jc.logger.JobDetail("❌ Evaluation failed for '%s': %v", job.Position, err)
			// Continue with next job even if one fails
			evaluatedJobs = append(evaluatedJobs, job)
			continue
		}

		// Log successful evaluation with score
		if evaluatedJob.Score != nil {
			jc.logger.JobDetail("✅ Scored %.1f: %s at %s", *evaluatedJob.Score, evaluatedJob.Position, evaluatedJob.Company)
		}

		evaluatedJobs = append(evaluatedJobs, evaluatedJob)
	}

	if errorCount > 0 {
		jc.logger.Warning("⚠️  %d evaluation errors encountered", errorCount)
	}

	return evaluatedJobs, nil
}

func (jc *JobController) evaluateJobsWithCV(jobs []*models.Job) ([]*models.Job, error) {
	if len(jobs) == 0 {
		return jobs, nil
	}

	// Load CV once at the beginning
	_, err := jc.cvReader.LoadCV()
	if err != nil {
		return nil, fmt.Errorf("failed to load CV: %w", err)
	}
	jc.logger.JobDetail("📄 CV loaded successfully for detailed matching")

	var finalEvaluatedJobs []*models.Job

	for i, job := range jobs {
		jc.logger.Debug("CV evaluation %d/%d: %s at %s", i+1, len(jobs), job.Position, job.Company)

		// Wait for rate limiter before making request
		if err := jc.rateLimiter.Acquire(); err != nil {
			return nil, fmt.Errorf("rate limiter error: %w", err)
		}

		evaluatedJob, err := jc.evaluator.EvaluateJobWithCV(job)
		if err != nil {
			jc.logger.Error("Error in CV evaluation for job %s: %v", job.Position, err)
			// Continue with next job even if one fails
			finalEvaluatedJobs = append(finalEvaluatedJobs, job)
			continue
		}

		finalEvaluatedJobs = append(finalEvaluatedJobs, evaluatedJob)
	}

	// Count jobs that meet notification score threshold
	emailJobsCount := jc.countNotificationEligible(finalEvaluatedJobs)

	jc.logger.Info("CV evaluation completed: %d jobs evaluated, %d eligible for email notification",
		len(finalEvaluatedJobs), emailJobsCount)

	return finalEvaluatedJobs, nil
}

func (jc *JobController) GetStats() map[string]interface{} {
	stats := make(map[string]interface{})

	// Rate limiter stats
	active, max, tokensUsed, tokenLimit := jc.rateLimiter.GetStats()
	stats["rate_limiter"] = map[string]interface{}{
		"active_requests": active,
		"max_requests":    max,
		"tokens_used":     tokensUsed,
		"token_limit":     tokenLimit,
	}

	// Configuration stats
	stats["config"] = map[string]interface{}{
		"locations":       jc.config.App.Locations,
		"cron_schedule":   jc.config.App.CronSchedule,
		"llm_configured":  jc.config.OpenAI.APIKey != "",
		"smtp_configured": jc.notifier.IsConfigured(),
		"cv_loaded":       jc.cvReader.IsLoaded(),
	}

	usage := jc.evaluator.GetLLMUsageTotals()
	stats["llm_usage"] = map[string]interface{}{
		"calls":                   usage.Calls,
		"input_tokens":            usage.InputTokens,
		"non_cached_input_tokens": usage.NonCachedInputTokens,
		"cached_input_tokens":     usage.CachedInputTokens,
		"billable_input_tokens":   usage.BillableInputTokens,
		"output_tokens":           usage.OutputTokens,
		"total_tokens":            usage.TotalTokens,
	}

	// Job tracking stats
	stats["job_tracker"] = jc.jobTracker.GetStats()

	return stats
}

// Helper methods for detailed logging
func (jc *JobController) logJobSummary(title string, jobs []*models.Job) {
	if len(jobs) == 0 {
		return
	}

	jc.logger.JobDetail("%s (%d total):", title, len(jobs))
	for i, job := range jobs {
		if i >= 5 { // Limit to first 5 jobs to avoid spam
			jc.logger.JobDetail("... and %d more jobs", len(jobs)-5)
			break
		}
		jc.logger.JobDetail("  • %s at %s (%s)", job.Position, job.Company, job.Location)
	}
}

func (jc *JobController) logFilteredJobs(originalJobs, filteredJobs []*models.Job) {
	// Find jobs that were filtered out
	filteredOut := make([]*models.Job, 0)
	filteredMap := make(map[string]bool)

	for _, job := range filteredJobs {
		if job.JobID != "" {
			filteredMap[job.JobID] = true
		}
	}

	for _, job := range originalJobs {
		if job.JobID != "" && !filteredMap[job.JobID] {
			filteredOut = append(filteredOut, job)
		}
	}

	if len(filteredOut) > 0 {
		jc.logger.JobDetail("Filtered out jobs:")
		for i, job := range filteredOut {
			if i >= 3 { // Limit to first 3 filtered jobs
				jc.logger.JobDetail("... and %d more filtered jobs", len(filteredOut)-3)
				break
			}
			jc.logger.JobDetail("  ❌ %s at %s (%s)", job.Position, job.Company, job.Location)
		}
	}
}

func (jc *JobController) logEvaluationSummary(title string, jobs []*models.Job) {
	if len(jobs) == 0 {
		return
	}

	scoreRanges := map[string]int{
		"9-10": 0, "7-8": 0, "5-6": 0, "3-4": 0, "1-2": 0, "0": 0, "failed": 0,
	}

	for _, job := range jobs {
		if job.Score == nil {
			scoreRanges["failed"]++
		} else {
			score := *job.Score
			switch {
			case score >= 9:
				scoreRanges["9-10"]++
			case score >= 7:
				scoreRanges["7-8"]++
			case score >= 5:
				scoreRanges["5-6"]++
			case score >= 3:
				scoreRanges["3-4"]++
			case score >= 1:
				scoreRanges["1-2"]++
			default:
				scoreRanges["0"]++
			}
		}
	}

	jc.logger.JobDetail("%s Score Distribution:", title)
	jc.logger.JobDetail("  🟢 Excellent (9-10): %d jobs", scoreRanges["9-10"])
	jc.logger.JobDetail("  🟡 Good (7-8): %d jobs", scoreRanges["7-8"])
	jc.logger.JobDetail("  🟠 Average (5-6): %d jobs", scoreRanges["5-6"])
	jc.logger.JobDetail("  🔴 Poor (3-4): %d jobs", scoreRanges["3-4"])
	jc.logger.JobDetail("  ⚫ Very Poor (1-2): %d jobs", scoreRanges["1-2"])
	jc.logger.JobDetail("  💀 Zero (0): %d jobs", scoreRanges["0"])
	if scoreRanges["failed"] > 0 {
		jc.logger.JobDetail("  ❌ Failed Evaluation: %d jobs", scoreRanges["failed"])
	}
}

func (jc *JobController) logFinalEvaluationSummary(title string, jobs []*models.Job) {
	if len(jobs) == 0 {
		return
	}

	eligibleCount := 0
	belowThresholdCount := 0
	failedCount := 0

	jc.logger.JobDetail("%s Results:", title)
	for _, job := range jobs {
		if job.FinalScore == nil {
			failedCount++
		} else if *job.FinalScore >= jc.config.Policy.Notification.MinFinalScore {
			eligibleCount++
			jc.logger.JobDetail("  ✅ ELIGIBLE: %s at %s (Score: %.1f)", job.Position, job.Company, *job.FinalScore)
		} else {
			belowThresholdCount++
		}
	}

	jc.logger.JobDetail("Summary: %d eligible, %d below threshold, %d failed",
		eligibleCount, belowThresholdCount, failedCount)
}

func (jc *JobController) countSuccessfulEvaluations(jobs []*models.Job) int {
	count := 0
	for _, job := range jobs {
		if job.Score != nil {
			count++
		}
	}
	return count
}

func (jc *JobController) countSuccessfulFinalEvaluations(jobs []*models.Job) int {
	count := 0
	for _, job := range jobs {
		if job.FinalScore != nil {
			count++
		}
	}
	return count
}

func (jc *JobController) countNotificationEligible(jobs []*models.Job) int {
	count := 0
	threshold := jc.config.Policy.Notification.MinFinalScore
	for _, job := range jobs {
		if job.FinalScore != nil && *job.FinalScore >= threshold {
			count++
		}
	}
	return count
}

func jobIDsFromJobs(jobs []*models.Job) []string {
	ids := make([]string, 0, len(jobs))
	for _, j := range jobs {
		if j.JobID != "" {
			ids = append(ids, j.JobID)
		}
	}
	return ids
}

// GetRunFolder returns the current/last run folder from the checkpoint service
func (jc *JobController) GetRunFolder() string {
	return jc.checkpoint.GetRunFolder()
}

// ListRuns returns recent run summaries for the API
func (jc *JobController) ListRuns(limit int) ([]*models.RunSummary, error) {
	if limit <= 0 {
		limit = 50
	}
	return jc.runSummaryStore.ListRunSummaries(limit)
}

// GetRun returns a single run summary by ID
func (jc *JobController) GetRun(runID string) (*models.RunSummary, error) {
	return jc.runSummaryStore.LoadRunSummary(runID)
}

// GetRunStageJobs returns jobs for a specific stage of a run
func (jc *JobController) GetRunStageJobs(runID, stage string) ([]*models.Job, error) {
	return jc.checkpoint.LoadCheckpointByStage(runID, stage)
}

// GetAnalyticsOverview returns aggregated analytics across runs for the dashboard
func (jc *JobController) GetAnalyticsOverview() (map[string]interface{}, error) {
	summaries, err := jc.runSummaryStore.ListRunSummaries(100)
	if err != nil {
		return nil, err
	}
	totalRuns := len(summaries)
	successCount := 0
	failedCount := 0
	var totalDurationMs int64
	var totalAllJobs, totalPrefiltered, totalEvaluated, totalPromising, totalFinal, totalNotification, totalValidated, totalEmailSent int
	var totalLLMCalls, totalInputTokens, totalOutputTokens int

	for _, s := range summaries {
		if s.Status == models.RunStatusSuccess {
			successCount++
		} else if s.Status == models.RunStatusFailed {
			failedCount++
		}
		totalDurationMs += s.DurationMs
		totalAllJobs += s.StageCounts.AllJobs
		totalPrefiltered += s.StageCounts.Prefiltered
		totalEvaluated += s.StageCounts.Evaluated
		totalPromising += s.StageCounts.Promising
		totalFinal += s.StageCounts.FinalEvaluated
		totalNotification += s.StageCounts.Notification
		totalValidated += s.StageCounts.Validated
		totalEmailSent += s.StageCounts.EmailSent
		totalLLMCalls += s.LLMUsage.Calls
		totalInputTokens += s.LLMUsage.InputTokens
		totalOutputTokens += s.LLMUsage.OutputTokens
	}

	avgDurationMs := int64(0)
	if totalRuns > 0 {
		avgDurationMs = totalDurationMs / int64(totalRuns)
	}

	return map[string]interface{}{
		"runs": map[string]interface{}{
			"total":   totalRuns,
			"success": successCount,
			"failed":  failedCount,
			"avg_duration_ms": avgDurationMs,
		},
		"funnel": map[string]interface{}{
			"all_jobs":        totalAllJobs,
			"prefiltered":     totalPrefiltered,
			"evaluated":       totalEvaluated,
			"promising":       totalPromising,
			"final_evaluated": totalFinal,
			"notification":    totalNotification,
			"validated":       totalValidated,
			"email_sent":      totalEmailSent,
		},
		"llm_usage": map[string]interface{}{
			"calls":          totalLLMCalls,
			"input_tokens":   totalInputTokens,
			"output_tokens":  totalOutputTokens,
		},
		"recent_runs": summaries,
	}, nil
}
