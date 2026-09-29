package llm

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// Pool scoring and cooldown constants.
//
// The score only orders backends in ModeScore; in ModePriority it is kept for
// operators to read. Cooldown is what stops a dead backend being retried on
// every single message, and it grows with each consecutive failure so a flaky
// preferred backend does not flip the voice every ninety seconds.
const (
	scoreOnSuccess  = 2.0
	scoreOnFailure  = -3.0
	scoreFloor      = -20.0
	scoreCeiling    = 20.0
	backendCooldown = 90 * time.Second
	// maxCooldown caps the escalation: 90s, 3m, 6m … 30m.
	maxCooldown = 30 * time.Minute
	// refusedCooldown rests a backend that answered with no credit, no key or
	// not allowed. Long, because nothing this process does will change the
	// answer.
	refusedCooldown = 30 * time.Minute
	attemptsPerTry  = 2
)

// Mode is how the pool orders its backends.
type Mode string

// Modes.
const (
	// ModePriority tries backends in the order the operator gave, and moves
	// past one only while it is cooling down.
	ModePriority Mode = "priority"
	// ModeScore orders backends by what they have done lately, best first.
	// Resilient, but the model answering wanders from call to call.
	ModeScore Mode = "score"
)

// ParseMode reads a mode by name; anything unrecognised is ModePriority.
func ParseMode(s string) (Mode, bool) {
	switch Mode(strings.ToLower(strings.TrimSpace(s))) {
	case ModeScore:
		return ModeScore, true
	case ModePriority, "":
		return ModePriority, true
	}
	return ModePriority, false
}

// backend pairs a Client with what the Pool has learned about it. All fields
// are guarded by Pool.mu.
type backend struct {
	client        *Client
	score         float64
	successes     int
	failures      int
	cooldownUntil time.Time
	lastErr       string
	// streak counts cooldowns in a row, and sets how long the next one is.
	// One success clears it.
	streak int
	// off is an operator's decision to leave this backend out entirely.
	off bool
}

// Pool sends to the first backend that will answer. It shares one set of
// backends and one record of their health across every caller, so a relay
// that fails for /summarize is already cooling down when knowledge asks.
type Pool struct {
	mu       sync.Mutex
	backends []*backend
	mode     Mode
	// order is every backend's name, most preferred first.
	order []string
	log   zerolog.Logger
}

// NewPool returns a Pool over clients, in ModePriority, in the order given.
//
// Backends are addressed by name, so a name given twice is made unique with a
// suffix rather than left to shadow the first.
func NewPool(log zerolog.Logger, clients ...*Client) *Pool {
	p := &Pool{log: log, mode: ModePriority}
	for _, c := range clients {
		p.addLocked(c)
	}
	return p
}

func (p *Pool) addLocked(c *Client) {
	if c == nil {
		return
	}
	base := c.Name
	if base == "" {
		base = c.Model
		if base == "" {
			base = "backend"
		}
		c.Name = base
	}
	for i := 2; p.byNameLocked(c.Name) != nil; i++ {
		c.Name = fmt.Sprintf("%s#%d", base, i)
	}
	p.backends = append(p.backends, &backend{client: c})
	p.order = append(p.order, c.Name)
}

// Add appends a backend after construction, e.g. g4f.space catalogue picks
// that arrived after startup. Safe for concurrent use with Complete.
func (p *Pool) Add(c *Client) {
	if p == nil || c == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.addLocked(c)
}

// Len reports how many backends the pool holds.
func (p *Pool) Len() int {
	if p == nil {
		return 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.backends)
}

// Complete implements Provider: it tries backends in order until one answers.
// Each backend gets attemptsPerTry attempts before the next is tried, and
// ErrNoBackend comes back when none of them answered.
func (p *Pool) Complete(ctx context.Context, messages []Message, temperature *float64) (string, error) {
	reply, _, err := p.CompleteNamed(ctx, messages, temperature)
	return reply, err
}

