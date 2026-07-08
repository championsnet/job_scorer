// Package setup provides a first-run, browser-based configuration wizard so a
// non-technical user can get Job Scorer running without editing any files or
// hunting for a LinkedIn geoId. It writes .env and config/config.json, then
// asks the host application to reload its configuration.
package setup

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"job-scorer/config"
	"job-scorer/services/cv"
	"job-scorer/utils"

	"gopkg.in/gomail.v2"
)

//go:embed static/*
var staticFS embed.FS

// Handler serves the wizard UI and its supporting JSON endpoints.
type Handler struct {
	logger  *utils.Logger
	baseDir string
	// OnSaved is invoked after a successful save so the host can reload its
	// configuration and start scoring without a restart.
	OnSaved func() error
	// IsConfigured reports whether the app is already fully configured.
	IsConfigured func() bool
	// Desktop is true when running as the native desktop app, where the webview
	// cannot open an HTML file dialog and we use a native picker instead.
	Desktop bool
	// OnRunNow, if set, triggers a single scan (used right after setup when the
	// user opted into an immediate first scan).
	OnRunNow func()
}

// New creates a wizard handler. baseDir is where .env, config/config.json and
// uploaded CVs are written (normally the working directory).
func New(logger *utils.Logger, baseDir string) *Handler {
	if logger == nil {
		logger = utils.NewLogger("Setup")
	}
	return &Handler{logger: logger, baseDir: baseDir}
}

// RegisterRoutes wires the wizard endpoints onto the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /setup", h.handleIndex)
	mux.HandleFunc("GET /api/setup/status", h.handleStatus)
	mux.HandleFunc("POST /api/setup/test-openai", h.handleTestOpenAI)
	mux.HandleFunc("POST /api/setup/test-email", h.handleTestEmail)
	mux.HandleFunc("GET /api/setup/resolve-location", h.handleResolveLocation)
	mux.HandleFunc("POST /api/setup/cv", h.handleUploadCV)
	mux.HandleFunc("POST /api/setup/pick-cv", h.handlePickCV)
	mux.HandleFunc("POST /api/setup/save", h.handleSave)
}

// ServeIndex writes the wizard HTML. Exposed so the host can show it at "/"
// when the app is not yet configured.
func (h *Handler) ServeIndex(w http.ResponseWriter, r *http.Request) {
	h.handleIndex(w, r)
}

func (h *Handler) handleIndex(w http.ResponseWriter, r *http.Request) {
	h.servePage(w, "static/index.html")
}

// ServeDashboard writes the jobs dashboard, shown at "/" once configured.
func (h *Handler) ServeDashboard(w http.ResponseWriter, r *http.Request) {
	h.servePage(w, "static/dashboard.html")
}

func (h *Handler) servePage(w http.ResponseWriter, name string) {
	page, err := staticFS.ReadFile(name)
	if err != nil {
		http.Error(w, "page unavailable", http.StatusInternalServerError)
		return
	}
	// Never cache the app pages — otherwise the embedded webview keeps showing
	// an old build's wizard/dashboard after an update.
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(page)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// -------- status --------

func (h *Handler) handleStatus(w http.ResponseWriter, r *http.Request) {
	configured := false
	if h.IsConfigured != nil {
		configured = h.IsConfigured()
	}
	values := h.readExistingValues()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"configured": configured,
		"desktop":    h.Desktop,
		"values":     values,
	})
}

// readExistingValues loads the current config.json (if any) so the wizard can
// prefill fields when someone reopens it to reconfigure. Secrets are not
// returned.
func (h *Handler) readExistingValues() map[string]interface{} {
	out := map[string]interface{}{}
	data, err := os.ReadFile(h.configPath())
	if err != nil {
		return out
	}
	var p config.Policy
	if json.Unmarshal(data, &p) != nil {
		return out
	}
	out["desiredFields"] = p.CandidateProfile.DesiredFields
	out["seniority"] = p.CandidateProfile.Seniority
	out["languages"] = p.CandidateProfile.Languages
	out["targetLocations"] = p.CandidateProfile.TargetLocations
	out["jobLocations"] = p.App.JobLocations
	out["schedule"] = p.App.CronSchedule
	out["languageFilterEnabled"] = p.Filters.LanguageFilterEnabled
	out["dateSincePosted"] = p.Scraper.DateSincePosted
	out["cvPath"] = p.CV.Path
	out["model"] = strings.TrimSpace(os.Getenv("OPENAI_MODEL"))
	out["minScore"] = p.Notification.MinFinalScore
	maxJobs := 50
	if v, err := strconv.Atoi(strings.TrimSpace(os.Getenv("MAX_JOBS_PER_LOCATION"))); err == nil && v > 0 {
		maxJobs = v
	}
	out["maxJobs"] = maxJobs
	return out
}

