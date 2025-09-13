package ai

import (
	"fmt"
	"server-domme/internal/config"
	"strings"
	"time"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Provider interface with reply + engine optional
type Provider interface {
	Generate(messages []Message) (string, error)
}

type MultiProvider struct {
	engines []string
}

func (m *MultiProvider) Generate(messages []Message) (string, error) {
	var lastErr error

	for _, engine := range m.engines {
		provider := newSingleProvider(engine)

		// retry same model 2 times
		for attempt := 1; attempt <= 2; attempt++ {
			reply, err := provider.Generate(messages)
			if err == nil {
				fmt.Printf("[AI] success with %s (attempt %d)\n", engine, attempt)
				return reply, nil
			}
			lastErr = err
			sleep := time.Duration(200*attempt) * time.Millisecond
			fmt.Printf("[AI] %s attempt %d failed: %v\n", engine, attempt, err)
			time.Sleep(sleep)
		}
	}
	return "", fmt.Errorf("all providers failed, last error: %w", lastErr)
}

// Wrap single provider engine
func newSingleProvider(engine string) Provider {
	switch {
	case engine == "pollinations":
		return NewPollinationsProvider()
	case strings.HasPrefix(engine, "g4f"):
		return NewG4FProvider(engine)
	default:
		panic(fmt.Sprintf("unsupported AI_PROVIDER: %s", engine))
	}
}

// DefaultProvider returns MultiProvider with optional manual preference first
func DefaultProvider() Provider {
	cfg := config.New()
	preferred := strings.TrimSpace(cfg.AIProvider)

	// Base failover list
	failovers := []string{
		"g4f:gpt-oss-120b",
		"g4f:groq/meta-llama/llama-prompt-guard-2-86m",
		"g4f:groq/allam-2-7b",
		"g4f:groq/qwen/qwen3-32b",
		"g4f:groq/gemma2-9b-it",
		"g4f:groq/meta-llama/llama-guard-4-12b",
		"g4f:groq/llama-3.1-8b-instant",
		"g4f:groq/openai/gpt-oss-120b",
		"g4f:groq/openai/gpt-oss-20b",
		"g4f:groq/groq/compound-mini",
		"g4f:groq/moonshotai/kimi-k2-instruct-0905",
		"g4f:groq/meta-llama/llama-4-maverick-17b-128e-instruct",
		"g4f:groq/meta-llama/llama-prompt-guard-2-22m",
		"g4f:groq/meta-llama/llama-4-scout-17b-16e-instruct",
		"g4f:groq/deepseek-r1-distill-llama-70b",
		"g4f:groq/llama-3.3-70b-versatile",
		"g4f:groq/playai-tts-arabic",
		"g4f:groq/whisper-large-v3-turbo",
		"g4f:groq/whisper-large-v3",
		"g4f:groq/moonshotai/kimi-k2-instruct",
		"g4f:groq/groq/compound",
		"g4f:groq/playai-tts",
		"g4f:ollama/deepseek-v3.1:671b",
		"g4f:ollama/gpt-oss:120b",
		"g4f:ollama/gpt-oss:20b",
		"pollinations",
	}

	// If user set preferred engine, put it first in the list
	var engines []string
	if preferred != "" {
		// Avoid duplicates
		engines = append(engines, preferred)
		for _, f := range failovers {
			if f != preferred {
				engines = append(engines, f)
			}
		}
	} else {
		engines = failovers
	}

	return &MultiProvider{engines: engines}
}
