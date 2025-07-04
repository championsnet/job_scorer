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
	storage        *storage.FileStorage
	checkpoint     *storage.CheckpointService
	jobTracker     *storage.JobTracker
	rateLimiter    *utils.RateLimiter
	logger         *utils.Logger
}

func NewJobController(cfg *config.Config) (*JobController, error) {
	// Create log directory
	logDir := filepath.Join(cfg.App.DataDir, "logs")
	
	// Create shared file logger for all services
	sharedLogger, err := utils.NewFileLogger("JobScorer", logDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create shared logger: %w", err)
	}

	// Create checkpoint service
	checkpointDir := filepath.Join(cfg.App.DataDir, "checkpoints")
	checkpointService, err := storage.NewCheckpointService(checkpointDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create checkpoint service: %w", err)
	}

	// Create job tracker
	jobTracker, err := storage.NewJobTracker(cfg.App.DataDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create job tracker: %w", err)
	}

	// Initialize services with shared logger
	linkedInScraper := scraper.NewLinkedInScraper()
	filterService := filter.NewFilter(sharedLogger)
	cvReader := cv.NewCVReader(cfg.App.CVPath)
	notifier := notification.NewEmailNotifier(cfg)
	storageService := storage.NewFileStorage(cfg.App.OutputDir)

	// Token-aware rate limiter for Groq
	// Based on Groq's actual limits from headers:
	// - 14400 requests per day = 10 requests per minute (conservative)
	// - 18000 tokens per minute (using 17000 for safety margin)
	tokenRateLimiter := utils.NewTokenRateLimiter(
		10, // Conservative: 14400 RPD / 1440 minutes = 10 RPM
		cfg.RateLimit.TimeWindow,
		cfg.RateLimit.MaxTokensPerMinute,
		time.Minute,
	)

	// Initialize Groq client and evaluator with shared logger
	groqClient := evaluator.NewGroqClient(cfg, tokenRateLimiter)
	evaluatorService := evaluator.NewEvaluator(groqClient, cvReader, linkedInScraper, tokenRateLimiter, sharedLogger)

	// Ensure output directory exists
	if err := storageService.EnsureOutputDir(); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	controller := &JobController{
		config:      cfg,
		scraper:     linkedInScraper,
		filter:      filterService,
		evaluator:   evaluatorService,
		cvReader:    cvReader,
		notifier:    notifier,
		storage:     storageService,
		checkpoint:  checkpointService,
		jobTracker:  jobTracker,
		rateLimiter: tokenRateLimiter,
		logger:      sharedLogger,
	}

	return controller, nil
}

