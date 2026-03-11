package multitenant

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type CreditPackage struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Credits     int    `json:"credits"`
	PriceID     string `json:"price_id"`
}

type RuntimeConfig struct {
	Mode                    string
	Port                    string
	FrontendDistDir         string
	CORSOrigins             []string
	SchedulerToken          string
	WorkerToken             string
	RunCreditCost           int
	DefaultMaxJobs          int
	GCSBucketName           string
	GCSProjectID            string
	GCSEnabled              bool
	GCSFallbackDir          string
	GCSCVPrefix             string
	FirebaseProjectID       string
	FirebaseCredentialsFile string
	FirestoreProjectID      string
	AuthBypass              bool
	CloudTasksProjectID     string
	CloudTasksLocation      string
	CloudTasksQueue         string
	CloudTasksWorkerURL     string
	CloudTasksServiceAcct   string
	StripeSecretKey         string
	StripeWebhookSecret     string
	StripeSuccessURL        string
	StripeCancelURL         string
	BillingPackages         []CreditPackage
}

func LoadRuntimeConfig() (*RuntimeConfig, error) {
	mode := strings.ToLower(strings.TrimSpace(getEnv("APP_MODE", "legacy")))
	cfg := &RuntimeConfig{
		Mode:                    mode,
		Port:                    getEnv("PORT", "8008"),
		FrontendDistDir:         getEnv("FRONTEND_DIST_DIR", "frontend/dist"),
		CORSOrigins:             splitCSV(getEnv("CORS_ORIGINS", "http://localhost:5173,http://localhost:3000")),
		SchedulerToken:          strings.TrimSpace(getEnv("SCHEDULER_TOKEN", "")),
		WorkerToken:             strings.TrimSpace(getEnv("WORKER_TOKEN", "")),
		RunCreditCost:           getEnvInt("RUN_CREDIT_COST", 1),
		DefaultMaxJobs:          getEnvInt("DEFAULT_MAX_JOBS_PER_RUN", 1000),
		GCSBucketName:           strings.TrimSpace(getEnv("GCS_BUCKET_NAME", "")),
		GCSProjectID:            strings.TrimSpace(getEnv("GCS_PROJECT_ID", "")),
		GCSEnabled:              getEnvBool("GCS_ENABLED", false),
		GCSFallbackDir:          getEnv("GCS_FALLBACK_DIR", "gcs_fallback"),
		GCSCVPrefix:             strings.Trim(strings.TrimSpace(getEnv("GCS_CV_PREFIX", "accounts")), "/"),
		FirebaseProjectID:       strings.TrimSpace(getEnv("FIREBASE_PROJECT_ID", "")),
		FirebaseCredentialsFile: strings.TrimSpace(getEnv("FIREBASE_CREDENTIALS_FILE", "")),
		FirestoreProjectID:      strings.TrimSpace(getEnv("FIRESTORE_PROJECT_ID", "")),
		AuthBypass:              getEnvBool("AUTH_BYPASS", false),
		CloudTasksProjectID:     strings.TrimSpace(getEnv("CLOUD_TASKS_PROJECT_ID", "")),
		CloudTasksLocation:      strings.TrimSpace(getEnv("CLOUD_TASKS_LOCATION", "")),
		CloudTasksQueue:         strings.TrimSpace(getEnv("CLOUD_TASKS_QUEUE", "")),
		CloudTasksWorkerURL:     strings.TrimSpace(getEnv("CLOUD_TASKS_WORKER_URL", "")),
		CloudTasksServiceAcct:   strings.TrimSpace(getEnv("CLOUD_TASKS_SERVICE_ACCOUNT", "")),
		StripeSecretKey:         strings.TrimSpace(getEnv("STRIPE_SECRET_KEY", "")),
		StripeWebhookSecret:     strings.TrimSpace(getEnv("STRIPE_WEBHOOK_SECRET", "")),
		StripeSuccessURL:        strings.TrimSpace(getEnv("STRIPE_SUCCESS_URL", "")),
		StripeCancelURL:         strings.TrimSpace(getEnv("STRIPE_CANCEL_URL", "")),
	}

	billingPkgs, err := parseBillingPackages()
	if err != nil {
		return nil, err
	}
	cfg.BillingPackages = billingPkgs

	if (cfg.Mode == "web" || cfg.Mode == "worker" || cfg.Mode == "import") && cfg.FirebaseProjectID == "" {
		return nil, fmt.Errorf("FIREBASE_PROJECT_ID is required when APP_MODE=%s", cfg.Mode)
	}

	if cfg.Mode == "web" && cfg.CloudTasksWorkerURL == "" {
		return nil, fmt.Errorf("CLOUD_TASKS_WORKER_URL is required in APP_MODE=web")
	}

	if cfg.Mode == "web" && cfg.CloudTasksProjectID == "" {
		return nil, fmt.Errorf("CLOUD_TASKS_PROJECT_ID is required in APP_MODE=web")
	}

	if cfg.Mode == "web" && cfg.CloudTasksLocation == "" {
		return nil, fmt.Errorf("CLOUD_TASKS_LOCATION is required in APP_MODE=web")
	}

	if cfg.Mode == "web" && cfg.CloudTasksQueue == "" {
		return nil, fmt.Errorf("CLOUD_TASKS_QUEUE is required in APP_MODE=web")
	}

	return cfg, nil
}

func parseBillingPackages() ([]CreditPackage, error) {
	raw := strings.TrimSpace(os.Getenv("BILLING_PACKAGES_JSON"))
	if raw == "" {
		priceID := strings.TrimSpace(os.Getenv("STRIPE_PRICE_ID"))
		if priceID == "" {
			return []CreditPackage{}, nil
		}
		return []CreditPackage{
			{
				ID:          "starter",
				Name:        "Starter pack",
				Description: "Starter credits",
				Credits:     getEnvInt("STRIPE_PRICE_CREDITS", 25),
				PriceID:     priceID,
			},
		}, nil
	}

	var pkgs []CreditPackage
	if err := json.Unmarshal([]byte(raw), &pkgs); err != nil {
		return nil, fmt.Errorf("failed parsing BILLING_PACKAGES_JSON: %w", err)
	}
	for i := range pkgs {
		pkgs[i].ID = strings.TrimSpace(pkgs[i].ID)
		pkgs[i].PriceID = strings.TrimSpace(pkgs[i].PriceID)
		if pkgs[i].ID == "" || pkgs[i].PriceID == "" || pkgs[i].Credits <= 0 {
			return nil, fmt.Errorf("invalid billing package at index %d", i)
		}
	}
	return pkgs, nil
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func getEnv(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func getEnvInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnvBool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}
