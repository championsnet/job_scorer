package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"job-scorer/api"
	"job-scorer/config"
	"job-scorer/controller"
	"job-scorer/multitenant"
	"job-scorer/utils"
)

// checkStartupFlag checks if the application has already run recently (within 30 minutes)
func checkStartupFlag(dataDir string) bool {
	flagFile := filepath.Join(dataDir, "startup_flag.txt")

	// Check if flag file exists and was created recently
	if info, err := os.Stat(flagFile); err == nil {
		// Allow rerun if more than 30 minutes have passed
		timeSinceLastRun := time.Since(info.ModTime())
		return timeSinceLastRun < 30*time.Minute
	}

	return false
}

// createStartupFlag creates a flag file to mark that startup processing has run
func createStartupFlag(dataDir string) error {
	flagFile := filepath.Join(dataDir, "startup_flag.txt")

	// Ensure directory exists
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return err
	}

	// Create flag file with current timestamp
	content := fmt.Sprintf("Startup processing completed at %s\n", time.Now().Format(time.RFC3339))
	return os.WriteFile(flagFile, []byte(content), 0644)
}

func main() {
	logger := utils.NewLogger("Main")

	logger.Info("🚀 Starting Job Scorer application...")

	mode := strings.ToLower(strings.TrimSpace(os.Getenv("APP_MODE")))
	if mode == "web" || mode == "worker" || mode == "migrate" || mode == "import" {
		logger.Info("Starting multi-tenant service mode: %s", mode)
		if err := multitenant.Run(context.Background()); err != nil {
			logger.Error("Multi-tenant mode failed: %v", err)
			os.Exit(1)
		}
		return
	}

	// Get port from environment variable (Cloud Run requirement)
	port := os.Getenv("PORT")
	if port == "" {
		port = "8008"
	}

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		logger.Error("Failed to load configuration: %v", err)
		// Start a basic HTTP server even if config fails
		startBasicServer(port, logger)
		return
	}

	// Validate configuration
	if err := validateConfig(cfg); err != nil {
		logger.Error("Configuration validation failed: %v", err)
		// Start a basic HTTP server even if validation fails
		startBasicServer(port, logger)
		return
	}

	// Create job controller
	jobController, err := controller.NewJobController(cfg)
	if err != nil {
		logger.Error("Failed to create job controller: %v", err)
		// Start a basic HTTP server even if controller creation fails
		startBasicServer(port, logger)
		return
	}

	// Print application status
	printApplicationStatus(cfg, jobController, logger)

	// Setup HTTP endpoints
	setupHTTPHandlers(cfg, jobController, logger)
	setupAPIHandlers(jobController)

	// Run jobs processing initially if RUN_ON_STARTUP is true
	if cfg.App.RunOnStartup {
		// Check if we're in development mode by looking for Air or development indicators
		isDevMode := os.Getenv("AIR_MAIN_BINARY") != "" || os.Getenv("DEV_MODE") == "true"

		// Check if we've already run startup processing recently (within 30 minutes)
		hasRunRecently := checkStartupFlag(cfg.App.DataDir)

		if hasRunRecently {
			logger.Info("Skipping initial job processing (already ran within 30 minutes)")
		} else {
			logger.Info("Running initial job processing (RUN_ON_STARTUP=true)")
			go func() {
				time.Sleep(5 * time.Second) // Give server time to start
				if err := jobController.SearchAndFilterJobs(); err != nil {
					logger.Error("Failed to process jobs: %v", err)
				} else {
					// Mark that we've run startup processing
					if err := createStartupFlag(cfg.App.DataDir); err != nil {
						logger.Warning("Failed to create startup flag: %v", err)
					}
				}
			}()
		}

		// In dev mode, run the job on a schedule (internal cron) since Cloud Scheduler isn't available
		if isDevMode {
			intervalMinutes := 60
			if v := os.Getenv("DEV_CRON_INTERVAL_MINUTES"); v != "" {
				if n, err := strconv.Atoi(v); err == nil {
					intervalMinutes = n
				}
			}
			if intervalMinutes > 0 {
				interval := time.Duration(intervalMinutes) * time.Minute
				logger.Info("⏰ Dev cron enabled: running job every %s", interval)
				go func() {
					ticker := time.NewTicker(interval)
					defer ticker.Stop()
					for range ticker.C {
						logger.Info("🔄 Dev cron: running job processing")
						if err := jobController.SearchAndFilterJobs(); err != nil {
							logger.Error("Dev cron failed: %v", err)
						}
					}
				}()
			}
		}
	} else {
		logger.Info("Skipping initial job processing (RUN_ON_STARTUP=false)")
	}

	logger.Info("Starting HTTP server on port %s", port)
	logger.Info("🔗 Health check: http://localhost:%s/health", port)
	logger.Info("🔗 Manual run: http://localhost:%s/run", port)
	logger.Info("🔗 Stats: http://localhost:%s/stats", port)
	logger.Info("💡 For scheduled runs, use Google Cloud Scheduler to call the /run endpoint")
	logger.Info("   Example cron expression: %s", cfg.App.CronSchedule)

	// Start HTTP server (blocking)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		logger.Error("Failed to start HTTP server: %v", err)
		os.Exit(1)
	}
}

