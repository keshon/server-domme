package ai

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// Pool scoring and cooldown constants.
//
// The numbers matter less than their ratio: one failure has to cost more than
// one success earns, or a backend that answers one call in three keeps its
// place at the head of the queue forever. Cooldown is what stops a dead
// backend being retried on every single message.
const (
	scoreOnSuccess  = 2.0
	scoreOnFailure  = -3.0
	scoreFloor      = -20.0
	scoreCeiling    = 20.0
	backendCooldown = 90 * time.Second
	// refusedCooldown rests a backend that answered with no credit, no key or
	// not allowed. Long, because nothing this process does will change the
	// answer, and every attempt in the meantime costs a request. See
	// ErrBackendRefused.
	refusedCooldown = 30 * time.Minute
	attemptsPerTry  = 2
)

// backend pairs a Client with what the Pool has learned about it. All fields
// are guarded by Pool.mu.
type backend struct {
	client        *Client
	score         float64
	successes     int
	failures      int
	cooldownUntil time.Time
	lastErr       string
}

// Pool sends to the first backend that will answer, learning the order from
// what happened rather than from configuration.
//
// The order is deliberately not fixed: the free relays take turns being
// broken, and a static primary/fallback list spends a request discovering the
// primary is down every time. Scores are held in memory only — a restart
// re-learns the ranking within a handful of calls, which is cheaper than
// owning a state file and the stale-data questions that come with it.
type Pool struct {
	mu       sync.Mutex
	backends []*backend
	log      zerolog.Logger
}

// NewPool returns a Pool over clients, in the order given. The order is the
// starting ranking only; it stops mattering after the first few calls.
func NewPool(log zerolog.Logger, clients ...*Client) *Pool {
	p := &Pool{log: log}
	for _, c := range clients {
		if c == nil {
			continue
		}
		p.backends = append(p.backends, &backend{client: c})
	}
	return p
}

// Len reports how many backends the pool holds.
func (p *Pool) Len() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.backends)
}

// Generate implements Provider. It tries backends in score order, giving each
// attemptsPerTry attempts before moving on, and returns ErrNoBackend when none
// of them answered.
func (p *Pool) Generate(ctx context.Context, messages []Message) (string, error) {
	var lastErr error

	for _, b := range p.ready(time.Now()) {
		for attempt := 1; attempt <= attemptsPerTry; attempt++ {
			if ctx.Err() != nil {
				return "", ctx.Err()
			}

			reply, err := b.client.Generate(ctx, messages)
			if err == nil {
				p.recordSuccess(b)
				p.log.Debug().
					Str("backend", b.client.Name).
					Int("attempt", attempt).
					Msg("ai_generate_succeeded")
				return reply, nil
			}

			lastErr = err
			cooled := p.recordFailure(b, err, attempt, time.Now())
			refused := errors.Is(err, ErrBackendRefused)

			p.log.Debug().
				Str("backend", b.client.Name).
				Int("attempt", attempt).
				Bool("cooled_down", cooled).
				Bool("refused", refused).
				Err(err).
				Msg("ai_generate_failed")

			// A cancelled or timed-out context is the caller giving up, not
			// the backend failing. Trying the next one would run past the
			// deadline the caller set.
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return "", err
			}

			// Move to the next backend rather than spending the second attempt
			// on an answer that will not change.
			if refused {
				break
			}
		}
	}

	if lastErr == nil {
		return "", fmt.Errorf("ai: %w: all backends in cooldown", ErrNoBackend)
	}
	return "", fmt.Errorf("ai: %w: last error: %w", ErrNoBackend, lastErr)
}

// ready returns the backends worth trying now, best score first. A backend in
// cooldown is skipped entirely unless every backend is cooled down, in which
// case the cooldowns are ignored: refusing to speak because all three relays
// misbehaved in the last minute is worse than trying the least-bad one.
func (p *Pool) ready(now time.Time) []*backend {
	p.mu.Lock()
	defer p.mu.Unlock()

	ranked := make([]*backend, len(p.backends))
	copy(ranked, p.backends)
	sort.SliceStable(ranked, func(i, j int) bool {
		return ranked[i].score > ranked[j].score
	})

	available := make([]*backend, 0, len(ranked))
	for _, b := range ranked {
		if b.cooldownUntil.After(now) {
			continue
		}
		available = append(available, b)
	}
	if len(available) == 0 {
		return ranked
	}
	return available
}

func (p *Pool) recordSuccess(b *backend) {
	p.mu.Lock()
	defer p.mu.Unlock()

	b.successes++
	b.lastErr = ""
	b.cooldownUntil = time.Time{}
	b.score = clampScore(b.score + scoreOnSuccess)
}

// recordFailure scores a failure and reports whether it put the backend into
// cooldown. Only the last attempt does: a single transient failure should not
// sideline a backend that is otherwise healthy.
func (p *Pool) recordFailure(b *backend, err error, attempt int, now time.Time) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	b.failures++
	b.lastErr = err.Error()
	b.score = clampScore(b.score + scoreOnFailure)

	// A refusal does not get a second attempt: the answer is already known,
	// and asking again only spends another request to hear it.
	if errors.Is(err, ErrBackendRefused) {
		b.cooldownUntil = now.Add(refusedCooldown)
		return true
	}

	if attempt >= attemptsPerTry {
		b.cooldownUntil = now.Add(backendCooldown)
		return true
	}
	return false
}

// BackendStat is one backend's observed behaviour, for operator-facing output.
type BackendStat struct {
	Name      string
	Model     string
	Score     float64
	Successes int
	Failures  int
	CooledFor time.Duration
	LastError string
}

// Stats returns a snapshot of every backend, best score first.
func (p *Pool) Stats() []BackendStat {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	out := make([]BackendStat, 0, len(p.backends))
	for _, b := range p.backends {
		var cooled time.Duration
		if b.cooldownUntil.After(now) {
			cooled = b.cooldownUntil.Sub(now).Round(time.Second)
		}
		out = append(out, BackendStat{
			Name:      b.client.Name,
			Model:     b.client.Model,
			Score:     b.score,
			Successes: b.successes,
			Failures:  b.failures,
			CooledFor: cooled,
			LastError: b.lastErr,
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}

func clampScore(v float64) float64 {
	if v < scoreFloor {
		return scoreFloor
	}
	if v > scoreCeiling {
		return scoreCeiling
	}
	return v
}
