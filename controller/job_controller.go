package controller

import (
	"fmt"
	"path/filepath"

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

	// Initialize services with shared logger
	linkedInScraper := scraper.NewLinkedInScraper()
	filterService := filter.NewFilter(sharedLogger)
	cvReader := cv.NewCVReader(cfg.App.CVPath)
	notifier := notification.NewEmailNotifier(cfg)
	storageService := storage.NewFileStorage(cfg.App.OutputDir)
	rateLimiter := utils.NewRateLimiter(cfg.RateLimit.MaxRequests, cfg.RateLimit.TimeWindow)

	// Initialize Groq client and evaluator with shared logger
	groqClient := evaluator.NewGroqClient(cfg)
	evaluatorService := evaluator.NewEvaluator(groqClient, cvReader, linkedInScraper, rateLimiter, sharedLogger)

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
		rateLimiter: rateLimiter,
		logger:      sharedLogger,
	}

	return controller, nil
}

func (jc *JobController) SearchAndFilterJobs() error {
	jc.logger.Info("Starting job search and evaluation pipeline...")

	// Step 1: Fetch jobs from all locations
	allJobs, err := jc.fetchJobsFromAllLocations()
	if err != nil {
		return fmt.Errorf("failed to fetch jobs: %w", err)
	}

	// Save checkpoint for all jobs
	metadata := map[string]interface{}{
		"locations": jc.config.App.Locations,
		"max_jobs":  jc.config.App.MaxJobs,
	}
	if err := jc.checkpoint.SaveCheckpoint(allJobs, "all_jobs", metadata); err != nil {
		jc.logger.Error("Failed to save all jobs checkpoint: %v", err)
	}

	// Save all jobs
	if err := jc.storage.SaveAllJobs(allJobs); err != nil {
		jc.logger.Error("Failed to save all jobs: %v", err)
	}

	// Step 2: Prefilter jobs (remove unwanted locations)
	prefilteredJobs := jc.filter.PrefilterJobs(allJobs)

	// Save checkpoint for prefiltered jobs
	if err := jc.checkpoint.SaveCheckpoint(prefilteredJobs, "prefiltered", metadata); err != nil {
		jc.logger.Error("Failed to save prefiltered jobs checkpoint: %v", err)
	}

	// Step 3: Evaluate jobs with initial screening
	evaluatedJobs, err := jc.evaluateJobsWithRateLimit(prefilteredJobs)
	if err != nil {
		return fmt.Errorf("failed to evaluate jobs: %w", err)
	}

	// Save checkpoint for evaluated jobs
	if err := jc.checkpoint.SaveCheckpoint(evaluatedJobs, "evaluated", metadata); err != nil {
		jc.logger.Error("Failed to save evaluated jobs checkpoint: %v", err)
	}

	// Step 4: Filter promising jobs (score >= 7)
	promisingJobs := jc.filter.FilterPromisingJobs(evaluatedJobs, 6.5)
	
	// Save checkpoint for promising jobs
	if err := jc.checkpoint.SaveCheckpoint(promisingJobs, "promising", metadata); err != nil {
		jc.logger.Error("Failed to save promising jobs checkpoint: %v", err)
	}
	
	// Save promising jobs
	if err := jc.storage.SavePromisingJobs(promisingJobs); err != nil {
		jc.logger.Error("Failed to save promising jobs: %v", err)
	}

	// Step 5: Evaluate promising jobs with CV matching
	finalEvaluatedJobs, err := jc.evaluateJobsWithCV(promisingJobs)
	if err != nil {
		return fmt.Errorf("failed to perform CV evaluation: %w", err)
	}

	// Save checkpoint for final evaluated jobs
	if err := jc.checkpoint.SaveCheckpoint(finalEvaluatedJobs, "final_evaluated", metadata); err != nil {
		jc.logger.Error("Failed to save final evaluated jobs checkpoint: %v", err)
	}

	// Save final evaluated jobs
	if err := jc.storage.SaveFinalEvaluatedJobs(finalEvaluatedJobs); err != nil {
		jc.logger.Error("Failed to save final evaluated jobs: %v", err)
	}

	jc.logger.Info("Found %d highly recommended jobs after CV evaluation", len(finalEvaluatedJobs))

	// Step 6: Filter jobs that should trigger notifications
	notificationJobs := jc.filter.FilterNotificationJobs(finalEvaluatedJobs)

	// Save checkpoint for notification jobs
	if err := jc.checkpoint.SaveCheckpoint(notificationJobs, "notification", metadata); err != nil {
		jc.logger.Error("Failed to save notification jobs checkpoint: %v", err)
	}

	// Step 7: Send email notifications if there are jobs to notify about
	if len(notificationJobs) > 0 {
		jc.logger.Info("Sending email notification for %d jobs...", len(notificationJobs))
		if err := jc.notifier.SendJobNotification(notificationJobs); err != nil {
			jc.logger.Error("Failed to send email notification: %v", err)
			return fmt.Errorf("failed to send email notification: %w", err)
		}
	} else {
		jc.logger.Info("No jobs passed the final CV evaluation criteria for notification")
	}

	// Save daily snapshot
	if err := jc.checkpoint.SaveDailySnapshot(allJobs, promisingJobs, finalEvaluatedJobs); err != nil {
		jc.logger.Error("Failed to save daily snapshot: %v", err)
	}

	jc.logger.Info("Job search and evaluation pipeline completed successfully")
	return nil
}

