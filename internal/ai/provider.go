package ai

import (
	"fmt"
	"server-domme/internal/config"
	"sort"
	"strings"
	"sync"
	"time"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Provider interface {
	Generate(messages []Message) (string, error)
}

type engineStats struct {
	Successes int
	Failures  int
	LastUsed  time.Time
	LastError string
	Cooldown  time.Time
}

type MultiProvider struct {
	engines []string // canonical order
	stats   map[string]*engineStats
	mu      sync.Mutex
}

func NewMultiProvider(engines []string) *MultiProvider {
	stats := make(map[string]*engineStats)
	for _, e := range engines {
		stats[e] = &engineStats{}
	}
	return &MultiProvider{
		engines: engines,
		stats:   stats,
	}
}

func (m *MultiProvider) Generate(messages []Message) (string, error) {
	var lastErr error

	for _, engine := range m.orderedEngines() {
		// cooldown check
		m.mu.Lock()
		es := m.stats[engine]
		if es.Cooldown.After(time.Now()) {
			m.mu.Unlock()
			continue
		}
		m.mu.Unlock()

		provider := newSingleProvider(engine)

		// retry same model 2 times
		for attempt := 1; attempt <= 2; attempt++ {
			reply, err := provider.Generate(messages)
			m.mu.Lock()
			es.LastUsed = time.Now()
			if err == nil {
				es.Successes++
				m.mu.Unlock()
				fmt.Printf("[AI] success with %s (attempt %d)\n", engine, attempt)
				return reply, nil
			}
			es.Failures++
			es.LastError = err.Error()
			// put into cooldown for a bit after repeated failures
			if attempt == 2 {
				es.Cooldown = time.Now().Add(30 * time.Second)
			}
			m.mu.Unlock()

			lastErr = err
			sleep := time.Duration(200*attempt) * time.Millisecond
			fmt.Printf("[AI] %s attempt %d failed: %v\n", engine, attempt, err)
			time.Sleep(sleep)
		}
	}
	return "", fmt.Errorf("all providers failed, last error: %w", lastErr)
}

func (m *MultiProvider) orderedEngines() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	engines := append([]string(nil), m.engines...) // copy canonical list

	sort.SliceStable(engines, func(i, j int) bool {
		si := m.stats[engines[i]]
		sj := m.stats[engines[j]]

		scoreI := m.score(si)
		scoreJ := m.score(sj)

		// higher score first
		return scoreI > scoreJ
	})

	return engines
}

func (m *MultiProvider) score(es *engineStats) float64 {
	total := es.Successes + es.Failures
	successRate := 0.0
	if total > 0 {
		successRate = float64(es.Successes) / float64(total)
	}
	// weight: recent success counts, failures penalized
	return successRate*10 + float64(es.Successes) - float64(es.Failures)*2
}

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

func DefaultProvider() Provider {
	cfg := config.New()
	preferred := strings.TrimSpace(cfg.AIProvider)

	failovers := []string{
		"g4f:gpt-oss-120b",
		"g4f:groq/qwen/qwen3-32b",
		"g4f:groq/gemma2-9b-it",
		"g4f:groq/llama-3.1-8b-instant",
		"g4f:groq/openai/gpt-oss-120b",
		"g4f:groq/openai/gpt-oss-20b",
		"g4f:groq/groq/compound-mini",
		"g4f:groq/moonshotai/kimi-k2-instruct-0905",
		"g4f:groq/meta-llama/llama-4-maverick-17b-128e-instruct",
		"g4f:groq/meta-llama/llama-4-scout-17b-16e-instruct",
		"g4f:groq/deepseek-r1-distill-llama-70b",
		"g4f:groq/llama-3.3-70b-versatile",
		"g4f:groq/moonshotai/kimi-k2-instruct",
		"g4f:groq/groq/compound",
		"g4f:ollama/deepseek-v3.1:671b",
		"g4f:ollama/gpt-oss:120b",
		"g4f:ollama/gpt-oss:20b",
		"pollinations",
	}

	var engines []string
	if preferred != "" {
		engines = append(engines, preferred)
		for _, f := range failovers {
			if f != preferred {
				engines = append(engines, f)
			}
		}
	} else {
		engines = failovers
	}

	return NewMultiProvider(engines)
}
