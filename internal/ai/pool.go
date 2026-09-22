package ai

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
// preferred backend does not flip her voice every ninety seconds.
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
	// answer, and every attempt in the meantime costs a request. See
	// ErrBackendRefused.
	refusedCooldown = 30 * time.Minute
	attemptsPerTry  = 2
)

// Mode is how the pool orders its backends.
type Mode string

// Modes.
const (
	// ModePriority tries backends in the order the operator gave, and moves
	// past one only while it is cooling down. Which model speaks is the
	// operator's choice, and it stays the same from call to call.
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

// Pool sends to the first backend that will answer.
//
// Two orders share one set of backends and one record of their health: the
// general order, which thinking uses, and the voice order, which the words she
// says use (see Voice). Health is learned once, whichever call discovered it.
type Pool struct {
	mu       sync.Mutex
	backends []*backend
	mode     Mode
	// order is every backend's name, most preferred first.
	order []string
	// voice is the names the voice prefers, in order. Empty means the voice
	// uses order.
	voice []string
	log   zerolog.Logger
}

// NewPool returns a Pool over clients, in ModePriority, in the order given.
//
// Backends are addressed by name, so a name given twice is made unique with a
// suffix rather than left to shadow the first.
func NewPool(log zerolog.Logger, clients ...*Client) *Pool {
	p := &Pool{log: log, mode: ModePriority}
	for _, c := range clients {
		if c == nil {
			continue
		}
		base := c.Name
		for i := 2; p.byNameLocked(c.Name) != nil; i++ {
			c.Name = fmt.Sprintf("%s#%d", base, i)
		}
		p.backends = append(p.backends, &backend{client: c})
		p.order = append(p.order, c.Name)
	}
	return p
}

// Len reports how many backends the pool holds.
func (p *Pool) Len() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.backends)
}

// preferKey carries the backend a caller would rather use; see WithPrefer.
type preferKey struct{}

// voiceKey marks a call as the voice's; see Voice.
type voiceKey struct{}

// WithPrefer asks the pool to try the named backend first, unless it is
// cooling down or switched off. It is how a conversation keeps one voice after
// a failover: the backend it failed over to stays first even once the
// preferred one recovers.
func WithPrefer(ctx context.Context, name string) context.Context {
	if name == "" {
		return ctx
	}
	return context.WithValue(ctx, preferKey{}, name)
}

func forVoice(ctx context.Context) context.Context {
	return context.WithValue(ctx, voiceKey{}, true)
}

// Generate implements Provider.
func (p *Pool) Generate(ctx context.Context, messages []Message) (string, error) {
	reply, _, err := p.GenerateNamed(ctx, messages)
	return reply, err
}

// GenerateNamed is Generate, also reporting which backend answered, for a
// caller that keeps a record of its replies.
//
// Each backend gets attemptsPerTry attempts before the next is tried, and
// ErrNoBackend comes back when none of them answered.
func (p *Pool) GenerateNamed(ctx context.Context, messages []Message) (string, string, error) {
	var lastErr error
	for _, b := range p.ready(ctx, time.Now()) {
		for attempt := 1; attempt <= attemptsPerTry; attempt++ {
			if ctx.Err() != nil {
				return "", "", ctx.Err()
			}
			reply, err := b.client.Generate(ctx, messages)
			if err == nil {
				p.recordSuccess(b)
				p.log.Debug().
					Str("backend", b.client.Name).
					Int("attempt", attempt).
					Msg("ai_generate_succeeded")
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
				Msg("ai_generate_failed")
			// A cancelled or timed-out context is the caller giving up, not
			// the backend failing. Trying the next one would run past the
			// deadline the caller set.
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return "", "", err
			}
			// Move to the next backend rather than spending the second attempt
			// on an answer that will not change.
			if refused {
				break
			}
		}
	}
	if lastErr == nil {
		return "", "", fmt.Errorf("ai: %w: all backends in cooldown or switched off", ErrNoBackend)
	}
	return "", "", fmt.Errorf("ai: %w: last error: %w", ErrNoBackend, lastErr)
}