// -------- OpenAI key test --------

type testOpenAIReq struct {
	Key     string `json:"key"`
	Model   string `json:"model"`
	BaseURL string `json:"baseURL"`
}

func (h *Handler) handleTestOpenAI(w http.ResponseWriter, r *http.Request) {
	var req testOpenAIReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "message": "invalid request"})
		return
	}
	req.Key = strings.TrimSpace(req.Key)
	if req.Key == "" {
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": false, "message": "Enter your OpenAI API key first."})
		return
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = "gpt-5.4-nano"
	}
	baseURL := strings.TrimSpace(req.BaseURL)
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1/chat/completions"
	}

	payload := map[string]interface{}{
		"model":                 model,
		"messages":              []map[string]string{{"role": "user", "content": "ping"}},
		"max_completion_tokens": 16,
	}
	// Reasoning models (o-series / GPT-5) reject max_tokens and want reasoning
	// minimized; regular models reject reasoning_effort, so only add it when set.
	if eff := config.ReasoningEffortFor(model); eff != "" {
		payload["reasoning_effort"] = eff
	}
	body, _ := json.Marshal(payload)
	httpReq, err := http.NewRequest(http.MethodPost, baseURL, bytes.NewReader(body))
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": false, "message": "Could not build request: " + err.Error()})
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+req.Key)

	client := &http.Client{Timeout: 25 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": false, "message": "Could not reach OpenAI — check your internet connection. (" + err.Error() + ")"})
		return
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	switch {
	case resp.StatusCode == http.StatusOK:
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "message": "Key works — model '" + model + "' responded."})
	case resp.StatusCode == http.StatusUnauthorized:
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": false, "message": "Invalid API key (401). Double-check the key you pasted."})
	default:
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": false, "message": fmt.Sprintf("Key reached OpenAI but got HTTP %d: %s", resp.StatusCode, extractAPIError(respBody))})
	}
}

func extractAPIError(body []byte) string {
	var parsed struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &parsed) == nil && parsed.Error.Message != "" {
		return parsed.Error.Message
	}
	s := strings.TrimSpace(string(body))
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}

// -------- email test --------

type emailConfig struct {
	Host   string   `json:"host"`
	Port   int      `json:"port"`
	Secure bool     `json:"secure"`
	User   string   `json:"user"`
	Pass   string   `json:"pass"`
	From   string   `json:"from"`
	To     []string `json:"to"`
}

func (h *Handler) handleTestEmail(w http.ResponseWriter, r *http.Request) {
	var cfg emailConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "message": "invalid request"})
		return
	}
	if err := h.sendTestEmail(cfg); err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": false, "message": friendlyEmailError(err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "message": "Test email sent — check your inbox (and spam)."})
}

func (h *Handler) sendTestEmail(cfg emailConfig) error {
	cfg.Host = strings.TrimSpace(cfg.Host)
	cfg.From = strings.TrimSpace(cfg.From)
	recipients := cleanList(cfg.To)
	if cfg.Host == "" {
		return fmt.Errorf("no SMTP host configured")
	}
	if len(recipients) == 0 {
		return fmt.Errorf("add at least one recipient email")
	}
	if cfg.Port == 0 {
		cfg.Port = 587
	}
	from := cfg.From
	if from == "" {
		from = cfg.User
	}

	m := gomail.NewMessage()
	m.SetHeader("From", from)
	m.SetHeader("To", recipients...)
	m.SetHeader("Subject", "✅ Job Scorer test email")
	m.SetBody("text/plain", "This is a test email from your Job Scorer setup. If you can read this, email notifications are working.")

	d := gomail.NewDialer(cfg.Host, cfg.Port, cfg.User, cfg.Pass)
	if cfg.Secure || cfg.Port == 465 {
		d.SSL = true
	}
	return d.DialAndSend(m)
}