// CompleteNamed is Complete, also reporting which backend answered, for
// logging which relay actually speaks.
func (p *Pool) CompleteNamed(ctx context.Context, messages []Message, temperature *float64) (string, string, error) {
	if p == nil {
		return "", "", fmt.Errorf("llm: %w: pool is nil", ErrNoBackend)
	}
	var lastErr error
	for _, b := range p.ready(time.Now()) {
		for attempt := 1; attempt <= attemptsPerTry; attempt++ {
			if ctx.Err() != nil {
				return "", "", ctx.Err()
			}
			reply, err := b.client.Complete(ctx, messages, temperature)
			if err == nil {
				p.recordSuccess(b)
				p.log.Debug().
					Str("backend", b.client.Name).
					Int("attempt", attempt).
					Msg("llm_generate_succeeded")
				return reply, b.client.Name, nil
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
				Msg("llm_generate_failed")
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return "", "", err
			}
			if refused {
				break
			}
		}
	}
	if lastErr == nil {
		return "", "", fmt.Errorf("llm: %w: all backends in cooldown or switched off", ErrNoBackend)
	}
	return "", "", fmt.Errorf("llm: %w: last error: %w", ErrNoBackend, lastErr)
}

// ready returns the backends worth trying now, in the order to try them.
//
// A backend in cooldown is skipped unless every backend is cooling down, in
// which case the cooldowns are ignored: refusing to speak because every relay
// misbehaved in the last minute is worse than trying the least-bad one. A
// backend switched off is never tried.
func (p *Pool) ready(now time.Time) []*backend {
	p.mu.Lock()
	defer p.mu.Unlock()

	ranked := p.rankedLocked()
	available := make([]*backend, 0, len(ranked))
	for _, b := range ranked {
		if !b.cooldownUntil.After(now) {
			available = append(available, b)
		}
	}
	if len(available) == 0 {
		return ranked
	}
	return available
}

// rankedLocked is every backend that is switched on, in the order to try them.
func (p *Pool) rankedLocked() []*backend {
	names := p.orderedNamesLocked()
	out := make([]*backend, 0, len(names))
	for _, n := range names {
		if b := p.byNameLocked(n); b != nil && !b.off {
			out = append(out, b)
		}
	}
	return out
}

// orderedNamesLocked is the general order: the operator's in ModePriority,
// best score first in ModeScore.
func (p *Pool) orderedNamesLocked() []string {
	names := slices.Clone(p.order)
	if p.mode == ModeScore {
		sort.SliceStable(names, func(i, j int) bool {
			return p.byNameLocked(names[i]).score > p.byNameLocked(names[j]).score
		})
	}
	return names
}

func (p *Pool) byNameLocked(name string) *backend {
	for _, b := range p.backends {
		if b.client.Name == name {
			return b
		}
	}
	return nil
}

func (p *Pool) recordSuccess(b *backend) {
	p.mu.Lock()
	defer p.mu.Unlock()
	b.successes++
	b.lastErr = ""
	b.cooldownUntil = time.Time{}
	b.streak = 0
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
	if errors.Is(err, ErrBackendRefused) {
		b.cooldownUntil = now.Add(refusedCooldown)
		return true
	}
	if attempt >= attemptsPerTry {
		b.streak++
		b.cooldownUntil = now.Add(cooldownFor(b.streak))
		return true
	}
	return false
}

// cooldownFor is how long a backend rests after its streak-th cooldown in a
// row: doubling from backendCooldown, capped at maxCooldown.
func cooldownFor(streak int) time.Duration {
	d := backendCooldown
	for i := 1; i < streak && d < maxCooldown; i++ {
		d *= 2
	}
	return min(d, maxCooldown)
}

// SetMode changes how the general order is decided.
func (p *Pool) SetMode(m Mode) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.mode = m
}

// Mode reports how the general order is decided.
func (p *Pool) Mode() Mode {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.mode
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
	Off       bool
}

// Stats returns a snapshot of every backend, in the general order.
func (p *Pool) Stats() []BackendStat {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now()
	out := make([]BackendStat, 0, len(p.backends))
	for _, n := range p.orderedNamesLocked() {
		b := p.byNameLocked(n)
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
			Off:       b.off,
		})
	}
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

var _ Provider = (*Pool)(nil)
