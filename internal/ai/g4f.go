package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type G4FProvider struct {
	baseURL string
	client  *http.Client
}

func NewG4FProvider() *G4FProvider {
	base := "https://g4f.dev/api/gpt-oss-120b"
	return &G4FProvider{
		baseURL: base,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (p *G4FProvider) Generate(messages []Message) (string, error) {
	payload := map[string]interface{}{
		"model":    "gpt-oss-120b",
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
		return "", err
	}
	if len(parsed.Choices) == 0 {
		return "No answer 🤐", nil
	}
	reply := strings.TrimSpace(parsed.Choices[0].Message.Content)
	if len(reply) > 1800 {
		reply = reply[:1800] + "\n\n[truncated]"
	}
	return reply, nil
}