func friendlyEmailError(err error) string {
	msg := err.Error()
	low := strings.ToLower(msg)
	switch {
	case strings.Contains(low, "authentication") || strings.Contains(low, "username and password") || strings.Contains(low, "535"):
		return "Login failed. For Gmail/Outlook you must use an app password, not your normal password. (" + msg + ")"
	case strings.Contains(low, "no such host") || strings.Contains(low, "timeout") || strings.Contains(low, "connection refused"):
		return "Could not connect to the mail server — check the host and port. (" + msg + ")"
	default:
		return msg
	}
}

// -------- location resolver --------

func (h *Handler) handleResolveLocation(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		writeJSON(w, http.StatusOK, map[string]interface{}{"results": []GeoResult{}})
		return
	}
	results, err := ResolveLocation(query)
	if err != nil {
		h.logger.Warning("location lookup failed for %q: %v", query, err)
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"results": []GeoResult{},
			"error":   "Could not reach LinkedIn to look up the city. You can type a geoId directly if you have one.",
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"results": results})
}

// -------- CV upload --------

func (h *Handler) handleUploadCV(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(16 << 20); err != nil { // 16MB
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "message": "Upload too large or invalid."})
		return
	}
	file, header, err := r.FormFile("cv")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "message": "No file received."})
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	allowed := map[string]bool{".pdf": true, ".txt": true, ".md": true, ".markdown": true}
	if !allowed[ext] {
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": false, "message": "Please upload a PDF, .txt, or .md file."})
		return
	}

	cvDir := filepath.Join(h.baseDir, "data", "cv")
	if err := os.MkdirAll(cvDir, 0o755); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "message": "Could not create storage folder."})
		return
	}
	dest := filepath.Join(cvDir, "cv"+ext)
	out, err := os.Create(dest)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "message": "Could not save the file."})
		return
	}
	defer out.Close()
	if _, err := io.Copy(out, file); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "message": "Could not write the file."})
		return
	}

	chars, extracted := cvReadInfo(dest)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":            true,
		"path":          relativeTo(h.baseDir, dest),
		"name":          header.Filename,
		"chars":         chars,
		"textExtracted": extracted,
	})
}

// handlePickCV opens a native OS file dialog and copies the chosen CV into the
// data dir. Used by the desktop app, where the webview's HTML file input does
// not work.
func (h *Handler) handlePickCV(w http.ResponseWriter, r *http.Request) {
	src, err := pickFileNative()
	if err != nil || strings.TrimSpace(src) == "" {
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": false, "message": "No file selected."})
		return
	}
	ext := strings.ToLower(filepath.Ext(src))
	allowed := map[string]bool{".pdf": true, ".txt": true, ".md": true, ".markdown": true}
	if !allowed[ext] {
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": false, "message": "Please choose a PDF, .txt, or .md file."})
		return
	}
	cvDir := filepath.Join(h.baseDir, "data", "cv")
	if err := os.MkdirAll(cvDir, 0o755); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "message": "Could not create storage folder."})
		return
	}
	dest := filepath.Join(cvDir, "cv"+ext)
	if err := copyFile(src, dest); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "message": "Could not copy the file: " + err.Error()})
		return
	}
	chars, extracted := cvReadInfo(dest)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":            true,
		"path":          relativeTo(h.baseDir, dest),
		"name":          filepath.Base(src),
		"chars":         chars,
		"textExtracted": extracted,
	})
}

// pickFileNative shows the OS's native "choose file" dialog and returns the
// selected path, or an error if the user cancelled / no picker is available.
func pickFileNative() (string, error) {
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.Command("osascript", "-e",
			`POSIX path of (choose file with prompt "Choose your CV (PDF, TXT, or MD)")`).Output()
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(out)), nil
	case "windows":
		ps := `Add-Type -AssemblyName System.Windows.Forms; ` +
			`$f = New-Object System.Windows.Forms.OpenFileDialog; ` +
			`$f.Filter = 'CV files|*.pdf;*.txt;*.md;*.markdown|All files|*.*'; ` +
			`if ($f.ShowDialog() -eq 'OK') { Write-Output $f.FileName }`
		out, err := exec.Command("powershell", "-NoProfile", "-Command", ps).Output()
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(out)), nil
	default:
		out, err := exec.Command("zenity", "--file-selection", "--title=Choose your CV").Output()
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(out)), nil
	}
}

