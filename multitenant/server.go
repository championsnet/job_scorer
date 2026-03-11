package multitenant

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/google/uuid"

	"job-scorer/config"
	"job-scorer/controller"
	cvreader "job-scorer/services/cv"
	"job-scorer/models"
	"job-scorer/services/storage"
	"job-scorer/utils"
)

type Server struct {
	runtime     *RuntimeConfig
	baseConfig  *config.Config
	firestore   *firestore.Client
	repo        *Repository
	auth        *AuthService
	enqueuer    RunEnqueuer
	objectStore *storage.GCSStorage
	logger      *utils.Logger
}

func NewServer(ctx context.Context, runtimeCfg *RuntimeConfig) (*Server, error) {
	baseCfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed loading base config: %w", err)
	}
	baseCfg.App.RunOnStartup = false

	fsClient, err := OpenFirestore(ctx, runtimeCfg)
	if err != nil {
		return nil, err
	}

	repo := NewRepository(fsClient, runtimeCfg)
	authService, err := NewAuthService(ctx, runtimeCfg, repo)
	if err != nil {
		return nil, err
	}
	store, err := NewObjectStorage(runtimeCfg)
	if err != nil {
		return nil, err
	}
	logger := utils.NewLogger("MultiTenant")

	return &Server{
		runtime:     runtimeCfg,
		baseConfig:  baseCfg,
		firestore:   fsClient,
		repo:        repo,
		auth:        authService,
		objectStore: store,
		logger:      logger,
	}, nil
}

