package config

import (
	"os"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	// Save original environment
	originalEnv := map[string]string{
		"GROQ_API_KEY":             os.Getenv("GROQ_API_KEY"),
		"GROQ_MODEL":               os.Getenv("GROQ_MODEL"),
		"SMTP_HOST":                os.Getenv("SMTP_HOST"),
		"SMTP_PORT":                os.Getenv("SMTP_PORT"),
		"JOB_LOCATIONS":            os.Getenv("JOB_LOCATIONS"),
		"CRON_SCHEDULE":            os.Getenv("CRON_SCHEDULE"),
		"MAX_REQUESTS_PER_MINUTE":  os.Getenv("MAX_REQUESTS_PER_MINUTE"),
	}

	// Clean up after test
	defer func() {
		for key, value := range originalEnv {
			if value == "" {
				os.Unsetenv(key)
			} else {
				os.Setenv(key, value)
			}
		}
	}()

	tests := []struct {
		name    string
		envVars map[string]string
		want    *Config
	}{
		{
			name: "Default configuration",
			envVars: map[string]string{
				"GROQ_API_KEY": "",
				"GROQ_MODEL":   "",
			},
			want: &Config{
				Groq: GroqConfig{
					APIKey: "",
					Model:  "gemma2-9b-it",
				},
				SMTP: SMTPConfig{
					Host:         "",
					Port:         587,
					Secure:       false,
					User:         "",
					Pass:         "",
					From:         "",
					ToRecipients: []string{},
				},
				App: AppConfig{
					Locations:    []string{"90009885", "90009888"},
					CronSchedule: "0 */1 * * *",
					RunOnStartup: true,
					CVPath:       "CV_Vasiliki Ploumistou_22_05.pdf",
					DataDir:      "data",
					OutputDir:    ".",
				},
				RateLimit: RateLimitConfig{
					MaxRequests: 20,
					TimeWindow:  time.Minute,
				},
			},
		},
		{
			name: "Custom configuration",
			envVars: map[string]string{
				"GROQ_API_KEY":            "test-api-key",
				"GROQ_MODEL":              "custom-model",
				"SMTP_HOST":               "smtp.test.com",
				"SMTP_PORT":               "465",
				"SMTP_SECURE":             "true",
				"JOB_LOCATIONS":           "123,456,789",
				"CRON_SCHEDULE":           "0 9 * * *",
				"RUN_ON_STARTUP":          "false",
				"MAX_REQUESTS_PER_MINUTE": "60",
			},
			want: &Config{
				Groq: GroqConfig{
					APIKey: "test-api-key",
					Model:  "custom-model",
				},
				SMTP: SMTPConfig{
					Host:   "smtp.test.com",
					Port:   465,
					Secure: true,
				},
				App: AppConfig{
					Locations:    []string{"123", "456", "789"},
					CronSchedule: "0 9 * * *",
					RunOnStartup: false,
				},
				RateLimit: RateLimitConfig{
					MaxRequests: 60,
					TimeWindow:  time.Minute,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables for this test
			for key, value := range tt.envVars {
				if value == "" {
					os.Unsetenv(key)
				} else {
					os.Setenv(key, value)
				}
			}

			got, err := Load()
			if err != nil {
				t.Errorf("Load() error = %v", err)
				return
			}

			// Check individual fields
			if got.Groq.APIKey != tt.want.Groq.APIKey {
				t.Errorf("Groq.APIKey = %v, want %v", got.Groq.APIKey, tt.want.Groq.APIKey)
			}
			if got.Groq.Model != tt.want.Groq.Model {
				t.Errorf("Groq.Model = %v, want %v", got.Groq.Model, tt.want.Groq.Model)
			}
			if got.SMTP.Host != tt.want.SMTP.Host {
				t.Errorf("SMTP.Host = %v, want %v", got.SMTP.Host, tt.want.SMTP.Host)
			}
			if got.SMTP.Port != tt.want.SMTP.Port {
				t.Errorf("SMTP.Port = %v, want %v", got.SMTP.Port, tt.want.SMTP.Port)
			}
			if got.SMTP.Secure != tt.want.SMTP.Secure {
				t.Errorf("SMTP.Secure = %v, want %v", got.SMTP.Secure, tt.want.SMTP.Secure)
			}
			if got.App.CronSchedule != tt.want.App.CronSchedule {
				t.Errorf("App.CronSchedule = %v, want %v", got.App.CronSchedule, tt.want.App.CronSchedule)
			}
			if got.App.RunOnStartup != tt.want.App.RunOnStartup {
				t.Errorf("App.RunOnStartup = %v, want %v", got.App.RunOnStartup, tt.want.App.RunOnStartup)
			}
			if got.RateLimit.MaxRequests != tt.want.RateLimit.MaxRequests {
				t.Errorf("RateLimit.MaxRequests = %v, want %v", got.RateLimit.MaxRequests, tt.want.RateLimit.MaxRequests)
			}

			// Check slice fields
			if len(got.App.Locations) != len(tt.want.App.Locations) {
				t.Errorf("App.Locations length = %v, want %v", len(got.App.Locations), len(tt.want.App.Locations))
			} else {
				for i, loc := range got.App.Locations {
					if loc != tt.want.App.Locations[i] {
						t.Errorf("App.Locations[%d] = %v, want %v", i, loc, tt.want.App.Locations[i])
					}
				}
			}
		})
	}
}

func TestGetEnvInt(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		value        string
		defaultValue int
		want         int
	}{
		{
			name:         "Valid integer",
			key:          "TEST_INT",
			value:        "42",
			defaultValue: 0,
			want:         42,
		},
		{
			name:         "Invalid integer",
			key:          "TEST_INT_INVALID",
			value:        "not-a-number",
			defaultValue: 100,
			want:         100,
		},
		{
			name:         "Empty value",
			key:          "TEST_INT_EMPTY",
			value:        "",
			defaultValue: 200,
			want:         200,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clean up
			defer os.Unsetenv(tt.key)

			if tt.value != "" {
				os.Setenv(tt.key, tt.value)
			}

			got := getEnvInt(tt.key, tt.defaultValue)
			if got != tt.want {
				t.Errorf("getEnvInt() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetEnvBool(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		value        string
		defaultValue bool
		want         bool
	}{
		{
			name:         "True value",
			key:          "TEST_BOOL_TRUE",
			value:        "true",
			defaultValue: false,
			want:         true,
		},
		{
			name:         "False value",
			key:          "TEST_BOOL_FALSE",
			value:        "false",
			defaultValue: true,
			want:         false,
		},
		{
			name:         "Invalid boolean",
			key:          "TEST_BOOL_INVALID",
			value:        "not-a-bool",
			defaultValue: true,
			want:         true,
		},
		{
			name:         "Empty value",
			key:          "TEST_BOOL_EMPTY",
			value:        "",
			defaultValue: false,
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clean up
			defer os.Unsetenv(tt.key)

			if tt.value != "" {
				os.Setenv(tt.key, tt.value)
			}

			got := getEnvBool(tt.key, tt.defaultValue)
			if got != tt.want {
				t.Errorf("getEnvBool() = %v, want %v", got, tt.want)
			}
		})
	}
} 