// cvReadInfo extracts text from the saved CV so the wizard can confirm it was
// read. Returns the character count and whether real text was extracted (as
// opposed to falling back because a PDF had no text layer).
func cvReadInfo(path string) (int, bool) {
	policy := config.DefaultPolicy().CV
	reader := cv.NewCVReader(path, policy)
	text, _ := reader.LoadCV()
	extracted := strings.TrimSpace(text)
	ok := extracted != "" &&
		extracted != strings.TrimSpace(policy.FallbackText) &&
		len([]rune(extracted)) >= policy.MinValidTextLength
	return len([]rune(extracted)), ok
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// -------- save --------

type savePayload struct {
	OpenAIKey   string      `json:"openaiKey"`
	OpenAIModel string      `json:"openaiModel"`
	BaseURL     string      `json:"baseURL"`
	Email       emailConfig `json:"email"`
	EmailOn     bool        `json:"emailEnabled"`

	DesiredFields []string    `json:"desiredFields"`
	Seniority     []string    `json:"seniority"`
	Languages     []string    `json:"languages"`
	Locations     []GeoResult `json:"locations"`
	OnlyMyLang    bool        `json:"onlyMyLanguage"`
	Schedule      string      `json:"schedule"`
	DatePosted    string      `json:"dateSincePosted"`
	CVPath        string      `json:"cvPath"`
	RunOnStartup  bool        `json:"runOnStartup"`
	MaxJobs       int         `json:"maxJobs"`
	MinScore      float64     `json:"minScore"`
}

func (h *Handler) handleSave(w http.ResponseWriter, r *http.Request) {
	var p savePayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "message": "invalid request"})
		return
	}

	// When reconfiguring, keep the existing key/CV if the user left them blank.
	if strings.TrimSpace(p.OpenAIKey) == "" {
		p.OpenAIKey = strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	}
	if strings.TrimSpace(p.CVPath) == "" {
		p.CVPath = currentCVPath(h.configPath())
	}

	if strings.TrimSpace(p.OpenAIKey) == "" {
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": false, "message": "An OpenAI API key is required."})
		return
	}
	if len(p.Locations) == 0 {
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": false, "message": "Add at least one location."})
		return
	}
	if strings.TrimSpace(p.CVPath) == "" {
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": false, "message": "Upload your CV first."})
		return
	}

	if err := h.writeConfigJSON(p); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "message": "Could not write config: " + err.Error()})
		return
	}
	if err := h.writeEnvFile(p); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "message": "Could not write .env: " + err.Error()})
		return
	}

	// Make the new secrets visible to config.Load immediately (godotenv does
	// not override already-set env vars, so set them explicitly too).
	h.exportEnv(p)

	if h.OnSaved != nil {
		if err := h.OnSaved(); err != nil {
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"ok":      false,
				"saved":   true,
				"message": "Settings were saved, but starting failed: " + err.Error(),
			})
			return
		}
	}

	// Kick off a single scan right after setup if the user asked for it.
	if p.RunOnStartup && h.OnRunNow != nil {
		h.OnRunNow()
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "message": "All set! Job Scorer is configured and ready."})
}

