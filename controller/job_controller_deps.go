package controller

import (
	"fmt"
	"time"

	"job-scorer/config"
	"job-scorer/services/cv"
	"job-scorer/services/evaluator"
	"job-scorer/services/filter"
	"job-scorer/services/notification"
	"job-scorer/services/scraper"
	"job-scorer/services/storage"
	"job-scorer/utils"
)

type JobControllerDependencies struct {
	Storage         storage.JobStorage
	Checkpoint      storage.CheckpointStorage
	RunSummaryStore storage.RunSummaryStore
	JobTracker      storage.JobTrackerInterface
	Logger          *utils.Logger
	GCSStorage      *storage.GCSStorage
}

func NewJobControllerWithDependencies(cfg *config.Config, deps JobControllerDependencies) (*JobController, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}
	if deps.Storage == nil {
		return nil, fmt.Errorf("storage dependency is required")
	}
	if deps.Checkpoint == nil {
		return nil, fmt.Errorf("checkpoint dependency is required")
	}
	if deps.RunSummaryStore == nil {
		return nil, fmt.Errorf("run summary store dependency is required")
	}
	if deps.JobTracker == nil {
		return nil, fmt.Errorf("job tracker dependency is required")
	}

	logger := deps.Logger
	if logger == nil {
		logger = utils.NewLogger("JobScorer")
	}

	linkedInScraper := scraper.NewLinkedInScraper(cfg.Policy.Scraper)
	filterService := filter.NewFilter(cfg.Policy.Filters, cfg.Policy.Notification, logger)
	cvReader := cv.NewCVReader(cfg.App.CVPath, cfg.Policy.CV)
	notifier := notification.NewEmailNotifier(cfg)

	tokenRateLimiter := utils.NewTokenRateLimiter(
		10,
		cfg.RateLimit.TimeWindow,
		cfg.RateLimit.MaxTokensPerMinute,
		time.Minute,
	)
	openAIClient := evaluator.NewOpenAIClient(cfg, tokenRateLimiter)
	evaluatorService := evaluator.NewEvaluator(openAIClient, cvReader, linkedInScraper, tokenRateLimiter, cfg.Policy, logger)

	if err := deps.Storage.EnsureOutputDir(); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	return &JobController{
		config:          cfg,
		scraper:         linkedInScraper,
		filter:          filterService,
		evaluator:       evaluatorService,
		cvReader:        cvReader,
		notifier:        notifier,
		storage:         deps.Storage,
		checkpoint:      deps.Checkpoint,
		runSummaryStore: deps.RunSummaryStore,
		jobTracker:      deps.JobTracker,
		rateLimiter:     tokenRateLimiter,
		logger:          logger,
		gcsStorage:      deps.GCSStorage,
	}, nil
}
