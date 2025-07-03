package config

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Groq      GroqConfig
	SMTP      SMTPConfig
	App       AppConfig
	RateLimit RateLimitConfig
}

type GroqConfig struct {
	APIKey string
	Model  string
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
	Locations      []string
	CronSchedule   string
	RunOnStartup   bool
	CVPath         string
	DataDir        string
	OutputDir      string
	MaxJobs        int
}

type RateLimitConfig struct {
	MaxRequests int
	TimeWindow  time.Duration
}

func Load() (*Config, error) {
	// Load .env file if it exists
	godotenv.Load()

	config := &Config{
		Groq: GroqConfig{
			APIKey: getEnv("GROQ_API_KEY", ""),
			Model:  getEnv("GROQ_MODEL", "gemma2-9b-it"),
		},
		SMTP: SMTPConfig{
			Host:         getEnv("SMTP_HOST", ""),
			Port:         getEnvInt("SMTP_PORT", 587),
			Secure:       getEnvBool("SMTP_SECURE", false),
			User:         getEnv("SMTP_USER", ""),
			Pass:         getEnv("SMTP_PASS", ""),
			From:         getEnv("SMTP_FROM", ""),
			ToRecipients: strings.Split(getEnv("SMTP_TO", ""), ","),
		},
		App: AppConfig{
			Locations:    strings.Split(getEnv("JOB_LOCATIONS", "90009885,90009888"), ","),
			CronSchedule: getEnv("CRON_SCHEDULE", "0 */1 * * *"),
			RunOnStartup: getEnvBool("RUN_ON_STARTUP", true),
			CVPath:       getEnv("CV_PATH", "CV_Vasiliki Ploumistou_22_05.pdf"),
			DataDir:      getEnv("DATA_DIR", "data"),
			OutputDir:    getEnv("OUTPUT_DIR", "."),
			MaxJobs:      getEnvInt("MAX_JOBS_PER_LOCATION", 1000),
		},
		RateLimit: RateLimitConfig{
			MaxRequests: getEnvInt("MAX_REQUESTS_PER_MINUTE", 20),
			TimeWindow:  time.Minute,
		},
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