func (h *Handler) writeConfigJSON(p savePayload) error {
	policy := config.DefaultPolicy()

	policy.CandidateProfile.DesiredFields = cleanList(p.DesiredFields)
	policy.CandidateProfile.Seniority = cleanList(p.Seniority)
	langs := cleanList(p.Languages)
	if len(langs) == 0 {
		langs = []string{"English"}
	}
	policy.CandidateProfile.Languages = langs

	geoIDs := make([]string, 0, len(p.Locations))
	locationNames := make([]string, 0, len(p.Locations))
	for _, loc := range p.Locations {
		id := strings.TrimSpace(loc.ID)
		if id == "" {
			continue
		}
		geoIDs = append(geoIDs, id)
		if n := strings.TrimSpace(loc.DisplayName); n != "" {
			locationNames = append(locationNames, n)
		}
	}
	policy.App.JobLocations = geoIDs
	policy.CandidateProfile.TargetLocations = locationNames

	if s := strings.TrimSpace(p.Schedule); s != "" {
		policy.App.CronSchedule = s
	}
	if d := strings.TrimSpace(p.DatePosted); d != "" {
		policy.Scraper.DateSincePosted = d
	}
	policy.CV.Path = strings.TrimSpace(p.CVPath)

	// Only recommend / email jobs scoring at least this (out of 10).
	if p.MinScore >= 0 && p.MinScore <= 10 {
		policy.Notification.MinFinalScore = p.MinScore
	}

	// Language filtering: opt-in. When on, give the detector a broad set of
	// common languages to compare against so it can actually tell them apart.
	policy.Filters.LanguageFilterEnabled = p.OnlyMyLang
	if p.OnlyMyLang {
		primary := "english"
		if len(langs) > 0 {
			primary = strings.ToLower(langs[0])
		}
		policy.Filters.PrimaryLanguage = primary
		policy.Filters.DetectionLanguages = detectionSet(langs)
		// Reject jobs whose description requires a language the candidate does
		// not speak (e.g. an English-only candidate seeing "fluent German
		// required"). This catches requirements the language detector misses
		// because the description is otherwise in English.
		policy.Filters.RedFlagLanguageKeywords = redFlagsForUnspoken(langs)
	}

	data, err := json.MarshalIndent(policy, "", "  ")
	if err != nil {
		return err
	}
	path := h.configPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func (h *Handler) writeEnvFile(p savePayload) error {
	model := strings.TrimSpace(p.OpenAIModel)
	if model == "" {
		model = "gpt-5.4-nano"
	}
	var b strings.Builder
	b.WriteString("# Generated by the Job Scorer setup wizard.\n")
	b.WriteString("OPENAI_API_KEY=" + envValue(p.OpenAIKey) + "\n")
	b.WriteString("OPENAI_MODEL=" + envValue(model) + "\n")
	if bu := strings.TrimSpace(p.BaseURL); bu != "" {
		b.WriteString("OPENAI_BASE_URL=" + envValue(bu) + "\n")
	}
	b.WriteString("POLICY_CONFIG_PATH=config/config.json\n")
	// The app never auto-scrapes on launch; recurring runs come from the
	// scheduler and one-offs from the dashboard / right after setup.
	b.WriteString("RUN_ON_STARTUP=false\n")
	maxJobs := p.MaxJobs
	if maxJobs <= 0 {
		maxJobs = 50
	}
	b.WriteString(fmt.Sprintf("MAX_JOBS_PER_LOCATION=%d\n", maxJobs))
	b.WriteString("\n# Email notifications (optional)\n")
	if p.EmailOn && strings.TrimSpace(p.Email.Host) != "" {
		port := p.Email.Port
		if port == 0 {
			port = 587
		}
		from := strings.TrimSpace(p.Email.From)
		if from == "" {
			from = strings.TrimSpace(p.Email.User)
		}
		b.WriteString("SMTP_HOST=" + envValue(p.Email.Host) + "\n")
		b.WriteString(fmt.Sprintf("SMTP_PORT=%d\n", port))
		b.WriteString(fmt.Sprintf("SMTP_SECURE=%t\n", p.Email.Secure || port == 465))
		b.WriteString("SMTP_USER=" + envValue(p.Email.User) + "\n")
		b.WriteString("SMTP_PASS=" + envValue(p.Email.Pass) + "\n")
		b.WriteString("SMTP_FROM=" + envValue(from) + "\n")
		b.WriteString("SMTP_TO=" + envValue(strings.Join(cleanList(p.Email.To), ",")) + "\n")
	} else {
		b.WriteString("# (email disabled — leave SMTP_* empty)\n")
	}

	return os.WriteFile(h.envPath(), []byte(b.String()), 0o600)
}

func (h *Handler) exportEnv(p savePayload) {
	model := strings.TrimSpace(p.OpenAIModel)
	if model == "" {
		model = "gpt-5.4-nano"
	}
	os.Setenv("OPENAI_API_KEY", strings.TrimSpace(p.OpenAIKey))
	os.Setenv("OPENAI_MODEL", model)
	if bu := strings.TrimSpace(p.BaseURL); bu != "" {
		os.Setenv("OPENAI_BASE_URL", bu)
	}
	os.Setenv("POLICY_CONFIG_PATH", "config/config.json")
	os.Setenv("RUN_ON_STARTUP", "false")
	maxJobs := p.MaxJobs
	if maxJobs <= 0 {
		maxJobs = 50
	}
	os.Setenv("MAX_JOBS_PER_LOCATION", fmt.Sprintf("%d", maxJobs))
	if p.EmailOn && strings.TrimSpace(p.Email.Host) != "" {
		port := p.Email.Port
		if port == 0 {
			port = 587
		}
		from := strings.TrimSpace(p.Email.From)
		if from == "" {
			from = strings.TrimSpace(p.Email.User)
		}
		os.Setenv("SMTP_HOST", strings.TrimSpace(p.Email.Host))
		os.Setenv("SMTP_PORT", fmt.Sprintf("%d", port))
		os.Setenv("SMTP_SECURE", fmt.Sprintf("%t", p.Email.Secure || port == 465))
		os.Setenv("SMTP_USER", strings.TrimSpace(p.Email.User))
		os.Setenv("SMTP_PASS", p.Email.Pass)
		os.Setenv("SMTP_FROM", from)
		os.Setenv("SMTP_TO", strings.Join(cleanList(p.Email.To), ","))
	}
}

func (h *Handler) configPath() string { return filepath.Join(h.baseDir, "config", "config.json") }
func (h *Handler) envPath() string    { return filepath.Join(h.baseDir, ".env") }

// currentCVPath reads the CV path from the existing config so reconfiguring
// without re-uploading keeps the saved CV.
func currentCVPath(configPath string) string {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return ""
	}
	var p config.Policy
	if json.Unmarshal(data, &p) != nil {
		return ""
	}
	return strings.TrimSpace(p.CV.Path)
}

