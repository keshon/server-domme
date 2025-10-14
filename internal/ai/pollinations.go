package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type PollinationsProvider struct {
	client *http.Client
}

func NewPollinationsProvider() *PollinationsProvider {
	return &PollinationsProvider{
		client: &http.Client{Timeout: 25 * time.Second},
	}
}

func (p *PollinationsProvider) Generate(messages []Message) (string, error) {
	payload := map[string]interface{}{
		"model":       "openai",
		"messages":    messages,
		"temperature": 1,
		"private":     true,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequest("POST", "https://text.pollinations.ai/openai", bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("pollinations status=%d body=%s", resp.StatusCode, string(body))
	}

	var parsed struct {
		Choices []struct {
			Message struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("unmarshal response: %w body=%s", err, string(body))
	}

	if len(parsed.Choices) > 0 {
		return strings.TrimSpace(parsed.Choices[0].Message.Content), nil
	}

	return "", fmt.Errorf("no choices returned: %s", string(body))
}
