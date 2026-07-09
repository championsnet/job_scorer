package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"job-scorer/api"
	"job-scorer/config"
	"job-scorer/controller"
	"job-scorer/setup"
	"job-scorer/utils"

	"github.com/robfig/cron/v3"
)

// checkStartupFlag reports whether an initial run happened within the last 30
// minutes, so restarts don't re-scrape immediately.
func checkStartupFlag(dataDir string) bool {
	flagFile := filepath.Join(dataDir, "startup_flag.txt")
	if info, err := os.Stat(flagFile); err == nil {
		return time.Since(info.ModTime()) < 30*time.Minute
	}
	return false
}

func createStartupFlag(dataDir string) error {
	flagFile := filepath.Join(dataDir, "startup_flag.txt")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return err
	}
	content := fmt.Sprintf("Startup processing completed at %s\n", time.Now().Format(time.RFC3339))
	return os.WriteFile(flagFile, []byte(content), 0o644)
}

// App holds the live, hot-swappable configuration and controller for local
// (single-user) mode. The setup wizard can rebuild it after the user saves
// their settings, without restarting the process.
type App struct {
	mu         sync.RWMutex
	logger     *utils.Logger
	cfg        *config.Config
	ctrl       *controller.JobController
	handlers   *api.Handlers
	apiHandler http.Handler
	scheduler  *cron.Cron
}

// triggerRun starts a scan through the single tracked path (so only one runs at
// a time and the dashboard can show progress). Used by the scheduler and setup.
func (a *App) triggerRun(trigger string) {
	a.mu.RLock()
	h := a.handlers
	a.mu.RUnlock()
	if h == nil {
		return
	}
	if req := h.StartTrackedRun(trigger); req == nil {
		a.logger.Info("Skipping %s run — a scan is already in progress", trigger)
	} else {
		a.logger.Info("Started %s run %s", trigger, req.RunID)
	}
}

func (a *App) configured() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.ctrl != nil
}

func (a *App) snapshot() (*config.Config, *controller.JobController, http.Handler) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.cfg, a.ctrl, a.apiHandler
}

// reload loads configuration from disk and rebuilds the controller. It is
// called at startup and again whenever the wizard saves new settings.
func (a *App) reload() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := validateConfig(cfg); err != nil {
		return err
	}
	ctrl, err := controller.NewJobController(cfg)
	if err != nil {
		return err
	}

	handlers := api.NewHandlers(ctrl)
	apiMux := http.NewServeMux()
	apiMux.HandleFunc("POST /api/runs", handlers.PostRuns)
	apiMux.HandleFunc("GET /api/runs/requests/{requestId}", handlers.GetRunRequest)
	apiMux.HandleFunc("GET /api/runs/active", handlers.GetActiveRun)
	apiMux.HandleFunc("GET /api/runs", handlers.GetRuns)
	apiMux.HandleFunc("GET /api/runs/{runId}", handlers.GetRun)
	apiMux.HandleFunc("GET /api/runs/{runId}/stages/{stage}", handlers.GetRunStageJobs)
	apiMux.HandleFunc("GET /api/analytics/overview", handlers.GetAnalyticsOverview)
	apiMux.HandleFunc("GET /api/jobs/search", handlers.GetJobsSearch)

	a.mu.Lock()
	a.cfg = cfg
	a.ctrl = ctrl
	a.handlers = handlers
	a.apiHandler = api.CORS(apiMux)
	a.mu.Unlock()

	printApplicationStatus(cfg, ctrl, a.logger)
	a.startBackgroundWork()
	return nil
}

// startBackgroundWork (re)builds the recurring scheduler for the current cron
// expression. It does NOT auto-run on every launch — recurring runs come from
// the scheduler and one-off runs from the dashboard or right after setup — so
// opening the app never silently kicks off a scrape. Safe to call repeatedly.
func (a *App) startBackgroundWork() {
	cfg, ctrl, _ := a.snapshot()
	if cfg == nil || ctrl == nil {
		return
	}

	if a.scheduler != nil {
		a.scheduler.Stop()
		a.scheduler = nil
	}
	expr := strings.TrimSpace(cfg.App.CronSchedule)
	if expr == "" || strings.EqualFold(expr, "manual") {
		a.logger.Info("Automatic scanning is OFF — scans run only when you click Run.")
		return
	}
	sched := cron.New()
	if _, err := sched.AddFunc(expr, func() {
		a.triggerRun("scheduled")
	}); err != nil {
		a.logger.Warning("Could not start scheduler for cron %q: %v (runs can still be triggered from the dashboard)", expr, err)
	} else {
		sched.Start()
		a.scheduler = sched
		a.logger.Info("⏰ Scheduler active: %s", expr)
	}
}