func startBasicServer(port string, logger *utils.Logger) {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Job Scorer is running (configuration error - check logs)"))
	})

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	logger.Info("Starting basic HTTP server on port %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		logger.Error("Failed to start HTTP server: %v", err)
		os.Exit(1)
	}
}

func setupHTTPHandlers(cfg *config.Config, jobController *controller.JobController, logger *utils.Logger) {
	// Root endpoint
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		html := `
<!DOCTYPE html>
<html>
<head>
    <title>Job Scorer</title>
    <style>
        body { font-family: Arial, sans-serif; max-width: 800px; margin: 0 auto; padding: 20px; }
        .endpoint { background: #f5f5f5; padding: 10px; margin: 10px 0; border-radius: 5px; }
        .method { background: #007acc; color: white; padding: 2px 8px; border-radius: 3px; font-size: 12px; }
    </style>
</head>
<body>
    <h1>🎯 Job Scorer API</h1>
    <p>Job scoring and notification service is running!</p>
    
    <h2>📊 Available Endpoints:</h2>
    <div class="endpoint">
        <span class="method">GET</span> <strong>/health</strong> - Health check
    </div>
    <div class="endpoint">
        <span class="method">POST</span> <strong>/run</strong> - Trigger job processing manually
    </div>
    <div class="endpoint">
        <span class="method">GET</span> <strong>/stats</strong> - View application statistics
    </div>
    <h2>📡 JSON API (for frontend):</h2>
    <div class="endpoint">
        <span class="method">POST</span> <strong>/api/runs</strong> - Trigger pipeline run (async)
    </div>
    <div class="endpoint">
        <span class="method">GET</span> <strong>/api/runs/requests/{requestId}</strong> - Get run request status
    </div>
    <div class="endpoint">
        <span class="method">GET</span> <strong>/api/runs</strong> - List recent runs
    </div>
    <div class="endpoint">
        <span class="method">GET</span> <strong>/api/runs/{runId}</strong> - Get run details
    </div>
    <div class="endpoint">
        <span class="method">GET</span> <strong>/api/runs/{runId}/stages/{stage}</strong> - Get jobs by stage
    </div>
    <div class="endpoint">
        <span class="method">GET</span> <strong>/api/analytics/overview</strong> - Dashboard analytics
    </div>
    <div class="endpoint">
        <span class="method">GET</span> <strong>/api/jobs/search?run_id=&amp;stage=</strong> - Search jobs
    </div>
    
    <h2>⏰ Scheduled Execution:</h2>
    <p>This service is designed to be triggered by Google Cloud Scheduler.</p>
    <p>Configure your scheduler to call: <code>POST /run</code></p>
    <p>Recommended schedule: <code>` + cfg.App.CronSchedule + `</code></p>
    
    <h2>🔧 Configuration:</h2>
    <ul>
        <li>Locations: ` + strings.Join(cfg.App.Locations, ", ") + `</li>
        <li>Cron Schedule: ` + cfg.App.CronSchedule + `</li>
        <li>Run on Startup: ` + fmt.Sprintf("%t", cfg.App.RunOnStartup) + `</li>
    </ul>
</body>
</html>`
		w.Write([]byte(html))
	})

	// Health check endpoint
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok","service":"job-scorer"}`))
	})

	// Job processing endpoint (triggered by Cloud Scheduler)
	http.HandleFunc("/run", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost && r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		logger.Info("🔄 Received request to run job processing (triggered via HTTP)")

		start := time.Now()
		if err := jobController.SearchAndFilterJobs(); err != nil {
			logger.Error("Failed to process jobs: %v", err)
			http.Error(w, fmt.Sprintf("Failed to process jobs: %v", err), http.StatusInternalServerError)
			return
		}

		duration := time.Since(start)
		message := fmt.Sprintf("✅ Job processing completed successfully in %v", duration)
		logger.Info("%s", message)

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(fmt.Sprintf(`{"status":"success","message":"%s","duration":"%v"}`, message, duration)))
	})

	// Statistics endpoint
	http.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
		stats := jobController.GetStats()

		w.Header().Set("Content-Type", "text/html")
		html := `
<!DOCTYPE html>
<html>
<head>
    <title>Job Scorer Stats</title>
    <style>
        body { font-family: Arial, sans-serif; max-width: 800px; margin: 0 auto; padding: 20px; }
        .stat-section { background: #f9f9f9; padding: 15px; margin: 10px 0; border-radius: 5px; }
        .stat-item { margin: 5px 0; }
        .badge { background: #28a745; color: white; padding: 2px 8px; border-radius: 12px; font-size: 12px; }
        .badge.false { background: #dc3545; }
    </style>
</head>
<body>
    <h1>📊 Job Scorer Statistics</h1>
    <div class="stat-section">
        <h3>🔧 Configuration</h3>`

		if configStats, ok := stats["config"].(map[string]interface{}); ok {
			html += fmt.Sprintf(`
        <div class="stat-item">Locations: %v</div>
        <div class="stat-item">Cron Schedule: %v</div>
        <div class="stat-item">LLM API: <span class="badge %v">%v</span></div>
        <div class="stat-item">SMTP: <span class="badge %v">%v</span></div>
        <div class="stat-item">CV Loaded: <span class="badge %v">%v</span></div>`,
				configStats["locations"],
				configStats["cron_schedule"],
				configStats["llm_configured"], configStats["llm_configured"],
				configStats["smtp_configured"], configStats["smtp_configured"],
				configStats["cv_loaded"], configStats["cv_loaded"])
		}

		html += `
    </div>
    <div class="stat-section">
        <h3>🧮 LLM Usage</h3>`

		if llmUsage, ok := stats["llm_usage"].(map[string]interface{}); ok {
			html += fmt.Sprintf(`
        <div class="stat-item">Calls: %v</div>
        <div class="stat-item">Input Tokens: %v</div>
        <div class="stat-item">Non-cached Input Tokens: %v</div>
        <div class="stat-item">Cached Input Tokens: %v</div>
        <div class="stat-item">Billable Input Tokens: %v</div>
        <div class="stat-item">Output Tokens: %v</div>
        <div class="stat-item">Total Tokens: %v</div>`,
				llmUsage["calls"],
				llmUsage["input_tokens"],
				llmUsage["non_cached_input_tokens"],
				llmUsage["cached_input_tokens"],
				llmUsage["billable_input_tokens"],
				llmUsage["output_tokens"],
				llmUsage["total_tokens"])
		}

		html += `
    </div>
    <div class="stat-section">
        <h3>🎯 Job Tracking</h3>`

		if jobStats, ok := stats["job_tracker"].(map[string]interface{}); ok {
			html += fmt.Sprintf(`
        <div class="stat-item">Total Processed Jobs: %v</div>
        <div class="stat-item">Recent Jobs (7 days): %v</div>`,
				jobStats["total_processed_jobs"],
				jobStats["recent_jobs_7_days"])
		}

		html += `
    </div>
    <div class="stat-section">
        <h3>⚡ Rate Limiter</h3>`

		if rateLimiterStats, ok := stats["rate_limiter"].(map[string]interface{}); ok {
			html += fmt.Sprintf(`
        <div class="stat-item">Active Requests: %v / %v</div>
        <div class="stat-item">Tokens Used: %v / %v</div>`,
				rateLimiterStats["active_requests"], rateLimiterStats["max_requests"],
				rateLimiterStats["tokens_used"], rateLimiterStats["token_limit"])
		}

		html += `
    </div>
    <p><a href="/">← Back to Home</a></p>
</body>
</html>`

		w.Write([]byte(html))
	})
}

