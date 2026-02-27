package config

import (
	"os"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	// Save original environment
	originalEnv := map[string]string{
		"OPENAI_API_KEY":          os.Getenv("OPENAI_API_KEY"),
		"OPENAI_MODEL":            os.Getenv("OPENAI_MODEL"),
		"OPENAI_BASE_URL":         os.Getenv("OPENAI_BASE_URL"),
		"GROQ_API_KEY":            os.Getenv("GROQ_API_KEY"),
		"GROQ_MODEL":              os.Getenv("GROQ_MODEL"),
		"SMTP_HOST":               os.Getenv("SMTP_HOST"),
		"SMTP_PORT":               os.Getenv("SMTP_PORT"),
		"JOB_LOCATIONS":           os.Getenv("JOB_LOCATIONS"),
		"CRON_SCHEDULE":           os.Getenv("CRON_SCHEDULE"),
		"MAX_REQUESTS_PER_MINUTE": os.Getenv("MAX_REQUESTS_PER_MINUTE"),
		"POLICY_CONFIG_PATH":      os.Getenv("POLICY_CONFIG_PATH"),
		"CV_PATH":                 os.Getenv("CV_PATH"),
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
				"OPENAI_API_KEY": "",
				"OPENAI_MODEL":   "",
				"GROQ_API_KEY":   "",
				"GROQ_MODEL":     "",
				"POLICY_CONFIG_PATH": "",
				"CV_PATH":        "",
			},
			want: &Config{
				OpenAI: OpenAIConfig{
					APIKey:  "",
					Model:   "gpt-5.2",
					BaseURL: "https://api.openai.com/v1/chat/completions",
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
					Locations:    []string{},
					CronSchedule: "0 */1 * * *",
					RunOnStartup: true,
					CVPath:       "your_cv.pdf",
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
				"OPENAI_API_KEY":          "test-openai-key",
				"OPENAI_MODEL":            "gpt-5.2",
				"OPENAI_BASE_URL":         "https://api.openai.com/v1/chat/completions",
				"GROQ_API_KEY":            "legacy-groq-key",
				"GROQ_MODEL":              "legacy-model",
				"SMTP_HOST":               "smtp.test.com",
				"SMTP_PORT":               "465",
				"SMTP_SECURE":             "true",
				"JOB_LOCATIONS":           "123,456,789",
				"CRON_SCHEDULE":           "0 9 * * *",
				"RUN_ON_STARTUP":          "false",
				"MAX_REQUESTS_PER_MINUTE": "60",
				"POLICY_CONFIG_PATH":      "",
				"CV_PATH":                 "",
			},
			want: &Config{
				OpenAI: OpenAIConfig{
					APIKey:  "test-openai-key",
					Model:   "gpt-5.2",
					BaseURL: "https://api.openai.com/v1/chat/completions",
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
			if got.OpenAI.APIKey != tt.want.OpenAI.APIKey {
				t.Errorf("OpenAI.APIKey = %v, want %v", got.OpenAI.APIKey, tt.want.OpenAI.APIKey)
			}
			if got.OpenAI.Model != tt.want.OpenAI.Model {
				t.Errorf("OpenAI.Model = %v, want %v", got.OpenAI.Model, tt.want.OpenAI.Model)
			}
			if got.OpenAI.BaseURL != tt.want.OpenAI.BaseURL {
				t.Errorf("OpenAI.BaseURL = %v, want %v", got.OpenAI.BaseURL, tt.want.OpenAI.BaseURL)
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
