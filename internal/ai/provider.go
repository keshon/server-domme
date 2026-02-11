package ai

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"server-domme/internal/config"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Provider interface {
	Generate(messages []Message) (string, error)
}

type ProviderError struct {
	Engine  string
	Attempt int
	Error   string
}

type GenerationReport struct {
	Engine   string
	Attempts int
	Errors   []ProviderError
	Success  bool
	Duration time.Duration
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
	engines []string
	stats   map[string]*engineStats
	mu      sync.Mutex
	path    string

	lastReport *GenerationReport
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
	start := time.Now()

	report := &GenerationReport{}
	defer func() {
		report.Duration = time.Since(start)
		m.mu.Lock()
		m.lastReport = report
		m.mu.Unlock()
	}()

	var lastErr error

	for _, engine := range m.orderedEngines() {
		es := m.getStats(engine)
		if es.Cooldown.After(time.Now()) {
			continue
		}

		provider := newSingleProvider(engine)

		for attempt := 1; attempt <= 2; attempt++ {
			report.Attempts++

			reply, err := provider.Generate(messages)

			m.mu.Lock()
			es.LastUsed = time.Now()

			if err == nil {
				es.Successes++
				es.Score += 5
				es.Cooldown = time.Time{}
				report.Engine = engine
				report.Success = true
				m.mu.Unlock()

				m.saveStatsAsync()
				return reply, nil
			}

			es.Failures++
			es.LastError = err.Error()
			es.Score -= 2.5

			report.Errors = append(report.Errors, ProviderError{
				Engine:  engine,
				Attempt: attempt,
				Error:   err.Error(),
			})

			if attempt == 2 {
				es.Score -= 5
				es.Cooldown = time.Now().Add(45 * time.Second)
			}
			m.mu.Unlock()

			lastErr = err
			time.Sleep(time.Duration(200*attempt) * time.Millisecond)
		}
	}

	m.saveStatsAsync()
	return "", fmt.Errorf("all providers failed: %w", lastErr)
}

type GenerationTrace struct {
	Engine string
	Errors []string
}

func (m *MultiProvider) LastTrace() GenerationTrace {
	m.mu.Lock()
	defer m.mu.Unlock()

	var errors []string
	var used string

	for _, engine := range m.engines {
		es := m.stats[engine]
		if es == nil {
			continue
		}
		if es.LastUsed.After(time.Now().Add(-2 * time.Second)) {
			used = engine
			break
		}
		if es.LastError != "" {
			errors = append(errors, fmt.Sprintf("%s: %s", engine, es.LastError))
		}
	}

	return GenerationTrace{
		Engine: used,
		Errors: errors,
	}
}

func (m *MultiProvider) getStats(engine string) *engineStats {
	m.mu.Lock()
	defer m.mu.Unlock()

	es := m.stats[engine]
	if es == nil {
		es = &engineStats{}
		m.stats[engine] = es
	}
	return es
}

var paramRegexp = regexp.MustCompile(`(\d+)b`)

func (m *MultiProvider) orderedEngines() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for _, es := range m.stats {
		days := now.Sub(es.LastUsed).Hours() / 24
		es.Score -= days * 0.1
		if es.Score < -50 {
			es.Score = -50
		}
	}

	engines := append([]string(nil), m.engines...)
	sort.SliceStable(engines, func(i, j int) bool {
		sizeI := extractBillionParams(engines[i])
		sizeJ := extractBillionParams(engines[j])

		if sizeI != sizeJ {
			return sizeI > sizeJ
		}

		si := m.stats[engines[i]]
		sj := m.stats[engines[j]]

		scoreI := 0.0
		scoreJ := 0.0

		if si != nil {
			if si.Cooldown.After(now) {
				scoreI = -1000
			} else {
				scoreI = si.Score
			}
		}
		if sj != nil {
			if sj.Cooldown.After(now) {
				scoreJ = -1000
			} else {
				scoreJ = sj.Score
			}
		}

		return scoreI > scoreJ
	})

	return engines
}

func extractBillionParams(name string) int {
	m := paramRegexp.FindStringSubmatch(name)
	if len(m) < 2 {
		return 0
	}
	v, _ := strconv.Atoi(m[1])
	return v
}

func (m *MultiProvider) loadStats() {
	data, err := os.ReadFile(m.path)
	if err != nil {
		return
	}
	json.Unmarshal(data, &m.stats)
}

func (m *MultiProvider) saveStatsAsync() {
	stats := m.stats
	path := m.path

	go func() {
		os.MkdirAll(filepath.Dir(path), 0755)
		data, _ := json.MarshalIndent(stats, "", "  ")
		_ = os.WriteFile(path, data, 0644)
	}()
}

func newSingleProvider(engine string) Provider {
	switch {
	case engine == "pollinations":
		return NewPollinationsProvider()
	default:
		panic("unsupported provider: " + engine)
	}
}

func DefaultProvider(cfg *config.Config) Provider {
	preferred := ""
	if cfg != nil {
		preferred = strings.TrimSpace(cfg.AIProvider)
	}

	failovers := []string{
		"pollinations",
	}

	var engines []string
	if preferred != "" {
		engines = append(engines, preferred)
		for _, e := range failovers {
			if e != preferred {
				engines = append(engines, e)
			}
		}
	} else {
		engines = failovers
	}

	return NewMultiProvider(engines)
}
