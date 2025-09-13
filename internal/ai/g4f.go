// g4f.go
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

type G4FProvider struct {
	baseURL string
	model   string
	client  *http.Client
}

func NewG4FProvider(engine string) *G4FProvider {
	// engine examples:
	//   g4f:gpt-oss-120b
	//   g4f:groq/qwen/qwen3-32b
	//   g4f:ollama/gpt-oss:20b
	parts := strings.SplitN(engine, ":", 2)
	if len(parts) != 2 {
		// fallback to legacy
		parts = []string{"g4f", "gpt-oss-120b"}
	}
	target := parts[1]

	var base, model string
	switch {
	case strings.HasPrefix(target, "groq/"):
		base = "https://g4f.dev/api/groq"
		model = strings.TrimPrefix(target, "groq/")
	case strings.HasPrefix(target, "ollama/"):
		base = "https://g4f.dev/api/ollama"
		model = strings.TrimPrefix(target, "ollama/")
	default:
		// default OSS
		base = "https://g4f.dev/api/gpt-oss-120b"
		model = target
	}

	return &G4FProvider{
		baseURL: base,
		model:   model,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (p *G4FProvider) Generate(messages []Message) (string, error) {
	payload := map[string]interface{}{
		"model":    p.model,
		"messages": messages,
	}
	bodyBytes, _ := json.Marshal(payload)

	req, err := http.NewRequest(http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req = req.WithContext(ctx)

	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("status=%d body=%s", resp.StatusCode, string(respBody))
	}

	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", fmt.Errorf("unmarshal: %w body=%s", err, string(respBody))
	}
	if len(parsed.Choices) == 0 {
		return "No answer 🤐", nil
	}
	reply := strings.TrimSpace(parsed.Choices[0].Message.Content)

	// strip any <think>...</think> blocks
	re := regexp.MustCompile(`(?s)<think>.*?</think>`)
	reply = re.ReplaceAllString(reply, "")
	reply = strings.TrimSpace(reply)

	if len(reply) > 1800 {
		reply = reply[:1800] + "\n\n[truncated]"
	}
	return reply, nil
}
