package evaluator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"job-scorer/config"
	"job-scorer/utils"
)

type OpenAIClient struct {
	apiKey      string
	model       string
	baseURL     string
	client      *http.Client
	logger      *utils.Logger
	rateLimiter *utils.RateLimiter // Token-aware rate limiter
}

type OpenAIRequest struct {
	Model               string    `json:"model"`
	Messages            []Message `json:"messages"`
	MaxCompletionTokens int       `json:"max_completion_tokens,omitempty"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type OpenAIResponse struct {
	Choices []Choice  `json:"choices"`
	Error   *APIError `json:"error,omitempty"`
}

type Choice struct {
	Message Message `json:"message"`
}

type APIError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

func NewOpenAIClient(cfg *config.Config, rateLimiter *utils.RateLimiter) *OpenAIClient {
	return &OpenAIClient{
		apiKey:  cfg.OpenAI.APIKey,
		model:   cfg.OpenAI.Model,
		baseURL: cfg.OpenAI.BaseURL,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger:      utils.NewLogger("LLMClient"),
		rateLimiter: rateLimiter,
	}
}

func (g *OpenAIClient) ChatCompletion(prompt string, maxTokens int) (string, error) {
	if g.apiKey == "" {
		return "", fmt.Errorf("OpenAI API key is not configured")
	}

	// Estimate tokens with a conservative ratio (1 token ≈ 3 characters).
	// This provides ~30% safety margin compared to the previous 4-char estimate
	// and helps keep us below provider TPM limits.
	estimatedPromptTokens := utf8.RuneCountInString(prompt) / 3
	if estimatedPromptTokens < 1 {
		estimatedPromptTokens = 1
	}
	totalTokens := estimatedPromptTokens + maxTokens

	// Enforce token-based rate limiting if configured
	if g.rateLimiter != nil {
		if err := g.rateLimiter.AcquireTokens(totalTokens); err != nil {
			return "", fmt.Errorf("token rate limit error: %w", err)
		}
	}

	// Add a small delay between requests to be extra safe
	time.Sleep(100 * time.Millisecond)

	requestBody := OpenAIRequest{
		Model: g.model,
		Messages: []Message{
			{
				Role:    "user",
				Content: prompt,
			},
		},
		MaxCompletionTokens: maxTokens,
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("error marshaling request: %w", err)
	}

	req, err := http.NewRequest("POST", g.baseURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+g.apiKey)
	req.Header.Set("Content-Type", "application/json")

	// Making request to LLM API (debug output removed to reduce spam)

	resp, err := g.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error making request to LLM API: %w", err)
	}
	defer resp.Body.Close()

	// Parse rate limit headers and update rate limiter
	g.parseRateLimitHeaders(resp.Header)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		g.logger.Error("LLM API error: HTTP %d: %s", resp.StatusCode, string(body))
		return "", fmt.Errorf("API error: HTTP %d", resp.StatusCode)
	}

	var openAIResp OpenAIResponse
	if err := json.Unmarshal(body, &openAIResp); err != nil {
		return "", fmt.Errorf("error unmarshaling response: %w", err)
	}

	if openAIResp.Error != nil {
		return "", fmt.Errorf("API error: %s", openAIResp.Error.Message)
	}

	if len(openAIResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	content := openAIResp.Choices[0].Message.Content
	return content, nil
}

// parseRateLimitHeaders parses provider rate limit headers and updates our rate limiter
func (g *OpenAIClient) parseRateLimitHeaders(headers http.Header) {
	if g.rateLimiter == nil {
		return
	}

	// Parse token-related headers (OpenAI-compatible + legacy-compatible)
	remainingTokens := firstHeaderValue(headers,
		"x-ratelimit-remaining-tokens",
		"x-ratelimit-remaining-tokens-minute",
	)
	resetTokens := firstHeaderValue(headers,
		"x-ratelimit-reset-tokens",
		"x-ratelimit-reset-tokens-minute",
	)
	limitTokens := firstHeaderValue(headers,
		"x-ratelimit-limit-tokens",
		"x-ratelimit-limit-tokens-minute",
	)

	// Log the current rate limit status
	if remainingTokens != "" && limitTokens != "" {
		if remaining, err := strconv.Atoi(remainingTokens); err == nil {
			if limit, err := strconv.Atoi(limitTokens); err == nil {
				g.logger.Info("🔢 LLM tokens: %d/%d remaining", remaining, limit)

				// Update our rate limiter with actual server state
				g.rateLimiter.UpdateFromHeaders(remaining, limit)

				if remaining < 1000 { // If we're running low on tokens
					if resetDuration := g.parseResetDuration(resetTokens); resetDuration > 0 {
						g.logger.Info("🚨 Low tokens remaining (%d). Waiting %v for reset...", remaining, resetDuration.Round(time.Second))
						time.Sleep(resetDuration + 500*time.Millisecond) // Add 500ms buffer
					}
				}
			}
		}
	}

	// Parse request-related headers
	remainingRequests := firstHeaderValue(headers,
		"x-ratelimit-remaining-requests",
		"x-ratelimit-remaining-requests-minute",
	)
	if remainingRequests != "" {
		if remaining, err := strconv.Atoi(remainingRequests); err == nil {
			g.logger.Info("📝 LLM requests remaining: %d", remaining)
			if remaining < 100 {
				g.logger.Warning("⚠️ Very low requests remaining: %d", remaining)
			}
		}
	}

	// Check for retry-after header (rate limit hit)
	retryAfter := headers.Get("retry-after")
	if retryAfter != "" {
		if seconds, err := strconv.Atoi(retryAfter); err == nil {
			g.logger.Info("⏸️ Rate limit hit. Retry-after: %d seconds", seconds)
			g.rateLimiter.HandleRetryAfter(seconds)
		}
	}
}

// parseResetDuration parses reset duration format (e.g., "7.66s", "2m59.56s")
func (g *OpenAIClient) parseResetDuration(resetStr string) time.Duration {
	resetStr = strings.TrimSpace(resetStr)

	// Handle formats like "7.66s" or "2m59.56s"
	if duration, err := time.ParseDuration(resetStr); err == nil {
		return duration
	}

	// If parsing fails, try to extract seconds from the end
	if strings.HasSuffix(resetStr, "s") {
		if seconds, err := strconv.ParseFloat(resetStr[:len(resetStr)-1], 64); err == nil {
			return time.Duration(seconds * float64(time.Second))
		}
	}

	return 0
}

func (g *OpenAIClient) IsConfigured() bool {
	return g.apiKey != ""
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func firstHeaderValue(headers http.Header, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(headers.Get(key)); value != "" {
			return value
		}
	}
	return ""
}
