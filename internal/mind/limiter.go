package mind

import (
	"sync"
	"time"
)

// LLMRateLimiter enforces global and per-guild limits on LLM calls.
type LLMRateLimiter struct {
	mu              sync.Mutex
	perMinute       []time.Time
	perHour         []time.Time
	maxPerMinute    int
	maxPerHour      int
	minGuildCooldown time.Duration
	lastByGuild     map[string]time.Time
}

// DefaultLLMLimiter returns a limiter: 6/min, 30/hour, 20s per-guild cooldown.
func DefaultLLMLimiter() *LLMRateLimiter {
	return &LLMRateLimiter{
		perMinute:        make([]time.Time, 0, 32),
		perHour:          make([]time.Time, 0, 64),
		maxPerMinute:     6,
		maxPerHour:       30,
		minGuildCooldown: 20 * time.Second,
		lastByGuild:      make(map[string]time.Time),
	}
}

// Allow returns true if an LLM call is allowed for this guild at now.
func (l *LLMRateLimiter) Allow(guildID string, lastGuildLLMCall time.Time, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Per-guild cooldown
	if !lastGuildLLMCall.IsZero() && now.Sub(lastGuildLLMCall) < l.minGuildCooldown {
		return false
	}

	// Trim old entries
	cutMin := now.Add(-1 * time.Minute)
	cutHour := now.Add(-1 * time.Hour)
	var nm []time.Time
	for _, t := range l.perMinute {
		if t.After(cutMin) {
			nm = append(nm, t)
		}
	}
	l.perMinute = nm
	var nh []time.Time
	for _, t := range l.perHour {
		if t.After(cutHour) {
			nh = append(nh, t)
		}
	}
	l.perHour = nh

	if len(l.perMinute) >= l.maxPerMinute || len(l.perHour) >= l.maxPerHour {
		return false
	}
	return true
}

// Record records that an LLM call was made for guildID at now. Call after successful Generate.
func (l *LLMRateLimiter) Record(guildID string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.perMinute = append(l.perMinute, now)
	l.perHour = append(l.perHour, now)
	l.lastByGuild[guildID] = now
}