func main() {
	logger := utils.NewLogger("Main")
	logger.Info("🚀 Starting Job Scorer application...")

	mode := strings.ToLower(strings.TrimSpace(os.Getenv("APP_MODE")))
	if mode == "web" || mode == "worker" || mode == "migrate" || mode == "import" {
		logger.Info("Starting multi-tenant service mode: %s", mode)
		if err := runSaaS(context.Background()); err != nil {
			logger.Error("Multi-tenant mode failed: %v", err)
			os.Exit(1)
		}
		return
	}

	port := os.Getenv("PORT")
	if port == "" {
		if guiEnabled {
			// Desktop app: ephemeral port so every launch is a fresh origin —
			// no clashing with a lingering old process and no stale webview cache.
			port = "0"
		} else {
			port = "8008"
		}
	}

	// Where writable state (.env, config/, data/) lives. APP_HOME (used by
	// Docker) wins; otherwise the desktop app uses a per-user data dir (Finder
	// launches apps with cwd="/"), and the plain binary uses the working dir.
	home := strings.TrimSpace(os.Getenv("APP_HOME"))
	if home == "" {
		home = guiDefaultHome()
	}
	if home != "" {
		if err := os.MkdirAll(home, 0o755); err != nil {
			logger.Warning("Could not create data dir %s: %v", home, err)
		} else if err := os.Chdir(home); err != nil {
			logger.Warning("Could not switch to data dir %s: %v", home, err)
		}
	}

	baseDir, err := os.Getwd()
	if err != nil {
		baseDir = "."
	}

	app := &App{logger: logger}

	wizard := setup.New(logger, baseDir)
	wizard.OnSaved = app.reload
	wizard.IsConfigured = app.configured
	wizard.Desktop = guiEnabled
	wizard.OnRunNow = func() { app.triggerRun("setup") }

	mux := http.NewServeMux()
	wizard.RegisterRoutes(mux)

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok","service":"job-scorer"}`))
	})
	mux.HandleFunc("/", app.handleRoot(wizard))
	mux.HandleFunc("/run", app.handleRun)
	mux.HandleFunc("/stats", app.handleStats)
	mux.HandleFunc("GET /api/open", app.handleOpenExternal)
	mux.Handle("/api/", http.HandlerFunc(app.handleAPI))

	// Try to load an existing configuration. If it fails, we stay in setup
	// mode and the wizard is served at "/".
	if err := app.reload(); err != nil {
		logger.Warning("Not configured yet: %v", err)
		logger.Info("👉 Open the setup wizard to get started: http://localhost:%s/setup", port)
	} else {
		logger.Info("🔗 Dashboard:   http://localhost:%s/", port)
		logger.Info("🔗 Reconfigure: http://localhost:%s/setup", port)
	}

	// Desktop app binds localhost only (private); headless/Docker binds all
	// interfaces so a container port mapping can reach it.
	bindAddr := ":" + port
	if guiEnabled {
		bindAddr = "127.0.0.1:" + port
	}
	ln, err := net.Listen("tcp", bindAddr)
	if err != nil {
		logger.Error("Failed to bind %s: %v", bindAddr, err)
		os.Exit(1)
	}
	actualPort := ln.Addr().(*net.TCPAddr).Port
	url := fmt.Sprintf("http://localhost:%d", actualPort)
	logger.Info("Serving on %s", url)
	serverErr := make(chan error, 1)
	go func() { serverErr <- http.Serve(ln, mux) }()

	if guiEnabled {
		// Desktop build: show the app in a native window and exit when closed.
		waitReady(url)
		logger.Info("Opening the app window…")
		// Cache-bust so the webview never shows a previous build's page.
		runGUI(url+"/?v="+strconv.FormatInt(time.Now().UnixNano(), 10), "Job Scorer")
		return
	}

	// Plain binary: open the default browser automatically unless disabled
	// (NO_BROWSER=true, set in Docker). Then block on the server.
	if noBrowser, _ := strconv.ParseBool(os.Getenv("NO_BROWSER")); !noBrowser {
		go func() {
			waitReady(url)
			if err := openBrowser(url); err != nil {
				logger.Info("Open %s in your browser to continue.", url)
			}
		}()
	}
	if err := <-serverErr; err != nil {
		logger.Error("Failed to start HTTP server: %v", err)
		os.Exit(1)
	}
}