func (jc *JobController) SearchAndFilterJobs() error {
	jc.logger.Info("🎯 Starting Job Scorer Pipeline - %s", time.Now().Format("2006-01-02 15:04:05"))

	// Set a single run/session folder for all checkpoints in this run
	runFolder := time.Now().Format("2006-01-02_15-04-05")
	jc.checkpoint.SetRunFolder(runFolder)

	metadata := map[string]interface{}{
		"locations": jc.config.App.Locations,
		"max_jobs":  jc.config.App.MaxJobs,
	}

	// STEP 1: FETCH JOBS
	jc.logger.PipelineStart("JOB FETCHING", "Scraping LinkedIn for job postings from all configured locations")
	allJobs, err := jc.fetchJobsFromAllLocations()
	if err != nil {
		return fmt.Errorf("failed to fetch jobs: %w", err)
	}
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
	newJobs := jc.jobTracker.FilterProcessedJobs(allJobs)
	processedCount := len(allJobs) - len(newJobs)
	jc.logger.PipelineResult("DUPLICATE FILTERING", len(allJobs), len(newJobs), fmt.Sprintf("(%d duplicates removed)", processedCount))
	
	if processedCount > 0 {
		jc.logger.JobDetail("Skipped %d already processed jobs", processedCount)
	}

	// STEP 3: PREFILTER JOBS
	jc.logger.PipelineStart("PREFILTERING", "Applying location, language and seniority filters")
	prefilteredJobs := jc.filter.PrefilterJobs(newJobs)
	filteredOutCount := len(newJobs) - len(prefilteredJobs)
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

	if err := jc.checkpoint.SaveCheckpoint(prefilteredJobs, "prefiltered", metadata); err != nil {
		jc.logger.Error("Failed to save prefiltered jobs checkpoint: %v", err)
	}

	// STEP 4: LLM INITIAL EVALUATION
	jc.logger.PipelineStart("LLM INITIAL EVALUATION", "AI scoring of jobs for basic suitability")
	evaluatedJobs, err := jc.evaluateJobsWithRateLimit(prefilteredJobs)
	if err != nil {
		return fmt.Errorf("failed to evaluate jobs: %w", err)
	}
	
	successCount := jc.countSuccessfulEvaluations(evaluatedJobs)
	jc.logger.PipelineResult("LLM EVALUATION", len(prefilteredJobs), successCount, "successfully scored")
	jc.logEvaluationSummary("Initial Evaluation", evaluatedJobs)

	if err := jc.checkpoint.SaveCheckpoint(evaluatedJobs, "evaluated", metadata); err != nil {
		jc.logger.Error("Failed to save evaluated jobs checkpoint: %v", err)
	}

	// STEP 5: FILTER PROMISING JOBS
	jc.logger.PipelineStart("PROMISING FILTER", "Selecting jobs with score >= 5.0 for detailed evaluation")
	promisingJobs := jc.filter.FilterPromisingJobs(evaluatedJobs, 5.0)
	jc.logger.PipelineResult("PROMISING FILTER", len(evaluatedJobs), len(promisingJobs), "passed initial threshold")
	jc.logJobSummary("Promising Jobs", promisingJobs)

	if err := jc.storage.SavePromisingJobs(promisingJobs); err != nil {
		jc.logger.Error("Failed to save promising jobs: %v", err)
	}
	if err := jc.checkpoint.SaveCheckpoint(promisingJobs, "promising", metadata); err != nil {
		jc.logger.Error("Failed to save promising jobs checkpoint: %v", err)
	}

	// STEP 6: CV-BASED EVALUATION
	jc.logger.PipelineStart("CV MATCHING", "Detailed AI evaluation with CV comparison")
	finalEvaluatedJobs, err := jc.evaluateJobsWithCV(promisingJobs)
	if err != nil {
		return fmt.Errorf("failed to perform CV evaluation: %w", err)
	}
	
	finalSuccessCount := jc.countSuccessfulFinalEvaluations(finalEvaluatedJobs)
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
	jc.logger.PipelineResult("NOTIFICATION FILTER", len(finalEvaluatedJobs), len(notificationJobs), "selected for notification")

	if err := jc.checkpoint.SaveCheckpoint(notificationJobs, "notification", metadata); err != nil {
		jc.logger.Error("Failed to save notification jobs checkpoint: %v", err)
	}

	// STEP 7.5: LLM VALIDATION (TWO-STAGE)
	jc.logger.PipelineStart("LLM VALIDATION", "Validating notification jobs with second LLM pass")
	var validatedNotificationJobs []*models.Job
	var validationRejected int
	redFlagPhrases := []string{
		"german required", "fluency in german", "fluent german", "deutsch erforderlich", "job description not available", "not available", "german is required", "german language requirement", "german as a requirement", "german as primary language",
	}
	for _, job := range notificationJobs {
		// Programmatic rejection: missing/placeholder description
		desc := strings.ToLower(strings.TrimSpace(job.JobDescription))
		if desc == "" || desc == "job description not available" || desc == "not available" {
			validationRejected++
			jc.logger.JobDetail("❌ Programmatic validation rejected: %s at %s (Reason: missing or placeholder job description)", job.Position, job.Company)
			continue
		}

		valid, reason, err := jc.evaluator.ValidateFinalEvaluation(job)
		if err != nil {
			jc.logger.Warning("LLM validation failed for %s at %s: %v", job.Position, job.Company, err)
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
			jc.logger.JobDetail("❌ Programmatic validation rejected: %s at %s (Reason: red-flag in LLM reason: %s)", job.Position, job.Company, reason)
			continue
		}

		if valid {
			validatedNotificationJobs = append(validatedNotificationJobs, job)
			jc.logger.JobDetail("✅ LLM validation passed: %s at %s (Reason: %s)", job.Position, job.Company, reason)
		} else {
			validationRejected++
			jc.logger.JobDetail("❌ LLM validation rejected: %s at %s (Reason: %s)", job.Position, job.Company, reason)
		}
	}
	jc.logger.PipelineResult("LLM VALIDATION", len(notificationJobs), len(validatedNotificationJobs), "passed validation")
	if validationRejected > 0 {
		jc.logger.Warning("%d jobs rejected by LLM validation", validationRejected)
	}

	// STEP 8: SEND NOTIFICATIONS
	jc.logger.PipelineStart("EMAIL NOTIFICATION", "Sending email alerts for selected jobs")
	if len(validatedNotificationJobs) > 0 {
		for i, job := range validatedNotificationJobs {
			jc.logger.Progress(i+1, len(validatedNotificationJobs), "Preparing notification for: %s at %s", job.Position, job.Company)
		}
		if err := jc.notifier.SendJobNotification(validatedNotificationJobs); err != nil {
			jc.logger.Error("Failed to send email notification: %v", err)
			return fmt.Errorf("failed to send email notification: %w", err)
		}
		jc.logger.Success("Email notification sent for %d jobs", len(validatedNotificationJobs))
	} else {
		jc.logger.Skip("No jobs qualified for email notification after LLM validation")
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
			DateSincePosted: "past 2 hours",
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

	// Count jobs that should be sent via email
	emailJobsCount := 0
	for _, job := range finalEvaluatedJobs {
		if job.ShouldSendEmail {
			emailJobsCount++
		}
	}

	jc.logger.Info("CV evaluation completed: %d jobs evaluated, %d marked for email notification", 
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
		"groq_configured": jc.config.Groq.APIKey != "",
		"smtp_configured": jc.notifier.IsConfigured(),
		"cv_loaded":       jc.cvReader.IsLoaded(),
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
	
	recommendedCount := 0
	notRecommendedCount := 0
	failedCount := 0
	
	jc.logger.JobDetail("%s Results:", title)
	for _, job := range jobs {
		if job.FinalScore == nil {
			failedCount++
		} else if job.ShouldSendEmail {
			recommendedCount++
			jc.logger.JobDetail("  ✅ RECOMMENDED: %s at %s (Score: %.1f)", job.Position, job.Company, *job.FinalScore)
		} else {
			notRecommendedCount++
		}
	}
	
	jc.logger.JobDetail("Summary: %d recommended, %d not recommended, %d failed", 
		recommendedCount, notRecommendedCount, failedCount)
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