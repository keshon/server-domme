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

	req, _ := http.NewRequest(http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	req = req.WithContext(ctx)

	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	text := strings.TrimSpace(string(respBody))
	if len(text) == 0 {
		return "", fmt.Errorf("empty response")
	}

	// Case 1: Try normal non-streaming JSON (standard OpenAI-like response)
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if json.Unmarshal(respBody, &parsed) == nil && len(parsed.Choices) > 0 {
		return cleanReply(parsed.Choices[0].Message.Content), nil
	}

	// Case 2: Fallback → streaming JSON (line-delimited)
	if strings.Contains(text, "\n{") || strings.Count(text, "}{") > 0 {
		var replyBuilder strings.Builder
		decoder := json.NewDecoder(strings.NewReader(text))

		for {
			var chunk struct {
				Done    bool `json:"done"`
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			}
			if err := decoder.Decode(&chunk); err != nil {
				if err == io.EOF {
					break
				}
				return "", fmt.Errorf("decode chunk: %w", err)
			}
			replyBuilder.WriteString(chunk.Message.Content)
			if chunk.Done {
				break
			}
		}

		reply := strings.TrimSpace(replyBuilder.String())
		if reply != "" {
			return cleanReply(reply), nil
		}
	}

	// Case 3: Not JSON at all (plain text fallback)
	if strings.HasPrefix(text, "{") {
		return "", fmt.Errorf("unrecognized JSON structure: %s", text[:min(200, len(text))])
	}

	return cleanReply(text), nil
}

func cleanReply(reply string) string {
	reply = strings.TrimSpace(reply)

	// Remove <think>...</think> blocks
	re := regexp.MustCompile(`(?s)<think>.*?</think>`)
	reply = re.ReplaceAllString(reply, "")
	reply = strings.TrimSpace(reply)

	// Strip surrounding quotes if both ends match
	if len(reply) >= 2 {
		quotes := []struct{ open, close string }{
			{`"`, `"`},
			{`'`, `'`},
			{"“", "”"},
			{"‘", "’"},
		}
		for _, q := range quotes {
			if strings.HasPrefix(reply, q.open) && strings.HasSuffix(reply, q.close) {
				reply = strings.TrimSuffix(strings.TrimPrefix(reply, q.open), q.close)
				reply = strings.TrimSpace(reply)
				break
			}
		}
	}

	// Truncate if absurdly long
	if len(reply) > 2800 {
		reply = reply[:2800] + "\n\n[truncated]"
	}

	return reply
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
