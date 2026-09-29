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

// Client is one OpenAI-compatible backend: a base URL, a model, and an
// optional key. One type covers pollinations, g4f.space, a local Ollama and a
// paid endpoint, because all four speak the same REST shape — there is no
// per-vendor driver here.
type Client struct {
	// Name identifies the backend in logs and in Pool's scoring. It is not
	// sent anywhere.
	Name    string
	BaseURL string
	Model   string
	// FallbackModel is a second model on the same endpoint, tried once when
	// Model fails. It covers "same relay, other model", not "other relay":
	// cross-endpoint failover is the Pool's job.
	FallbackModel string
	APIKey        string
	Timeout       time.Duration
	MaxTokens     int
	HTTP          *http.Client
}

// NewClient returns a Client with a timeout-bounded HTTP client.
func NewClient(name, baseURL, model, apiKey string) *Client {
	return &Client{
		Name:    name,
		BaseURL: strings.TrimRight(baseURL, "/"),
		Model:   model,
		APIKey:  apiKey,
		HTTP:    &http.Client{Timeout: 25 * time.Second},
	}
}

// NewFromConfig builds a single-endpoint client from the explicit settings.
// Timeout and tokens come from config with sane floors so a zero value cannot
// mean "wait forever". Prefer ProviderForConfig in production: it returns the
// shared relay pool (custom + LLM_BACKENDS + free relays) instead of one
// endpoint.
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
		Name:          "custom",
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
		Message Message `json:"message"`
		// Delta carries the content when a backend streams despite
		// stream:false. Reading both is what lets one decoder handle a relay
		// that ignores the flag.
		Delta Message `json:"delta"`
	} `json:"choices"`
	// Message is the Ollama-native shape, which some relayed servers answer
	// with instead of the OpenAI one.
	Message Message `json:"message"`
	Done    bool    `json:"done"`
	Error   *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

// maxErrorBody caps how much of a failed response is quoted into an error.
const maxErrorBody = 300

// Complete sends messages and returns the trimmed text of the first choice.
// It tries Model, then FallbackModel once on failure. Any error is returned
// for the caller to fail closed on: no silent empty string.
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

	name := c.Name
	if name == "" {
		name = model
	}
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: timeout + 5*time.Second}
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("llm: call %s: %w", name, err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("llm: read %s response: %w", name, err)
	}
	if resp.StatusCode != http.StatusOK {
		if refusesUntilChanged(resp.StatusCode) {
			return "", fmt.Errorf("llm: %s returned %d (%w): %s",
				name, resp.StatusCode, ErrBackendRefused, snippet(raw))
		}
		return "", fmt.Errorf("llm: %s returned %d: %s", name, resp.StatusCode, snippet(raw))
	}

	reply, err := parseReply(name, raw)
	if err != nil {
		return "", err
	}
	if reply == "" {
		return "", fmt.Errorf("llm: %s: %w", name, ErrEmptyReply)
	}
	return reply, nil
}

// parseReply pulls the assistant text out of a response body.
//
// Three shapes reach this, all observed from the free relays rather than
// assumed: a single OpenAI chat completion; a stream of line-delimited JSON
// objects, sent even though the request set stream:false; and an
// application-level error carried inside a 200. Anything that does not parse
// as JSON at all is treated as plain text, because some relayed servers answer
// that way when they are proxying a non-OpenAI backend.
func parseReply(name string, raw []byte) (string, error) {
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return "", nil
	}

	if !strings.HasPrefix(text, "{") && !strings.HasPrefix(text, "[") {
		return text, nil
	}

	decoder := json.NewDecoder(strings.NewReader(text))
	var assembled strings.Builder
	var decoded bool

	for {
		var chunk chatResponse
		if err := decoder.Decode(&chunk); err != nil {
			if err == io.EOF {
				break
			}
			if decoded {
				break
			}
			return "", fmt.Errorf("llm: decode %s response: %w", name, err)
		}
		decoded = true

		if chunk.Error != nil && chunk.Error.Message != "" {
			return "", fmt.Errorf("llm: %s refused the request: %s", name, chunk.Error.Message)
		}

		if len(chunk.Choices) > 0 {
			assembled.WriteString(chunk.Choices[0].Message.Content)
			assembled.WriteString(chunk.Choices[0].Delta.Content)
		}
		assembled.WriteString(chunk.Message.Content)

		if chunk.Done {
			break
		}
	}

	return assembled.String(), nil
}

// refusesUntilChanged reports whether a status means the backend will answer
// the same way until credentials or credit change.
//
// 429 is deliberately absent: rate limiting is the transient case this exists
// to be distinguished from, and a backend that is merely busy should come back
// on the ordinary cooldown.
func refusesUntilChanged(status int) bool {
	switch status {
	case http.StatusUnauthorized, http.StatusPaymentRequired, http.StatusForbidden:
		return true
	default:
		return false
	}
}

// snippet trims a response body down to something a log line can carry.
func snippet(raw []byte) string {
	s := strings.TrimSpace(string(raw))
	if len(s) > maxErrorBody {
		return s[:maxErrorBody] + "…"
	}
	return s
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

var _ Provider = (*Client)(nil)
