package mind

import (
	"context"
	"sync"
	"time"
)

// TickInterval returns how often to tick a guild based on ActivityScore (0..100).
// Low activity -> 30-60s, medium -> 10-20s, high -> 2-5s.
func TickInterval(activityScore float64) time.Duration {
	switch {
	case activityScore >= 60:
		return 4*time.Second + time.Duration(100-activityScore)*40*time.Millisecond
	case activityScore >= 20:
		return 10*time.Second + time.Duration(60-activityScore)*166*time.Millisecond
	default:
		return 30*time.Second + time.Duration(20-activityScore)*2*time.Second/20
	}
}

// TickResult is passed to OnTick so the handler can decide to call LLM.
type TickResult struct {
	GuildID       string
	DesireToSpeak float64
	ShouldSpeak   bool // DesireToSpeak >= threshold
}

// Scheduler runs one goroutine, processes guilds by priority (next tick time).
type Scheduler struct {
	store    *Store
	decCfg   DecisionConfig
	nextTick map[string]time.Time
	mu       sync.Mutex
	ticker   *time.Ticker
	onTick   func(TickResult) // called when a guild is chosen to tick
}

// NewScheduler creates a scheduler. onTick can be nil; set with SetOnTick before Run.
func NewScheduler(store *Store, onTick func(TickResult)) *Scheduler {
	return &Scheduler{
		store:    store,
		decCfg:   DefaultDecisionConfig(),
		nextTick: make(map[string]time.Time),
		onTick:   onTick,
	}
}

// SetOnTick sets the tick callback (e.g. after Discord session is available).
func (s *Scheduler) SetOnTick(f func(TickResult)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onTick = f
}

// SetDecisionConfig overrides default decision params.
func (s *Scheduler) SetDecisionConfig(cfg DecisionConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.decCfg = cfg
}

// NotifyMessage is called when a message arrives in a guild (from bot event handler).
// It ensures the guild is in the scheduler and advances its next tick.
func (s *Scheduler) NotifyMessage(guildID string) {
	_ = s.store.Guild(guildID) // ensure guild state exists
	s.mu.Lock()
	now := time.Now()
	// Next tick soon for this guild (high priority)
	s.nextTick[guildID] = now.Add(2 * time.Second)
	s.mu.Unlock()
}

// Run starts the single scheduler goroutine. Stops when ctx is done.
func (s *Scheduler) Run(ctx context.Context) {
	s.ticker = time.NewTicker(2 * time.Second)
	defer s.ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.ticker.C:
			s.tick(ctx)
		}
	}
}

func (s *Scheduler) tick(ctx context.Context) {
	now := time.Now()
	var chosen string
	s.mu.Lock()
	for gid, next := range s.nextTick {
		if next.After(now) {
			continue
		}
		if chosen == "" || next.Before(s.nextTick[chosen]) {
			chosen = gid
		}
	}
	if chosen == "" {
		s.mu.Unlock()
		return
	}
	g := s.store.Guild(chosen)
	a := g.GetActivity()
	interval := TickInterval(a.Score)
	s.nextTick[chosen] = now.Add(interval)
	s.mu.Unlock()

	s.runGuildTick(chosen, g, now)
}

func (s *Scheduler) runGuildTick(guildID string, g *GuildState, now time.Time) {
	// 1. Decay emotions
	e := g.GetEmotions()
	e = DecayEmotionsSinceLastUpdate(e, now)
	g.SetEmotions(e)

	// 2. Decay activity (per second decay)
	g.ApplyActivityDecay(0.02, now)
	g.SaveActivity()

	// 3. Decision: DesireToSpeak
	act := g.GetActivity()
	emoAct := EmotionalActivation(e)
	msgs := g.GetShortBuffer()
	topicRel := TopicRelevanceFromBuffer(msgs, 5*time.Minute)
	desire := DesireToSpeak(s.decCfg, act.Score, emoAct, topicRel, act.LastSpokeAt, now)

	g.SetLastTickAt(now)

	res := TickResult{
		GuildID:       guildID,
		DesireToSpeak: desire,
		ShouldSpeak:   desire >= s.decCfg.SpeakThreshold,
	}
	if s.onTick != nil {
		s.onTick(res)
	}
}
