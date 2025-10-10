package ai

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	Score     float64
}

type MultiProvider struct {
	engines []string // canonical order
	stats   map[string]*engineStats
	mu      sync.Mutex
	path    string // path to persisted file
}

func NewMultiProvider(engines []string) *MultiProvider {
	m := &MultiProvider{
		engines: engines,
		stats:   make(map[string]*engineStats),
		path:    filepath.Join("data", "ai_stats.json"),
	}
	m.loadStats()
	return m
}

func (m *MultiProvider) Generate(messages []Message) (string, error) {
	var lastErr error

	for _, engine := range m.orderedEngines() {
		m.mu.Lock()
		es := m.stats[engine]
		if es == nil {
			es = &engineStats{}
			m.stats[engine] = es
		}

		// skip if in cooldown
		if es.Cooldown.After(time.Now()) {
			m.mu.Unlock()
			continue
		}
		m.mu.Unlock()

		provider := newSingleProvider(engine)
		for attempt := 1; attempt <= 2; attempt++ {
			reply, err := provider.Generate(messages)

			m.mu.Lock()
			es.LastUsed = time.Now()
			if err == nil {
				es.Successes++
				es.Score += 5.0           // strong reward
				es.Cooldown = time.Time{} // clear cooldown
				m.mu.Unlock()
				fmt.Printf("[AI] success with %s (attempt %d)\n", engine, attempt)
				m.saveStatsAsync()
				return reply, nil
			}

			es.Failures++
			es.LastError = err.Error()
			es.Score -= 2.5 // moderate penalty

			if attempt == 2 {
				es.Cooldown = time.Now().Add(45 * time.Second)
				es.Score -= 5 // strong penalty after full failure
			}
			m.mu.Unlock()

			lastErr = err
			fmt.Printf("[AI] %s attempt %d failed: %v\n", engine, attempt, err)
			time.Sleep(time.Duration(200*attempt) * time.Millisecond)
		}
	}

	m.saveStatsAsync()
	return "", fmt.Errorf("all providers failed, last error: %w", lastErr)
}

// Sort engines by global score, decay old stats
func (m *MultiProvider) orderedEngines() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for _, es := range m.stats {
		// decay scores gradually: -0.1/day
		days := now.Sub(es.LastUsed).Hours() / 24
		es.Score -= days * 0.1
		if es.Score < -50 {
			es.Score = -50
		}
	}

	engines := append([]string(nil), m.engines...)
	sort.SliceStable(engines, func(i, j int) bool {
		si := m.stats[engines[i]]
		sj := m.stats[engines[j]]
		if si == nil {
			return false
		}
		if sj == nil {
			return true
		}
		return si.Score > sj.Score
	})

	return engines
}

// Persistence

func (m *MultiProvider) loadStats() {
	data, err := os.ReadFile(m.path)
	if err != nil {
		return
	}
	json.Unmarshal(data, &m.stats)
}

func (m *MultiProvider) saveStatsAsync() {
	go func(stats map[string]*engineStats, path string) {
		os.MkdirAll(filepath.Dir(path), 0755)
		data, _ := json.MarshalIndent(stats, "", "  ")
		_ = os.WriteFile(path, data, 0644)
	}(m.stats, m.path)
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
