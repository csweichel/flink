package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	defaultOpenAIBaseURL = "https://api.openai.com/v1"
	defaultOpenAIModel   = "gpt-4.1-mini"
)

type AIClient struct {
	APIKey     string
	BaseURL    string
	Model      string
	HTTPClient *http.Client
}

type AIRequest struct {
	Prompt          string `json:"prompt"`
	Instructions    string `json:"instructions,omitempty"`
	Model           string `json:"model,omitempty"`
	MaxOutputTokens int    `json:"maxOutputTokens,omitempty"`
}

type AIResponse struct {
	Text       string `json:"text"`
	Model      string `json:"model,omitempty"`
	Configured bool   `json:"configured"`
}

func NewAIClientFromEnv() *AIClient {
	baseURL := strings.TrimRight(env("OPENAI_BASE_URL", defaultOpenAIBaseURL), "/")
	return &AIClient{
		APIKey:  os.Getenv("OPENAI_API_KEY"),
		BaseURL: baseURL,
		Model:   env("OPENAI_MODEL", defaultOpenAIModel),
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *AIClient) Configured() bool {
	return c != nil && c.APIKey != ""
}

func (c *AIClient) Generate(ctx context.Context, in AIRequest) (AIResponse, error) {
	prompt := strings.TrimSpace(in.Prompt)
	if prompt == "" {
		return AIResponse{}, fmt.Errorf("missing prompt")
	}
	model := strings.TrimSpace(in.Model)
	if model == "" {
		model = c.Model
	}

	payload := map[string]any{
		"model": model,
		"input": prompt,
		"store": false,
	}
	if in.Instructions != "" {
		payload["instructions"] = in.Instructions
	}
	if in.MaxOutputTokens > 0 {
		payload["max_output_tokens"] = in.MaxOutputTokens
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return AIResponse{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/responses", bytes.NewReader(body))
	if err != nil {
		return AIResponse{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")

	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	res, err := httpClient.Do(req)
	if err != nil {
		return AIResponse{}, err
	}
	defer res.Body.Close()

	var out openAIResponse
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return AIResponse{}, err
	}
	if res.StatusCode < 200 || res.StatusCode > 299 {
		if out.Error != nil && out.Error.Message != "" {
			return AIResponse{}, fmt.Errorf("AI request failed: %s", out.Error.Message)
		}
		return AIResponse{}, fmt.Errorf("AI request failed with status %d", res.StatusCode)
	}
	if out.Error != nil && out.Error.Message != "" {
		return AIResponse{}, fmt.Errorf("AI request failed: %s", out.Error.Message)
	}

	text := out.OutputText
	if text == "" {
		text = out.collectText()
	}
	return AIResponse{Text: text, Model: out.Model, Configured: true}, nil
}

type openAIResponse struct {
	Model      string `json:"model"`
	OutputText string `json:"output_text"`
	Error      *struct {
		Message string `json:"message"`
	} `json:"error"`
	Output []struct {
		Content []struct {
			Type    string `json:"type"`
			Text    string `json:"text"`
			Refusal string `json:"refusal"`
		} `json:"content"`
	} `json:"output"`
}

func (r openAIResponse) collectText() string {
	var parts []string
	for _, item := range r.Output {
		for _, content := range item.Content {
			switch {
			case content.Text != "":
				parts = append(parts, content.Text)
			case content.Refusal != "":
				parts = append(parts, content.Refusal)
			}
		}
	}
	return strings.Join(parts, "\n")
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