func setupAPIHandlers(jobController *controller.JobController) {
	h := api.NewHandlers(jobController)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/runs", h.PostRuns)
	mux.HandleFunc("GET /api/runs/requests/{requestId}", h.GetRunRequest)
	mux.HandleFunc("GET /api/runs", h.GetRuns)
	mux.HandleFunc("GET /api/runs/{runId}", h.GetRun)
	mux.HandleFunc("GET /api/runs/{runId}/stages/{stage}", h.GetRunStageJobs)
	mux.HandleFunc("GET /api/analytics/overview", h.GetAnalyticsOverview)
	mux.HandleFunc("GET /api/jobs/search", h.GetJobsSearch)
	http.Handle("/api/", api.CORS(mux))
}

func validateConfig(cfg *config.Config) error {
	if cfg.OpenAI.APIKey == "" {
		return fmt.Errorf("OPENAI_API_KEY is required (or GROQ_API_KEY for backward compatibility)")
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
	logger.Info("   ⏰ Cron Schedule: %s (for Google Cloud Scheduler)", cfg.App.CronSchedule)
	logger.Info("   🏃 Run on Startup: %t", cfg.App.RunOnStartup)
	logger.Info("   📄 CV Path: %s", cfg.App.CVPath)
	logger.Info("   💾 Output Directory: %s", cfg.App.OutputDir)
	logger.Info("   🤖 LLM Model: %s", cfg.OpenAI.Model)
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
		logger.Info("   ✅ LLM API: %t", configStats["llm_configured"])
		logger.Info("   ✅ SMTP: %t", configStats["smtp_configured"])
		logger.Info("   ✅ CV Loaded: %t", configStats["cv_loaded"])
	}
}