// -------- helpers --------

func cleanList(items []string) []string {
	out := make([]string, 0, len(items))
	seen := map[string]bool{}
	for _, it := range items {
		s := strings.TrimSpace(it)
		if s == "" || seen[strings.ToLower(s)] {
			continue
		}
		seen[strings.ToLower(s)] = true
		out = append(out, s)
	}
	return out
}

// redFlagsForUnspoken returns "requires <language>" phrases for common
// languages the candidate does NOT speak, so jobs whose description demands
// them are rejected outright.
func redFlagsForUnspoken(spoken []string) []string {
	spokenSet := map[string]bool{}
	for _, s := range spoken {
		spokenSet[strings.ToLower(strings.TrimSpace(s))] = true
	}
	phrases := map[string][]string{
		"german":     {"german required", "fluent german", "fluent in german", "german fluency", "german language skills", "native german", "deutsch erforderlich", "deutschkenntnisse", "muttersprache deutsch", "verhandlungssicher deutsch", "fließend deutsch", "sehr gute deutschkenntnisse"},
		"french":     {"french required", "fluent french", "fluent in french", "french fluency", "native french", "français courant", "maîtrise du français", "langue française"},
		"italian":    {"italian required", "fluent italian", "fluent in italian", "native italian", "italiano richiesto", "lingua italiana"},
		"spanish":    {"spanish required", "fluent spanish", "fluent in spanish", "español requerido"},
		"dutch":      {"dutch required", "fluent dutch", "nederlands vereist"},
		"portuguese": {"portuguese required", "fluent portuguese"},
	}
	var out []string
	for lang, ps := range phrases {
		if !spokenSet[lang] {
			out = append(out, ps...)
		}
	}
	return out
}

// detectionSet returns the candidate's languages plus common ones, so the
// language detector has enough alternatives to compare against.
func detectionSet(langs []string) []string {
	common := []string{"english", "german", "french", "italian", "spanish", "dutch", "portuguese"}
	seen := map[string]bool{}
	out := []string{}
	add := func(l string) {
		l = strings.ToLower(strings.TrimSpace(l))
		if l == "" || seen[l] {
			return
		}
		seen[l] = true
		out = append(out, l)
	}
	for _, l := range langs {
		add(l)
	}
	for _, l := range common {
		add(l)
	}
	return out
}

func envValue(v string) string {
	// Keep .env values on a single line; strip newlines that could break parsing.
	return strings.NewReplacer("\n", "", "\r", "").Replace(strings.TrimSpace(v))
}

func relativeTo(base, path string) string {
	if rel, err := filepath.Rel(base, path); err == nil {
		return rel
	}
	return path
}
