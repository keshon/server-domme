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

// Pollinations chat API — тот же бэк, что и у https://chat.pollinations.ai
// Запрос в формате их фронта: gen.pollinations.ai/v1/chat/completions

const (
	pollinationsChatURL = "https://gen.pollinations.ai/v1/chat/completions"
	pollinationsOrigin   = "https://chat.pollinations.ai"
)

type PollinationsProvider struct {
	client *http.Client
	apiKey string
}

// NewPollinationsProvider создаёт провайдер. apiKey опционален (без ключа запрос как с их веб-страницы).
func NewPollinationsProvider(apiKey string) *PollinationsProvider {
	return &PollinationsProvider{
		client: &http.Client{
			Timeout: 25 * time.Second,
		},
		apiKey: strings.TrimSpace(apiKey),
	}
}

func (p *PollinationsProvider) Generate(messages []Message) (string, error) {
	// Формат как у фронта chat.pollinations.ai; stream: false — получаем один JSON-ответ
	payload := map[string]interface{}{
		"model":       "openai",
		"messages":   messages,
		"max_tokens": 2000,
		"temperature": 0.7,
		"stream":     false,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest(http.MethodPost, pollinationsChatURL, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Origin", pollinationsOrigin)
	req.Header.Set("Referer", pollinationsOrigin+"/")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

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