func (s *Server) Close() error {
	var errs []string
	if s.enqueuer != nil {
		if err := s.enqueuer.Close(); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if s.objectStore != nil {
		if err := s.objectStore.Close(); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if s.firestore != nil {
		if err := s.firestore.Close(); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("close errors: %s", strings.Join(errs, "; "))
	}
	return nil
}

func (s *Server) ConfigureEnqueuer(ctx context.Context) error {
	enqueuer, err := NewRunEnqueuer(ctx, s.runtime)
	if err != nil {
		return err
	}
	s.enqueuer = enqueuer
	return nil
}

func (s *Server) WebHandler() http.Handler {
	mux := http.NewServeMux()

	// Public endpoints.
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("POST /api/v1/billing/webhook", s.handleStripeWebhook)
	mux.HandleFunc("POST /internal/dispatch", s.handleDispatchDueRuns)

	// Authenticated API.
	mux.HandleFunc("GET /api/v1/me", s.auth.Middleware(s.handleMe))
	mux.HandleFunc("GET /api/v1/settings", s.auth.Middleware(s.handleGetSettings))
	mux.HandleFunc("PUT /api/v1/settings", s.auth.Middleware(s.handleUpdateSettings))
	mux.HandleFunc("POST /api/v1/settings/cv", s.auth.Middleware(s.handleUploadCV))
	mux.HandleFunc("GET /api/v1/settings/cv", s.auth.Middleware(s.handleGetActiveCV))
	mux.HandleFunc("GET /api/v1/runs", s.auth.Middleware(s.handleListRuns))
	mux.HandleFunc("POST /api/v1/runs", s.auth.Middleware(s.handleCreateRun))
	mux.HandleFunc("GET /api/v1/runs/requests/{requestId}", s.auth.Middleware(s.handleRunRequestStatus))
	mux.HandleFunc("GET /api/v1/runs/{runId}", s.auth.Middleware(s.handleGetRun))
	mux.HandleFunc("GET /api/v1/runs/{runId}/stages/{stage}", s.auth.Middleware(s.handleGetRunStage))
	mux.HandleFunc("GET /api/v1/analytics/overview", s.auth.Middleware(s.handleAnalyticsOverview))
	mux.HandleFunc("GET /api/v1/billing/summary", s.auth.Middleware(s.handleBillingSummary))
	mux.HandleFunc("POST /api/v1/billing/checkout", s.auth.Middleware(s.handleBillingCheckout))

	// Backward compatibility routes for current dashboard.
	mux.HandleFunc("GET /api/analytics/overview", s.auth.Middleware(s.handleAnalyticsOverview))
	mux.HandleFunc("GET /api/runs", s.auth.Middleware(s.handleListRuns))
	mux.HandleFunc("POST /api/runs", s.auth.Middleware(s.handleCreateRun))
	mux.HandleFunc("GET /api/runs/requests/{requestId}", s.auth.Middleware(s.handleRunRequestStatus))
	mux.HandleFunc("GET /api/runs/{runId}", s.auth.Middleware(s.handleGetRun))
	mux.HandleFunc("GET /api/runs/{runId}/stages/{stage}", s.auth.Middleware(s.handleGetRunStage))

	s.registerSPAFallback(mux)
	return withRuntimeCORS(s.runtime.CORSOrigins, mux)
}

func (s *Server) WorkerHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("POST /internal/tasks/run", s.handleWorkerRunTask)
	return mux
}

func (s *Server) registerSPAFallback(mux *http.ServeMux) {
	distDir := strings.TrimSpace(s.runtime.FrontendDistDir)
	if distDir == "" {
		return
	}
	indexPath := filepath.Join(distDir, "index.html")
	if _, err := os.Stat(indexPath); err != nil {
		mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"service": "job-scorer-multitenant",
				"status":  "ok",
				"mode":    s.runtime.Mode,
			})
		})
		return
	}

	fileServer := http.FileServer(http.Dir(distDir))
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/internal/") || r.URL.Path == "/health" {
			http.NotFound(w, r)
			return
		}
		filePath := filepath.Join(distDir, filepath.Clean(r.URL.Path))
		if info, err := os.Stat(filePath); err == nil && !info.IsDir() {
			fileServer.ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, indexPath)
	}))
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	_, err := s.firestore.Collection("accounts").Limit(1).Documents(r.Context()).GetAll()
	if err != nil {
		s.logger.Error("Health check Firestore query failed: %v", err)
		writeJSONError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "ok",
		"mode":   s.runtime.Mode,
	})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	user, ok := CurrentUser(r)
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	balance, err := s.repo.GetCreditBalance(r.Context(), user.AccountID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed loading credit balance")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"user_id":         user.UserID,
		"account_id":      user.AccountID,
		"firebase_uid":    user.FirebaseUID,
		"email":           user.Email,
		"email_verified":  user.EmailVerified,
		"credit_balance":  balance,
		"auth_bypass":     s.runtime.AuthBypass,
		"run_credit_cost": s.runtime.RunCreditCost,
	})
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	user, ok := CurrentUser(r)
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	settings, err := s.repo.GetAccountSettings(r.Context(), user.AccountID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed loading settings")
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (s *Server) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	user, ok := CurrentUser(r)
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	var payload AccountSettings
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}
	if strings.TrimSpace(payload.ScheduleCron) == "" {
		payload.ScheduleCron = "0 */1 * * *"
	}
	if strings.TrimSpace(payload.ScheduleTimezone) == "" {
		payload.ScheduleTimezone = "UTC"
	}
	if payload.MaxJobs <= 0 {
		payload.MaxJobs = s.runtime.DefaultMaxJobs
	}
	if reflect.DeepEqual(payload.Policy, config.Policy{}) {
		payload.Policy = config.DefaultPolicy()
	}
	if err := validatePolicy(payload.Policy); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	updated, err := s.repo.UpsertAccountSettings(r.Context(), user.AccountID, user.UserID, &payload)
	if err != nil {
		errText := strings.ToLower(err.Error())
		if strings.Contains(errText, "invalid cron") || strings.Contains(errText, "invalid timezone") {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "failed updating settings")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleUploadCV(w http.ResponseWriter, r *http.Request) {
	user, ok := CurrentUser(r)
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid multipart payload")
		return
	}
	file, header, err := r.FormFile("cv")
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "cv form field is required")
		return
	}
	defer file.Close()
	body, err := io.ReadAll(file)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed reading uploaded file")
		return
	}
	if len(body) == 0 {
		writeJSONError(w, http.StatusBadRequest, "empty CV file")
		return
	}

	objectPath, sha, err := SaveAccountCV(s.objectStore, s.runtime, user.AccountID, header.Filename, body)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed storing CV file")
		return
	}
	mimeType := strings.TrimSpace(header.Header.Get("Content-Type"))
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	// Extract and cache CV text at upload time so the worker never has to re-parse the PDF.
	extractedText := s.extractCVText(body, header.Filename)
	if extractedText == "" {
		s.logger.Warning("CV upload for account %s: text extraction produced no output (PDF may use unsupported encoding)", user.AccountID)
	} else {
		s.logger.Info("CV upload for account %s: extracted %d chars of text", user.AccountID, len(extractedText))
	}

	cv, err := s.repo.SaveAccountCV(r.Context(), user.AccountID, objectPath, mimeType, sha, extractedText, int64(len(body)))
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed saving CV metadata")
		return
	}
	writeJSON(w, http.StatusCreated, cv)
}

