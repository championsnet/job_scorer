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

type GroqClient struct {
	apiKey      string
	model       string
	client      *http.Client
	logger      *utils.Logger
	rateLimiter  *utils.RateLimiter // Token-aware rate limiter
}

type GroqRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	MaxTokens int      `json:"max_tokens,omitempty"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type GroqResponse struct {
	Choices []Choice `json:"choices"`
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

func NewGroqClient(cfg *config.Config, rateLimiter *utils.RateLimiter) *GroqClient {
	return &GroqClient{
		apiKey: cfg.Groq.APIKey,
		model:  cfg.Groq.Model,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger:      utils.NewLogger("GroqClient"),
		rateLimiter:  rateLimiter,
	}
}

func (g *GroqClient) ChatCompletion(prompt string, maxTokens int) (string, error) {
	if g.apiKey == "" {
		return "", fmt.Errorf("Groq API key is not configured")
	}

	// Estimate tokens with a conservative ratio (1 token ≈ 3 characters).
	// This provides ~30% safety margin compared to the previous 4-char estimate
	// and helps keep us below the real Groq TPM limit.
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

	requestBody := GroqRequest{
		Model: g.model,
		Messages: []Message{
			{
				Role:    "user",
				Content: prompt,
			},
		},
		MaxTokens: maxTokens,
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("error marshaling request: %w", err)
	}

	req, err := http.NewRequest("POST", "https://api.groq.com/openai/v1/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+g.apiKey)
	req.Header.Set("Content-Type", "application/json")

	// Making request to Groq API (debug output removed to reduce spam)

	resp, err := g.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error making request to Groq API: %w", err)
	}
	defer resp.Body.Close()

	// Parse rate limit headers and update rate limiter
	g.parseRateLimitHeaders(resp.Header)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		g.logger.Error("Groq API error: HTTP %d: %s", resp.StatusCode, string(body))
		return "", fmt.Errorf("API error: HTTP %d", resp.StatusCode)
	}

	var groqResp GroqResponse
	if err := json.Unmarshal(body, &groqResp); err != nil {
		return "", fmt.Errorf("error unmarshaling response: %w", err)
	}

	if groqResp.Error != nil {
		return "", fmt.Errorf("API error: %s", groqResp.Error.Message)
	}

	if len(groqResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	content := groqResp.Choices[0].Message.Content
	return content, nil
}

// parseRateLimitHeaders parses Groq's rate limit headers and updates our rate limiter
func (g *GroqClient) parseRateLimitHeaders(headers http.Header) {
	if g.rateLimiter == nil {
		return
	}

	// Parse token-related headers
	remainingTokens := headers.Get("x-ratelimit-remaining-tokens")
	resetTokens := headers.Get("x-ratelimit-reset-tokens")
	limitTokens := headers.Get("x-ratelimit-limit-tokens")
	
	// Log the current rate limit status
	if remainingTokens != "" && limitTokens != "" {
		if remaining, err := strconv.Atoi(remainingTokens); err == nil {
			if limit, err := strconv.Atoi(limitTokens); err == nil {
				g.logger.Info("🔢 Groq tokens: %d/%d remaining", remaining, limit)
				
				// Update our rate limiter with actual server state
				g.rateLimiter.UpdateFromHeaders(remaining, 0)
				
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
	remainingRequests := headers.Get("x-ratelimit-remaining-requests")
	if remainingRequests != "" {
		if remaining, err := strconv.Atoi(remainingRequests); err == nil {
			g.logger.Info("📝 Groq requests: %d remaining today", remaining)
			if remaining < 100 {
				g.logger.Warning("⚠️ Very low requests remaining for today: %d", remaining)
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

// parseResetDuration parses Groq's reset duration format (e.g., "7.66s", "2m59.56s")
func (g *GroqClient) parseResetDuration(resetStr string) time.Duration {
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

func (g *GroqClient) IsConfigured() bool {
	return g.apiKey != ""
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
} 