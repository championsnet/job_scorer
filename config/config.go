package config

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	OpenAI     OpenAIConfig
	SMTP       SMTPConfig
	App        AppConfig
	RateLimit  RateLimitConfig
	GCS        GCSConfig
	PolicyPath string
	Policy     Policy
}

type OpenAIConfig struct {
	APIKey  string
	Model   string
	BaseURL string
}

type SMTPConfig struct {
	Host         string
	Port         int
	Secure       bool
	User         string
	Pass         string
	From         string
	ToRecipients []string
}

type AppConfig struct {
	Locations             []string
	CronSchedule          string
	RunOnStartup          bool
	CVPath                string
	DataDir               string
	OutputDir             string
	MaxJobs               int
	EnableFinalValidation bool
}

type RateLimitConfig struct {
	MaxRequests        int           `json:"maxRequests"`
	TimeWindow         time.Duration `json:"timeWindow"`
	MaxTokensPerMinute int           `json:"maxTokensPerMinute"`
}

type GCSConfig struct {
	BucketName  string `json:"bucketName"`
	ProjectID   string `json:"projectId"`
	Enabled     bool   `json:"enabled"`
	FallbackDir string `json:"fallbackDir"`
}

func Load() (*Config, error) {
	// Load .env file if it exists
	godotenv.Load()

	policyPath := getEnv("POLICY_CONFIG_PATH", "config/config.json")
	policy, err := loadPolicy(policyPath)
	if err != nil {
		return nil, err
	}
	cronSchedule := strings.TrimSpace(policy.App.CronSchedule)
	// Backward compatibility: allow .env override when explicitly set.
	if envCron := strings.TrimSpace(os.Getenv("CRON_SCHEDULE")); envCron != "" {
		cronSchedule = envCron
	}
	if cronSchedule == "" {
		cronSchedule = "0 */1 * * *"
	}
	cvPath := strings.TrimSpace(policy.CV.Path)
	// Backward compatibility: allow .env override when explicitly set.
	if envCVPath := strings.TrimSpace(os.Getenv("CV_PATH")); envCVPath != "" {
		cvPath = envCVPath
	}
	if cvPath == "" {
		cvPath = "your_cv.pdf"
	}
	locations := append([]string{}, policy.App.JobLocations...)
	// Backward compatibility: allow .env fallback when policy locations are not set.
	if len(locations) == 0 {
		locations = parseCSV(getEnv("JOB_LOCATIONS", ""))
	}

	config := &Config{
		OpenAI: OpenAIConfig{
			APIKey:  getEnv("OPENAI_API_KEY", getEnv("GROQ_API_KEY", "")),
			Model:   getEnv("OPENAI_MODEL", getEnv("GROQ_MODEL", "gpt-5.2")),
			BaseURL: getEnv("OPENAI_BASE_URL", "https://api.openai.com/v1/chat/completions"),
		},
		SMTP: SMTPConfig{
			Host:         getEnv("SMTP_HOST", ""),
			Port:         getEnvInt("SMTP_PORT", 587),
			Secure:       getEnvBool("SMTP_SECURE", false),
			User:         getEnv("SMTP_USER", ""),
			Pass:         getEnv("SMTP_PASS", ""),
			From:         getEnv("SMTP_FROM", ""),
			ToRecipients: parseCSV(getEnv("SMTP_TO", "")),
		},
		App: AppConfig{
			Locations:             locations,
			CronSchedule:          cronSchedule,
			RunOnStartup:          getEnvBool("RUN_ON_STARTUP", true),
			CVPath:                cvPath,
			DataDir:               getEnv("DATA_DIR", "data"),
			OutputDir:             getEnv("OUTPUT_DIR", "."),
			MaxJobs:               getEnvInt("MAX_JOBS_PER_LOCATION", 1000),
			EnableFinalValidation: policy.Pipeline.EnableFinalValidation,
		},
		RateLimit: RateLimitConfig{
			MaxRequests:        getEnvInt("MAX_REQUESTS_PER_MINUTE", 20),
			TimeWindow:         time.Minute,
			MaxTokensPerMinute: getEnvInt("MAX_TOKENS_PER_MINUTE", 5000),
		},
		GCS: GCSConfig{
			BucketName:  getEnv("GCS_BUCKET_NAME", ""),
			ProjectID:   getEnv("GCS_PROJECT_ID", ""),
			Enabled:     getEnvBool("GCS_ENABLED", false),
			FallbackDir: getEnv("GCS_FALLBACK_DIR", "gcs_fallback"),
		},
		PolicyPath: policyPath,
		Policy:     policy,
	}

	return config, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}

func parseCSV(value string) []string {
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
