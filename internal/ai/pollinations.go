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
		client: &http.Client{
			Timeout: 25 * time.Second,
		},
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
		return "", err
	}

	req, err := http.NewRequest(
		http.MethodPost,
		"https://text.pollinations.ai/openai",
		bytes.NewReader(data),
	)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("pollinations http %d: %s", resp.StatusCode, truncate(body))
	}

	if strings.Contains(resp.Header.Get("Content-Type"), "text/html") {
		return "", fmt.Errorf("pollinations returned html")
	}

	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", err
	}

	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("pollinations empty choices")
	}

	reply := cleanReply(parsed.Choices[0].Message.Content)
	if isGarbageResponse(reply) {
		return "", fmt.Errorf("pollinations returned garbage")
	}

	return reply, nil
}