// extractCVText writes data to a temp file, runs the CV parsers, and returns the
// extracted text. Returns "" if extraction fails or produces only fallback text.
func (s *Server) extractCVText(data []byte, filename string) string {
	ext := strings.ToLower(filepath.Ext(strings.TrimSpace(filename)))
	if ext == "" {
		ext = ".bin"
	}
	tmpFile, err := os.CreateTemp("", "cv-extract-*"+ext)
	if err != nil {
		s.logger.Warning("extractCVText: failed creating temp file: %v", err)
		return ""
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		s.logger.Warning("extractCVText: failed writing temp file: %v", err)
		return ""
	}
	tmpFile.Close()

	reader := cvreader.NewCVReader(tmpPath, s.baseConfig.Policy.CV)
	text, err := reader.LoadCV()
	if err != nil {
		s.logger.Warning("extractCVText: CVReader error: %v", err)
		return ""
	}
	// Discard fallback sentinel — it means no real text was found.
	fallback := strings.TrimSpace(s.baseConfig.Policy.CV.FallbackText)
	if fallback != "" && strings.TrimSpace(text) == fallback {
		s.logger.Warning("extractCVText: extraction fell back to fallback text, treating as empty")
		return ""
	}
	return text
}

func (s *Server) handleGetActiveCV(w http.ResponseWriter, r *http.Request) {
	user, ok := CurrentUser(r)
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	cv, err := s.repo.GetActiveCV(r.Context(), user.AccountID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed loading active CV")
		return
	}
	if cv == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"hasCV": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"hasCV":         true,
		"id":            cv.ID,
		"storagePath":   cv.StoragePath,
		"mimeType":      cv.MimeType,
		"sizeBytes":     cv.SizeBytes,
		"createdAt":     cv.CreatedAt,
		"textExtracted": strings.TrimSpace(cv.ExtractedText) != "",
	})
}

func (s *Server) handleCreateRun(w http.ResponseWriter, r *http.Request) {
	user, ok := CurrentUser(r)
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	var body struct {
		ForceReeval bool `json:"force_reeval"`
	}
	// Best-effort parse; an empty body or non-JSON body is fine (force_reeval defaults to false).
	if ct := r.Header.Get("Content-Type"); strings.Contains(ct, "application/json") {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	settings, err := s.repo.GetAccountSettings(r.Context(), user.AccountID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed loading account settings")
		return
	}
	runKey := fmt.Sprintf("manual:%s:%s", user.AccountID, uuid.NewString())
	runReq, err := s.repo.CreateQueuedRun(
		r.Context(),
		user.AccountID,
		&user.UserID,
		"manual",
		runKey,
		settings,
		s.runtime.RunCreditCost,
		body.ForceReeval,
	)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "insufficient credits") {
			writeJSONError(w, http.StatusPaymentRequired, err.Error())
			return
		}
		if strings.Contains(strings.ToLower(err.Error()), "active run") {
			writeJSONError(w, http.StatusConflict, err.Error())
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "failed creating run")
		return
	}
	if err := s.enqueuer.EnqueueRun(r.Context(), runReq.RunID); err != nil {
		_ = s.markRunFailed(r.Context(), user.AccountID, runReq.RunID, fmt.Sprintf("failed enqueueing run: %v", err))
		s.logger.Error("Failed enqueueing run %s for account %s: %v", runReq.RunID, user.AccountID, err)
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed enqueueing run: %v", err))
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"status":     "accepted",
		"run_id":     runReq.RunID,
		"request_id": runReq.RequestID,
		"status_url": "/api/v1/runs/requests/" + runReq.RequestID,
		"message":    "pipeline run queued",
	})
}

