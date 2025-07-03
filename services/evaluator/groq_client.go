package evaluator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"job-scorer/config"
	"job-scorer/utils"
)

type GroqClient struct {
	apiKey string
	model  string
	client *http.Client
	logger *utils.Logger
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

func NewGroqClient(cfg *config.Config) *GroqClient {
	return &GroqClient{
		apiKey: cfg.Groq.APIKey,
		model:  cfg.Groq.Model,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: utils.NewLogger("GroqClient"),
	}
}

func (g *GroqClient) ChatCompletion(prompt string, maxTokens int) (string, error) {
	if g.apiKey == "" {
		return "", fmt.Errorf("Groq API key is not configured")
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

	g.logger.Debug("Making request to Groq API with model: %s", g.model)

	resp, err := g.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error making request to Groq API: %w", err)
	}
	defer resp.Body.Close()

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
	g.logger.Debug("Received response from Groq API: %s", content[:min(100, len(content))])

	return content, nil
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