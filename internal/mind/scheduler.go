package mind

import (
	"context"
	"log"
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
	limiter  *LLMRateLimiter
	decCfg   DecisionConfig
	nextTick map[string]time.Time
	mu       sync.Mutex
	ticker   *time.Ticker
	onTick   func(TickResult)
}

// NewScheduler creates a scheduler. limiter can be nil (no global/per-guild limit). onTick can be set with SetOnTick.
func NewScheduler(store *Store, limiter *LLMRateLimiter, onTick func(TickResult)) *Scheduler {
	return &Scheduler{
		store:    store,
		limiter:  limiter,
		decCfg:   DefaultDecisionConfig(),
		nextTick: make(map[string]time.Time),
		onTick:   onTick,
	}
}

// SetRateLimiter sets the LLM rate limiter (e.g. after Runner creates it).
func (s *Scheduler) SetRateLimiter(l *LLMRateLimiter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.limiter = l
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

	// 2. Apply exponential activity decay by elapsed time
	g.ApplyActivityDecay(now)
	g.SaveActivity()

	// 3. Decision: DesireToSpeak (with Fatigue, runaway, rate limit)
	act := g.GetActivity()
	emoAct := EmotionalActivation(e)
	msgs := g.GetShortBuffer()
	topicRel := TopicRelevanceFromBuffer(msgs, 5*time.Minute)
	hasMention := HasRecentMention(msgs, 5*time.Minute, now)
	actNorm := act.Score / 100.0
	if actNorm > 1 {
		actNorm = 1
	}
	desire := DesireToSpeak(s.decCfg, DesireToSpeakInput{
		ActivityScoreNorm:     actNorm,
		EmotionalActivation:  emoAct,
		TopicRelevance:       topicRel,
		Fatigue:              e.Fatigue,
		LastSpokeAt:          act.LastSpokeAt,
		ConsecutiveBotReplies: act.ConsecutiveBotReplies,
		HasRecentMention:     hasMention,
		Now:                  now,
	})
	// Topic continuity: don't repeat a question we just asked
	if act.LastAIAction == "asked_question" && now.Sub(act.LastAITimestamp) < 90*time.Second {
		desire *= 0.5
	}

	shouldSpeak := desire >= s.decCfg.SpeakThreshold
	if shouldSpeak && act.AwaitingReply && now.Sub(act.AwaitingReplySince) < AwaitingReplyTimeout {
		shouldSpeak = false
		log.Printf("[MIND] awaiting_reply guild=%s topic=%s (block proactive)", guildID, act.AwaitingTopic)
	}
	rateLimited := false
	if shouldSpeak && s.limiter != nil && !s.limiter.Allow(guildID, act.LastLLMCallAt, now) {
		shouldSpeak = false
		rateLimited = true
	}
	if act.ConsecutiveBotReplies >= s.decCfg.RunawayMaxConsecutive {
		e.Engagement = clamp01(e.Engagement - 0.02)
		g.SetEmotions(e)
	}

	g.SetLastTickAt(now)

	log.Printf("[MIND] tick guild=%s desire=%.2f threshold=%.2f shouldSpeak=%v activity=%.1f fatigue=%.2f topicRel=%.2f",
		guildID, desire, s.decCfg.SpeakThreshold, shouldSpeak, act.Score, e.Fatigue, topicRel)
	if rateLimited {
		log.Printf("[MIND] rate_limited guild=%s (global or per-guild cooldown)", guildID)
	}

	res := TickResult{
		GuildID:       guildID,
		DesireToSpeak: desire,
		ShouldSpeak:   shouldSpeak,
	}
	if s.onTick != nil {
		s.onTick(res)
	}
}
