package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/keshon/server-domme/internal/config"
)

// Message is one chat turn.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Client is one OpenAI-compatible backend. No per-vendor drivers, no catalog,
// no pool: one primary plus one optional fallback, configured explicitly.
type Client struct {
	BaseURL       string
	Model         string
	FallbackModel string
	APIKey        string
	Timeout       time.Duration
	MaxTokens     int
	HTTP          *http.Client
}

// NewFromConfig builds a client. Timeout and tokens come from config with
// sane floors so a zero value cannot mean "wait forever".
func NewFromConfig(cfg *config.Config) *Client {
	timeout := 20 * time.Second
	if cfg != nil && cfg.LLMTimeoutSec > 0 {
		timeout = time.Duration(cfg.LLMTimeoutSec) * time.Second
	}
	maxTokens := 512
	if cfg != nil && cfg.LLMMaxTokens > 0 {
		maxTokens = cfg.LLMMaxTokens
	}
	var base, model, fallback, key string
	if cfg != nil {
		base, model, fallback, key = cfg.LLMBaseURL, cfg.LLMModel, cfg.LLMFallbackModel, cfg.LLMAPIKey
	}
	return &Client{
		BaseURL:       strings.TrimRight(base, "/"),
		Model:         model,
		FallbackModel: fallback,
		APIKey:        key,
		Timeout:       timeout,
		MaxTokens:     maxTokens,
		HTTP:          &http.Client{Timeout: timeout + 5*time.Second},
	}
}

type chatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Stream      bool      `json:"stream"`
	Temperature *float64  `json:"temperature,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// Complete sends messages and returns the trimmed text of the first choice.
// It tries Model, then FallbackModel once on failure. Any error is returned
// for the caller to fail closed on: no retries beyond the fallback, no
// silent empty string.
func (c *Client) Complete(ctx context.Context, messages []Message, temperature *float64) (string, error) {
	if c.BaseURL == "" || c.Model == "" {
		return "", fmt.Errorf("llm: not configured")
	}
	models := []string{c.Model}
	if c.FallbackModel != "" && c.FallbackModel != c.Model {
		models = append(models, c.FallbackModel)
	}
	var lastErr error
	for _, model := range models {
		text, err := c.call(ctx, model, messages, temperature)
		if err == nil {
			return text, nil
		}
		lastErr = err
	}
	return "", lastErr
}

func (c *Client) call(ctx context.Context, model string, messages []Message, temperature *float64) (string, error) {
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	body, err := json.Marshal(chatRequest{
		Model:       model,
		Messages:    messages,
		Stream:      false,
		Temperature: temperature,
		MaxTokens:   c.MaxTokens,
	})
	if err != nil {
		return "", fmt.Errorf("llm: encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("llm: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("llm: call %s: %w", model, err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("llm: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("llm: backend %d: %s", resp.StatusCode, truncate(string(raw), 300))
	}

	var parsed chatResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("llm: decode response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("llm: empty choices")
	}
	text := strings.TrimSpace(parsed.Choices[0].Message.Content)
	if text == "" {
		return "", fmt.Errorf("llm: empty reply")
	}
	return text, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