func (jc *JobController) fetchJobsFromAllLocations() ([]*models.Job, error) {
	var allJobs []*models.Job

	for _, location := range jc.config.App.Locations {
		jc.logger.Info("Fetching jobs for location: %s", location)

		options := scraper.QueryOptions{
			Location:        location,
			DateSincePosted: "past hour",
			Limit:           jc.config.App.MaxJobs,
		}

		jobs, err := jc.scraper.Query(options)
		if err != nil {
			jc.logger.Error("Error fetching jobs for location %s: %v", location, err)
			// Continue to next location even if one fails
			continue
		}

		jc.logger.Info("Fetched %d jobs from location %s", len(jobs), location)
		allJobs = append(allJobs, jobs...)
	}

	jc.logger.Info("Total jobs fetched from all locations: %d", len(allJobs))
	return allJobs, nil
}

func (jc *JobController) evaluateJobsWithRateLimit(jobs []*models.Job) ([]*models.Job, error) {
	jc.logger.Info("Starting initial evaluation of %d jobs with rate limiting...", len(jobs))

	var evaluatedJobs []*models.Job

	// Process jobs sequentially to respect rate limit
	for i, job := range jobs {
		jc.logger.Debug("Evaluating job %d/%d: %s at %s", i+1, len(jobs), job.Position, job.Company)

		// Wait for rate limiter before making request
		if err := jc.rateLimiter.Acquire(); err != nil {
			return nil, fmt.Errorf("rate limiter error: %w", err)
		}

		evaluatedJob, err := jc.evaluator.EvaluateJob(job)
		if err != nil {
			jc.logger.Error("Error evaluating job %s: %v", job.Position, err)
			// Continue with next job even if one fails
			evaluatedJobs = append(evaluatedJobs, job)
			continue
		}

		evaluatedJobs = append(evaluatedJobs, evaluatedJob)
	}

	// Count successful evaluations
	successCount := 0
	for _, job := range evaluatedJobs {
		if job.Score != nil {
			successCount++
		}
	}

	jc.logger.Info("Initial evaluation completed: %d/%d jobs successfully evaluated", successCount, len(jobs))
	return evaluatedJobs, nil
}

func (jc *JobController) evaluateJobsWithCV(jobs []*models.Job) ([]*models.Job, error) {
	if len(jobs) == 0 {
		return jobs, nil
	}

	jc.logger.Info("Starting CV-based evaluation of %d promising jobs...", len(jobs))

	// Load CV once at the beginning
	_, err := jc.cvReader.LoadCV()
	if err != nil {
		return nil, fmt.Errorf("failed to load CV: %w", err)
	}

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
	active, max := jc.rateLimiter.GetStats()
	stats["rate_limiter"] = map[string]interface{}{
		"active_requests": active,
		"max_requests":    max,
	}

	// Configuration stats
	stats["config"] = map[string]interface{}{
		"locations":       jc.config.App.Locations,
		"cron_schedule":   jc.config.App.CronSchedule,
		"groq_configured": jc.config.Groq.APIKey != "",
		"smtp_configured": jc.notifier.IsConfigured(),
		"cv_loaded":       jc.cvReader.IsLoaded(),
	}

	return stats
} 