func (s *Server) handleRunRequestStatus(w http.ResponseWriter, r *http.Request) {
	user, ok := CurrentUser(r)
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	requestID := strings.TrimSpace(r.PathValue("requestId"))
	if requestID == "" {
		writeJSONError(w, http.StatusBadRequest, "request ID is required")
		return
	}
	reqStatus, err := s.repo.GetRunByRequestID(r.Context(), user.AccountID, requestID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed reading request status")
		return
	}
	if reqStatus == nil {
		writeJSONError(w, http.StatusNotFound, "request not found")
		return
	}
	writeJSON(w, http.StatusOK, reqStatus)
}

func (s *Server) handleListRuns(w http.ResponseWriter, r *http.Request) {
	user, ok := CurrentUser(r)
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	limit := 50
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if parsed, err := strconvAtoi(raw); err == nil && parsed > 0 && parsed <= 200 {
			limit = parsed
		}
	}
	runs, err := s.repo.ListRuns(r.Context(), user.AccountID, limit)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed loading runs")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"runs": runs})
}

func (s *Server) handleGetRun(w http.ResponseWriter, r *http.Request) {
	user, ok := CurrentUser(r)
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	runID := strings.TrimSpace(r.PathValue("runId"))
	if runID == "" {
		writeJSONError(w, http.StatusBadRequest, "run ID is required")
		return
	}
	run, err := s.repo.GetRun(r.Context(), user.AccountID, runID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed loading run")
		return
	}
	if run == nil {
		writeJSONError(w, http.StatusNotFound, "run not found")
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (s *Server) handleGetRunStage(w http.ResponseWriter, r *http.Request) {
	user, ok := CurrentUser(r)
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	runID := strings.TrimSpace(r.PathValue("runId"))
	stage := strings.TrimSpace(r.PathValue("stage"))
	if runID == "" || stage == "" {
		writeJSONError(w, http.StatusBadRequest, "run ID and stage are required")
		return
	}
	included, err := s.repo.LoadStageSnapshot(r.Context(), user.AccountID, runID, stage)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed loading stage jobs")
		return
	}
	if included == nil {
		included = []*models.Job{}
	}
	excluded, err := s.repo.LoadStageSnapshot(r.Context(), user.AccountID, runID, stage+"_excluded")
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed loading excluded stage jobs")
		return
	}
	if excluded == nil {
		excluded = []*models.Job{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"jobs":           included,
		"count":          len(included),
		"included_jobs":  included,
		"included_count": len(included),
		"excluded_jobs":  excluded,
		"excluded_count": len(excluded),
		"base_count":     len(included) + len(excluded),
	})
}