// ready returns the backends worth trying now, in the order to try them.
//
// A backend in cooldown is skipped unless every backend is cooling down, in
// which case the cooldowns are ignored: refusing to speak because every relay
// misbehaved in the last minute is worse than trying the least-bad one. A
// backend switched off is never tried.
func (p *Pool) ready(ctx context.Context, now time.Time) []*backend {
	p.mu.Lock()
	defer p.mu.Unlock()

	ranked := p.rankedLocked(ctx.Value(voiceKey{}) != nil)
	if name, _ := ctx.Value(preferKey{}).(string); name != "" {
		for i, b := range ranked {
			if b.client.Name == name && !b.cooldownUntil.After(now) {
				ranked = append([]*backend{b}, slices.Delete(ranked, i, i+1)...)
				break
			}
		}
	}

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

// rankedLocked is every backend that is switched on, in the order to try
// them. The voice's order goes first when it has one, then the rest: a voice
// order is a preference, and staying silent because every preferred voice is
// down is worse than speaking through another.
func (p *Pool) rankedLocked(voice bool) []*backend {
	var names []string
	if voice && len(p.voice) > 0 {
		names = append(names, p.voice...)
		for _, n := range p.orderedNamesLocked() {
			if !slices.Contains(names, n) {
				names = append(names, n)
			}
		}
	} else {
		names = p.orderedNamesLocked()
	}

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
	// A refusal does not get a second attempt: the answer is already known,
	// and asking again only spends another request to hear it.
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

// Voice returns a Provider that speaks through the voice order. It shares
// this pool's backends and what it has learned about them.
func (p *Pool) Voice() *VoicePool { return &VoicePool{pool: p} }

// VoicePool is a Pool as the voice uses it. See Pool.Voice.
type VoicePool struct{ pool *Pool }

// Generate implements Provider.
func (v *VoicePool) Generate(ctx context.Context, messages []Message) (string, error) {
	return v.pool.Generate(forVoice(ctx), messages)
}

// GenerateNamed is Generate, also reporting which backend answered.
func (v *VoicePool) GenerateNamed(ctx context.Context, messages []Message) (string, string, error) {
	return v.pool.GenerateNamed(forVoice(ctx), messages)
}

// Settings is how an operator has arranged the pool: what survives a restart.
type Settings struct {
	Mode  Mode
	Order []string
	Voice []string
	Off   []string
}

// Settings reports the pool's current arrangement.
func (p *Pool) Settings() Settings {
	p.mu.Lock()
	defer p.mu.Unlock()
	st := Settings{Mode: p.mode, Order: slices.Clone(p.order), Voice: slices.Clone(p.voice)}
	for _, b := range p.backends {
		if b.off {
			st.Off = append(st.Off, b.client.Name)
		}
	}
	return st
}

// Apply restores an arrangement. It is tolerant, because a stored
// arrangement can outlive the backends it names: an unknown name is skipped
// and reported, never fatal, and a backend new since the arrangement was saved
// keeps its place after the ones it names. It never switches every backend
// off.
func (p *Pool) Apply(st Settings) (unknown []string) {
	if st.Mode != "" {
		p.SetMode(st.Mode)
	}
	known := func(names []string) []string {
		var out []string
		for _, n := range names {
			if p.has(n) {
				out = append(out, n)
			} else {
				unknown = append(unknown, n)
			}
		}
		return out
	}
	if order := known(st.Order); len(order) > 0 {
		_ = p.SetOrder(order)
	}
	_ = p.SetVoiceOrder(known(st.Voice))
	for _, n := range known(st.Off) {
		_ = p.SetOff(n, true)
	}
	return unknown
}

func (p *Pool) has(name string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.byNameLocked(name) != nil
}

// ErrUnknownBackend is an operator naming a backend the pool does not have.
var ErrUnknownBackend = errors.New("no backend by that name")

// ErrLastBackend is an operator switching off the only backend left on.
var ErrLastBackend = errors.New("that is the last backend switched on")

// SetMode changes how the general order is decided.
func (p *Pool) SetMode(m Mode) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.mode = m
}

// SetOrder puts the named backends first, in the order given; the others keep
// their relative order after them.
func (p *Pool) SetOrder(names []string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.checkNamesLocked(names); err != nil {
		return err
	}
	order := slices.Clone(names)
	for _, n := range p.order {
		if !slices.Contains(order, n) {
			order = append(order, n)
		}
	}
	p.order = order
	return nil
}

// SetVoiceOrder sets the backends the voice prefers, in order. Empty clears
// it, and the voice then uses the general order.
func (p *Pool) SetVoiceOrder(names []string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.checkNamesLocked(names); err != nil {
		return err
	}
	p.voice = slices.Compact(slices.Clone(names))
	return nil
}

// SetOff switches a backend off, or back on.
func (p *Pool) SetOff(name string, off bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	b := p.byNameLocked(name)
	if b == nil {
		return fmt.Errorf("%w: %q", ErrUnknownBackend, name)
	}
	if off && !b.off {
		on := 0
		for _, o := range p.backends {
			if !o.off {
				on++
			}
		}
		if on <= 1 {
			return ErrLastBackend
		}
	}
	b.off = off
	return nil
}

func (p *Pool) checkNamesLocked(names []string) error {
	for _, n := range names {
		if p.byNameLocked(n) == nil {
			return fmt.Errorf("%w: %q", ErrUnknownBackend, n)
		}
	}
	return nil
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
	// Off is an operator having switched it off.
	Off bool
	// Voice is its place in the voice order, from 1; 0 when it is not in it.
	Voice int
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
			Voice:     slices.Index(p.voice, n) + 1,
		})
	}
	return out
}

// Mode reports how the general order is decided.
func (p *Pool) Mode() Mode {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.mode
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