// waitReady blocks until the local server answers /health (or times out).
func waitReady(url string) {
	client := &http.Client{Timeout: time.Second}
	for i := 0; i < 100; i++ {
		if resp, err := client.Get(url + "/health"); err == nil {
			resp.Body.Close()
			return
		}
		time.Sleep(150 * time.Millisecond)
	}
}

func openBrowser(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}

func (a *App) handleRoot(wizard *setup.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		if !a.configured() {
			wizard.ServeIndex(w, r)
			return
		}
		wizard.ServeDashboard(w, r)
	}
}

func (a *App) handleRun(w http.ResponseWriter, r *http.Request) {
	_, ctrl, _ := a.snapshot()
	if ctrl == nil {
		http.Error(w, "Not configured yet — open /setup first.", http.StatusServiceUnavailable)
		return
	}
	a.logger.Info("🔄 Manual run triggered via HTTP")
	start := time.Now()
	if err := ctrl.SearchAndFilterJobs(); err != nil {
		a.logger.Error("Run failed: %v", err)
		http.Error(w, fmt.Sprintf("Run failed: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(fmt.Sprintf(`{"status":"success","duration":"%v"}`, time.Since(start))))
}

func (a *App) handleStats(w http.ResponseWriter, r *http.Request) {
	_, ctrl, _ := a.snapshot()
	if ctrl == nil {
		http.Error(w, "Not configured yet — open /setup first.", http.StatusServiceUnavailable)
		return
	}
	stats := ctrl.GetStats()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{"stats": stats}); err != nil {
		a.logger.Warning("Failed to encode stats: %v", err)
	}
}

// handleOpenExternal opens a job URL in the user's real browser — needed in the
// desktop app, where the embedded webview can't open target="_blank" links.
func (a *App) handleOpenExternal(w http.ResponseWriter, r *http.Request) {
	u := strings.TrimSpace(r.URL.Query().Get("url"))
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		http.Error(w, "invalid url", http.StatusBadRequest)
		return
	}
	if err := openBrowser(u); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"ok":true}`))
}

func (a *App) handleAPI(w http.ResponseWriter, r *http.Request) {
	_, _, apiHandler := a.snapshot()
	if apiHandler == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"error":"not configured — complete setup at /setup"}`))
		return
	}
	apiHandler.ServeHTTP(w, r)
}

func validateConfig(cfg *config.Config) error {
	if cfg.OpenAI.APIKey == "" {
		return fmt.Errorf("OPENAI_API_KEY is required")
	}
	if len(cfg.App.Locations) == 0 {
		return fmt.Errorf("at least one job location must be configured")
	}
	if cfg.App.CVPath == "" {
		return fmt.Errorf("a CV file is required")
	}
	if _, err := os.Stat(cfg.App.CVPath); os.IsNotExist(err) {
		return fmt.Errorf("CV file not found at path: %s", cfg.App.CVPath)
	}
	return nil
}

func printApplicationStatus(cfg *config.Config, jobController *controller.JobController, logger *utils.Logger) {
	logger.Info("📋 Configuration:")
	logger.Info("   📍 Locations: %v", cfg.App.Locations)
	logger.Info("   ⏰ Schedule: %s", cfg.App.CronSchedule)
	logger.Info("   📄 CV: %s", cfg.App.CVPath)
	logger.Info("   🤖 Model: %s", cfg.OpenAI.Model)
	if cfg.SMTP.Host != "" {
		logger.Info("   📧 Email: enabled (%s → %s)", cfg.SMTP.From, strings.Join(cfg.SMTP.ToRecipients, ", "))
	} else {
		logger.Info("   📧 Email: disabled")
	}
}