func (s *Server) handleAnalyticsOverview(w http.ResponseWriter, r *http.Request) {
	user, ok := CurrentUser(r)
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	data, err := s.repo.GetAnalyticsOverview(r.Context(), user.AccountID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed loading analytics")
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func (s *Server) handleBillingSummary(w http.ResponseWriter, r *http.Request) {
	user, ok := CurrentUser(r)
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	summary, err := s.repo.GetBillingSummary(r.Context(), user.AccountID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed loading billing summary")
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (s *Server) handleDispatchDueRuns(w http.ResponseWriter, r *http.Request) {
	if !s.isAuthorizedSchedulerRequest(r) {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized scheduler request")
		return
	}
	now := time.Now().UTC()
	due, err := s.repo.ListDueSchedules(r.Context(), now, 200)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed listing due schedules")
		return
	}

	queued := 0
	skipped := 0
	failures := 0
	for _, item := range due {
		settings, err := s.repo.GetAccountSettings(r.Context(), item.AccountID)
		if err != nil {
			failures++
			continue
		}
		runKey := fmt.Sprintf("scheduled:%s:%s", item.AccountID, now.Format("200601021504"))
		runReq, err := s.repo.CreateQueuedRun(r.Context(), item.AccountID, nil, "scheduled", runKey, settings, s.runtime.RunCreditCost, false)
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "insufficient credits") || strings.Contains(strings.ToLower(err.Error()), "active run") {
				skipped++
				_ = s.repo.AdvanceSchedule(r.Context(), item.AccountID, item.ScheduleCron, item.Timezone, now)
				continue
			}
			failures++
			continue
		}
		if err := s.enqueuer.EnqueueRun(r.Context(), runReq.RunID); err != nil {
			failures++
			_ = s.markRunFailed(r.Context(), item.AccountID, runReq.RunID, fmt.Sprintf("failed enqueueing scheduled run: %v", err))
			continue
		}
		_ = s.repo.AdvanceSchedule(r.Context(), item.AccountID, item.ScheduleCron, item.Timezone, now)
		queued++
	}
	writeJSON(w, http.StatusOK, map[string]int{
		"queued":   queued,
		"skipped":  skipped,
		"failures": failures,
	})
}

func (s *Server) handleWorkerRunTask(w http.ResponseWriter, r *http.Request) {
	if !s.isAuthorizedWorkerRequest(r) {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized worker request")
		return
	}
	var payload struct {
		RunID string `json:"run_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid worker payload")
		return
	}
	payload.RunID = strings.TrimSpace(payload.RunID)
	if payload.RunID == "" {
		writeJSONError(w, http.StatusBadRequest, "run_id is required")
		return
	}

	if err := s.executeRun(r.Context(), payload.RunID); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
		"run_id": payload.RunID,
	})
}

func (s *Server) executeRun(ctx context.Context, runID string) error {
	run, accountID, err := s.repo.GetRunByID(ctx, runID)
	if err != nil {
		return fmt.Errorf("failed loading run row: %w", err)
	}
	if run == nil {
		return fmt.Errorf("run %s not found", runID)
	}
	if run.Status == models.RunStatusSuccess {
		return nil
	}
	if err := s.repo.MarkRunRunning(ctx, runID); err != nil {
		return fmt.Errorf("failed marking run as running: %w", err)
	}

	settings, err := s.repo.GetAccountSettings(ctx, accountID)
	if err != nil {
		_ = s.markRunFailed(ctx, accountID, runID, fmt.Sprintf("failed loading account settings: %v", err))
		return fmt.Errorf("failed loading account settings: %w", err)
	}
	cvMeta, err := s.repo.GetActiveCV(ctx, accountID)
	if err != nil {
		_ = s.markRunFailed(ctx, accountID, runID, "no active CV uploaded for this account")
		return fmt.Errorf("failed loading active CV: %w", err)
	}
	if cvMeta == nil {
		_ = s.markRunFailed(ctx, accountID, runID, "no active CV uploaded for this account")
		return fmt.Errorf("no active CV uploaded for account %s", accountID)
	}

	// Prefer pre-extracted text (cached at upload time) over re-parsing the PDF on every run.
	var cvPath string
	var cleanup func()
	if strings.TrimSpace(cvMeta.ExtractedText) != "" {
		s.logger.Info("Run %s: using cached CV text (%d chars)", runID, len(cvMeta.ExtractedText))
		tmpFile, tmpErr := os.CreateTemp("", "cv-text-*.txt")
		if tmpErr == nil {
			if _, writeErr := tmpFile.Write([]byte(cvMeta.ExtractedText)); writeErr == nil {
				tmpFile.Close()
				cvPath = tmpFile.Name()
				cleanup = func() { _ = os.Remove(cvPath) }
			} else {
				tmpFile.Close()
				_ = os.Remove(tmpFile.Name())
			}
		}
	}
	if cvPath == "" {
		s.logger.Info("Run %s: no cached CV text, materializing PDF from GCS", runID)
		cvPath, cleanup, err = MaterializeAccountCV(s.objectStore, cvMeta.StoragePath)
		if err != nil {
			_ = s.markRunFailed(ctx, accountID, runID, fmt.Sprintf("failed downloading CV object: %v", err))
			return fmt.Errorf("failed materializing CV file: %w", err)
		}
	}
	defer cleanup()

	runConfig := s.buildRunConfig(settings, cvPath, run.ForceReeval)
	deps := controller.JobControllerDependencies{
		Storage:         storage.NewFirestoreJobStorage(s.firestore, accountID),
		Checkpoint:      storage.NewFirestoreCheckpointService(s.firestore, accountID),
		RunSummaryStore: storage.NewFirestoreRunSummaryStore(s.firestore, accountID),
		JobTracker:      storage.NewFirestoreJobTracker(s.firestore, accountID),
		Logger:          utils.NewLogger("JobScorer-" + accountID),
	}
	jobController, err := controller.NewJobControllerWithDependencies(runConfig, deps)
	if err != nil {
		_ = s.markRunFailed(ctx, accountID, runID, fmt.Sprintf("failed creating job controller: %v", err))
		return fmt.Errorf("failed creating account-scoped job controller: %w", err)
	}
	if err := jobController.SearchAndFilterJobsWithRunID(runID); err != nil {
		return fmt.Errorf("pipeline failed: %w", err)
	}
	return nil
}

func (s *Server) buildRunConfig(settings *AccountSettings, cvPath string, forceReeval bool) *config.Config {
	cloned := *s.baseConfig
	cloned.Policy = settings.Policy
	cloned.Policy.CV.Path = cvPath
	cloned.App.Locations = append([]string{}, settings.Policy.App.JobLocations...)
	cloned.App.CronSchedule = settings.ScheduleCron
	cloned.App.CVPath = cvPath
	cloned.App.MaxJobs = settings.MaxJobs
	cloned.App.EnableFinalValidation = settings.Policy.Pipeline.EnableFinalValidation
	cloned.App.RunOnStartup = false
	cloned.App.ForceReeval = forceReeval
	cloned.SMTP.ToRecipients = append([]string{}, settings.NotificationEmails...)
	return &cloned
}

func (s *Server) markRunFailed(ctx context.Context, accountID, runID, errMsg string) error {
	now := time.Now().UTC()
	failedSummary := &models.RunSummary{
		RunID:        runID,
		Status:       models.RunStatusFailed,
		StartedAt:    now,
		CompletedAt:  &now,
		DurationMs:   0,
		StageCounts:  models.RunStageCounts{},
		Config:       models.RunConfigSnapshot{},
		LLMUsage:     models.LLMUsageSnapshot{},
		ErrorMessage: errMsg,
	}
	return s.repo.SaveRunSummary(ctx, accountID, failedSummary)
}

func (s *Server) isAuthorizedSchedulerRequest(r *http.Request) bool {
	if s.runtime.SchedulerToken == "" {
		return true
	}
	if token := strings.TrimSpace(r.Header.Get("X-Scheduler-Token")); token == s.runtime.SchedulerToken {
		return true
	}
	if bearer := bearerToken(r.Header.Get("Authorization")); bearer == s.runtime.SchedulerToken {
		return true
	}
	return false
}

func (s *Server) isAuthorizedWorkerRequest(r *http.Request) bool {
	if s.runtime.WorkerToken == "" {
		return true
	}
	if token := strings.TrimSpace(r.Header.Get("X-Worker-Token")); token == s.runtime.WorkerToken {
		return true
	}
	if bearer := bearerToken(r.Header.Get("Authorization")); bearer == s.runtime.WorkerToken {
		return true
	}
	// Cloud Tasks delivers OIDC tokens as "Authorization: Bearer <header>.<payload>.<sig>".
	// The worker is deployed --no-allow-unauthenticated, so Cloud Run IAM has already
	// verified the caller identity before the request arrives here.
	// Static WORKER_TOKEN values (openssl rand -hex 32) never contain dots;
	// a real JWT always has exactly two dots separating its three segments.
	if auth := strings.TrimSpace(r.Header.Get("Authorization")); strings.HasPrefix(auth, "Bearer ") {
		if strings.Count(strings.TrimPrefix(auth, "Bearer "), ".") == 2 {
			return true
		}
	}
	return false
}

func withRuntimeCORS(origins []string, next http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(origins))
	for _, origin := range origins {
		trimmed := strings.TrimSpace(origin)
		if trimmed != "" {
			allowed[trimmed] = struct{}{}
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		if origin != "" {
			if _, ok := allowed[origin]; ok {
				w.Header().Set("Access-Control-Allow-Origin", origin)
			}
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type,Authorization,X-Worker-Token,X-Scheduler-Token,X-Debug-User,X-Debug-UID")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func validatePolicy(policy config.Policy) error {
	if policy.Pipeline.PromisingScoreThreshold < 0 || policy.Pipeline.PromisingScoreThreshold > 10 {
		return fmt.Errorf("pipeline.promisingScoreThreshold must be between 0 and 10")
	}
	if policy.Notification.MinFinalScore < 0 || policy.Notification.MinFinalScore > 10 {
		return fmt.Errorf("notification.minFinalScore must be between 0 and 10")
	}
	return nil
}

func strconvAtoi(raw string) (int, error) {
	var value int
	for _, ch := range raw {
		if ch < '0' || ch > '9' {
			return 0, fmt.Errorf("invalid integer")
		}
		value = value*10 + int(ch-'0')
	}
	return value, nil
}
