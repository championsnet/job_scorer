package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"job-scorer/config"
	"job-scorer/controller"
	"job-scorer/utils"

	"github.com/robfig/cron/v3"
)

func main() {
	logger := utils.NewLogger("Main")
	
	logger.Info("🚀 Starting Job Scorer application...")

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		logger.Error("Failed to load configuration: %v", err)
		return
	}

	// Validate configuration
	if err := validateConfig(cfg); err != nil {
		logger.Error("Configuration validation failed: %v: %v", err)
		os.Exit(1)
	}

	// Create job controller
	controller, err := controller.NewJobController(cfg)
	if err != nil {
		logger.Error("Failed to create job controller: %v", err)
		return
	}

	// Print application status
	printApplicationStatus(cfg, controller, logger)

	// Run jobs processing initially if RUN_ON_STARTUP is true
	if cfg.App.RunOnStartup {
		logger.Info("Running initial job processing (RUN_ON_STARTUP=true)")
		if err := controller.SearchAndFilterJobs(); err != nil {
			logger.Error("Failed to process jobs: %v", err)
		}
	} else {
		logger.Info("Skipping initial job processing (RUN_ON_STARTUP=false)")
	}

	// Setup cron scheduler using configured schedule
	c := cron.New()
	_, err = c.AddFunc(cfg.App.CronSchedule, func() {
		logger.Info("Running scheduled job processing (cron: %s)", cfg.App.CronSchedule)
		if err := controller.SearchAndFilterJobs(); err != nil {
			logger.Error("Scheduled job processing failed: %v", err)
		}
	})
	if err != nil {
		logger.Error("Failed to setup cron job with schedule '%s': %v", cfg.App.CronSchedule, err)
		return
	}

	// Start the cron scheduler
	logger.Info("Starting cron scheduler with schedule: %s", cfg.App.CronSchedule)
	c.Start()
	defer c.Stop()

	// Setup HTTP server for health checks and manual triggers
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})
	
	// Manual trigger endpoint for testing
	http.HandleFunc("/trigger", func(w http.ResponseWriter, r *http.Request) {
		logger.Info("Manual job processing triggered via HTTP")
		if err := controller.SearchAndFilterJobs(); err != nil {
			logger.Error("Manual job processing failed: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Job processing failed"))
		} else {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Job processing completed"))
		}
	})

	// Start HTTP server
	port := ":8080"
	logger.Info("Starting HTTP server on port %s", port)
	if err := http.ListenAndServe(port, nil); err != nil {
		logger.Error("HTTP server failed: %v", err)
	}
}

func validateConfig(cfg *config.Config) error {
	if cfg.Groq.APIKey == "" {
		return fmt.Errorf("GROQ_API_KEY is required")
	}

	if len(cfg.App.Locations) == 0 {
		return fmt.Errorf("at least one job location must be configured")
	}

	if cfg.App.CVPath == "" {
		return fmt.Errorf("CV_PATH is required")
	}

	// Check if CV file exists
	if _, err := os.Stat(cfg.App.CVPath); os.IsNotExist(err) {
		return fmt.Errorf("CV file not found at path: %s", cfg.App.CVPath)
	}

	return nil
}

func printApplicationStatus(cfg *config.Config, jobController *controller.JobController, logger *utils.Logger) {
	logger.Info("📋 Application Configuration:")
	logger.Info("   📍 Job Locations: %v", cfg.App.Locations)
	logger.Info("   ⏰ Cron Schedule: %s", cfg.App.CronSchedule)
	logger.Info("   🏃 Run on Startup: %t", cfg.App.RunOnStartup)
	logger.Info("   📄 CV Path: %s", cfg.App.CVPath)
	logger.Info("   💾 Output Directory: %s", cfg.App.OutputDir)
	logger.Info("   🤖 Groq Model: %s", cfg.Groq.Model)
	logger.Info("   🚦 Rate Limit: %d requests per minute", cfg.RateLimit.MaxRequests)
	
	if cfg.SMTP.Host != "" {
		recipients := strings.Join(cfg.SMTP.ToRecipients, ", ")
		logger.Info("   📧 Email Notifications: Enabled (%s → %s)", cfg.SMTP.From, recipients)
	} else {
		logger.Warning("   📧 Email Notifications: Disabled (SMTP not configured)")
	}

	// Print stats
	stats := jobController.GetStats()
	if configStats, ok := stats["config"].(map[string]interface{}); ok {
		logger.Info("📊 Service Status:")
		logger.Info("   ✅ Groq API: %t", configStats["groq_configured"])
		logger.Info("   ✅ SMTP: %t", configStats["smtp_configured"])
		logger.Info("   ✅ CV Loaded: %t", configStats["cv_loaded"])
	}
} 