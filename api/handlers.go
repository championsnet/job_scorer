package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"sync"
	"time"

	"job-scorer/controller"
	"job-scorer/models"
)

// Valid pipeline stages for LoadCheckpointByStage
var validStages = map[string]bool{
	"all_jobs":            true,
	"prefiltered":         true,
	"evaluated":           true,
	"promising":           true,
	"final_evaluated":     true,
	"notification":        true,
	"validated_notification": true,
	"email_sent":          true,
}

// Handlers holds dependencies for API handlers
type Handlers struct {
	Controller *controller.JobController

	mu          sync.RWMutex
	requests    map[string]*RunRequest
	activeRunID string
}

type RunRequest struct {
	RequestID   string `json:"request_id"`
	RunID       string `json:"run_id"`
	Status      string `json:"status"`
	StartedAt   string `json:"started_at"`
	CompletedAt string `json:"completed_at,omitempty"`
	Error       string `json:"error,omitempty"`
}

func NewHandlers(controller *controller.JobController) *Handlers {
	return &Handlers{
		Controller: controller,
		requests:   make(map[string]*RunRequest),
	}
}

// writeJSON writes a JSON response
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

// writeError writes a JSON error response
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// PostRuns triggers a new pipeline run asynchronously
func (h *Handlers) PostRuns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	if r.PathValue("runId") != "" {
		writeError(w, http.StatusBadRequest, "POST /api/runs does not take a path parameter")
		return
	}
	h.mu.Lock()
	if h.activeRunID != "" {
		active := h.requests[h.activeRunID]
		h.mu.Unlock()
		writeJSON(w, http.StatusConflict, map[string]interface{}{
			"status":  "busy",
			"message": "a run is already in progress",
			"active":  active,
		})
		return
	}
	requestID := strconv.FormatInt(time.Now().UnixNano(), 10)
	runID := time.Now().Format("2006-01-02_15-04-05")
	req := &RunRequest{
		RequestID: requestID,
		RunID:     runID,
		Status:    "running",
		StartedAt: time.Now().Format(time.RFC3339),
	}
	h.requests[requestID] = req
	h.activeRunID = requestID
	h.mu.Unlock()

	go func(reqID, run string) {
		err := h.Controller.SearchAndFilterJobsWithRunID(run)
		h.mu.Lock()
		defer h.mu.Unlock()
		stored := h.requests[reqID]
		if stored == nil {
			return
		}
		if err != nil {
			stored.Status = "failed"
			stored.Error = err.Error()
		} else {
			stored.Status = "success"
		}
		stored.CompletedAt = time.Now().Format(time.RFC3339)
		if h.activeRunID == reqID {
			h.activeRunID = ""
		}
	}(requestID, runID)

	writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"status":      "accepted",
		"request_id":  requestID,
		"run_id":      runID,
		"status_url":  "/api/runs/requests/" + requestID,
		"message":     "pipeline run started",
	})
}

// GetRunRequest returns background run request status
func (h *Handlers) GetRunRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	requestID := r.PathValue("requestId")
	if requestID == "" {
		writeError(w, http.StatusBadRequest, "request_id is required")
		return
	}
	h.mu.RLock()
	req := h.requests[requestID]
	h.mu.RUnlock()
	if req == nil {
		writeError(w, http.StatusNotFound, "request not found")
		return
	}
	writeJSON(w, http.StatusOK, req)
}

// GetRuns lists recent runs
func (h *Handlers) GetRuns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	limit := 50
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	runs, err := h.Controller.ListRuns(limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"runs": runs})
}

// GetRun returns a single run by ID
func (h *Handlers) GetRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	runID := r.PathValue("runId")
	if runID == "" {
		writeError(w, http.StatusBadRequest, "run_id is required")
		return
	}
	run, err := h.Controller.GetRun(runID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if run == nil {
		writeError(w, http.StatusNotFound, "run not found")
		return
	}
	writeJSON(w, http.StatusOK, run)
}

// GetRunStageJobs returns jobs for a stage
func (h *Handlers) GetRunStageJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	runID := r.PathValue("runId")
	stage := r.PathValue("stage")
	if runID == "" || stage == "" {
		writeError(w, http.StatusBadRequest, "run_id and stage are required")
		return
	}
	if !validStages[stage] {
		writeError(w, http.StatusBadRequest, "invalid stage: "+stage)
		return
	}
	includedJobs, excludedJobs, baseCount, err := h.Controller.GetRunStageView(runID, stage)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if includedJobs == nil {
		includedJobs = []*models.Job{}
	}
	if excludedJobs == nil {
		excludedJobs = []*models.Job{}
	}
	if baseCount == 0 {
		baseCount = len(includedJobs) + len(excludedJobs)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"jobs":           includedJobs,
		"count":          len(includedJobs),
		"included_jobs":  includedJobs,
		"included_count": len(includedJobs),
		"excluded_jobs":  excludedJobs,
		"excluded_count": len(excludedJobs),
		"base_count":     baseCount,
	})
}

// GetAnalyticsOverview returns dashboard analytics
func (h *Handlers) GetAnalyticsOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	data, err := h.Controller.GetAnalyticsOverview()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, data)
}

// GetJobsSearch allows filtering jobs by run and stage (optional)
func (h *Handlers) GetJobsSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	runID := r.URL.Query().Get("run_id")
	stage := r.URL.Query().Get("stage")
	if runID == "" || stage == "" {
		writeError(w, http.StatusBadRequest, "run_id and stage query params are required")
		return
	}
	if !validStages[stage] {
		writeError(w, http.StatusBadRequest, "invalid stage: "+stage)
		return
	}
	includedJobs, excludedJobs, baseCount, err := h.Controller.GetRunStageView(runID, stage)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if includedJobs == nil {
		includedJobs = []*models.Job{}
	}
	if excludedJobs == nil {
		excludedJobs = []*models.Job{}
	}
	if baseCount == 0 {
		baseCount = len(includedJobs) + len(excludedJobs)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"jobs":           includedJobs,
		"count":          len(includedJobs),
		"included_jobs":  includedJobs,
		"included_count": len(includedJobs),
		"excluded_jobs":  excludedJobs,
		"excluded_count": len(excludedJobs),
		"base_count":     baseCount,
	})
}
