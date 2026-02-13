package mind

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"sync"
	"time"

	"server-domme/internal/ai"
	"server-domme/internal/config"

	"github.com/bwmarrin/discordgo"
)

// Proactive reflection: max 1 per interval, only when idle and engaged.
const (
	MinReflectionInterval = 10 * time.Minute
	ReflectionProbability = 0.03
	ReflectionActivityMax = 15.0  // ActivityScore below this to consider "idle"
	ReflectionEngagementMin = 0.25
)

// Runner wires mind Store + Scheduler to Discord and AI. Call IngestMessage from message handler, Start from bot after Open.
type Runner struct {
	Store             *Store
	Scheduler         *Scheduler
	Limiter           *LLMRateLimiter
	budget            TokenBudgetManager
	lastReflectionAt  time.Time
	reflectionMu      sync.Mutex
}

// NewRunner creates Runner with default token budget and rate limiter.
func NewRunner(dataRoot string) *Runner {
	store := NewStore(dataRoot)
	limiter := DefaultLLMLimiter()
	sched := NewScheduler(store, limiter, nil)
	return &Runner{
		Store:     store,
		Scheduler: sched,
		Limiter:   limiter,
		budget:    DefaultTokenBudget(),
	}
}

// Start starts the scheduler goroutine and sets the tick handler. Call after Discord session is open.
func (r *Runner) Start(ctx context.Context, session *discordgo.Session, provider ai.Provider, cfg *config.Config) {
	if session == nil || provider == nil {
		return
	}
	r.Scheduler.SetOnTick(func(tr TickResult) {
		r.handleTick(ctx, session, provider, cfg, tr)
	})
	go r.Scheduler.Run(ctx)
	go r.runWorldviewEvolution(ctx, provider)
}

// runWorldviewEvolution runs rare worldview evolution (e.g. every 6 hours).
func (r *Runner) runWorldviewEvolution(ctx context.Context, provider ai.Provider) {
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ids := r.Store.AllGuildIDs()
			if len(ids) == 0 {
				continue
			}
			guildID := ids[rand.Intn(len(ids))]
			log.Printf("[MIND] action=evolution guild=%s (scheduled)", guildID)
			g := r.Store.Guild(guildID)
			core := r.Store.Core()
			if err := EvolveWorldview(provider, core, g, guildID); err != nil {
				log.Printf("[MIND] worldview evolution failed guild %s: %v", guildID, err)
			} else {
				log.Printf("[MIND] evolution done guild=%s", guildID)
			}
		}
	}
}

// BuildMessagesForReactiveChat builds LLM messages for reactive (mention) reply using mind context.
// Filters short buffer by channelID. Call from chat command when mind is available.
func (r *Runner) BuildMessagesForReactiveChat(guildID, channelID string) ([]ai.Message, error) {
	if guildID == "" || channelID == "" {
		return nil, fmt.Errorf("guildID and channelID required")
	}
	g := r.Store.Guild(guildID)
	shortBuf := g.GetShortBuffer()
	var filtered []ShortMessage
	for _, m := range shortBuf {
		if m.ChannelID == channelID {
			filtered = append(filtered, m)
		}
	}
	if len(filtered) == 0 {
		return nil, fmt.Errorf("no messages in channel %s", channelID)
	}
	core := r.Store.Core()
	return BuildMessagesForLLM(core, g, filtered, r.budget, r.Store), nil
}

// RecordAssistantReply pushes a reactive (mention) AI reply into the guild short buffer so proactive tick sees it.
// Call from the chat command after sending the reply to Discord.
func (r *Runner) RecordAssistantReply(guildID, channelID, reply string) {
	if guildID == "" || channelID == "" || reply == "" {
		return
	}
	g := r.Store.Guild(guildID)
	now := time.Now()
	g.PushMessage(ShortMessage{
		Role:      "assistant",
		Content:   reply,
		ChannelID: channelID,
		At:        now,
	})
	g.SetLastSpokeAt(now)
	topic := inferTopicFromReply(reply)
	if isQuestion(reply) {
		g.SetAwaitingReply(now, topic)
		g.SetLastAIIntent("asked_question", topic, now)
	} else {
		g.SetLastAIIntent("comment", topic, now)
	}
}

// IngestMessage records a message in the guild mind and notifies the scheduler. Updates person model and emotions.
func (r *Runner) IngestMessage(guildID, channelID, userID, username, content string, mentioned bool) {
	if guildID == "" || channelID == "" {
		return
	}
	g := r.Store.Guild(guildID)
	role := "user"
	kind := ClassifyMessageForPerson(content)
	if userID != "" {
		p := g.GetPerson(userID)
		if p == nil {
			p = &Person{UserID: userID}
		}
		updated := ApplyPersonUpdate(p, kind, 0.08)
		g.SetPerson(updated)
		e := g.GetEmotions()
		g.SetEmotions(BumpEmotionFromPerson(e, updated.Affinity, updated.Irritation))
	}
	g.PushMessage(ShortMessage{
		Role:      role,
		UserID:    userID,
		Username:  username,
		Content:   content,
		ChannelID: channelID,
		At:        time.Now(),
		Mentioned: mentioned,
	})
	r.Scheduler.NotifyMessage(guildID)
}

// handleTick runs on each scheduler tick. May run summarization or speak.
func (r *Runner) handleTick(ctx context.Context, session *discordgo.Session, provider ai.Provider, cfg *config.Config, tr TickResult) {
	g := r.Store.Guild(tr.GuildID)

	if g.NeedSummarization() {
		log.Printf("[MIND] action=summarize guild=%s shortChars=%d", tr.GuildID, g.GetShortCharCount())
		if err := SummarizeMemory(provider, g, tr.GuildID, r.Store); err != nil {
			log.Printf("[MIND] summarization failed guild %s: %v", tr.GuildID, err)
		} else {
			log.Printf("[MIND] summarization done guild=%s", tr.GuildID)
		}
		return
	}

	if !tr.ShouldSpeak {
		r.maybeIdleReflection(ctx, session, provider, g, tr.GuildID)
		return
	}

	log.Printf("[MIND] action=speak guild=%s channel=%s", tr.GuildID, g.GetActivity().LastChannelID)
	act := g.GetActivity()
	if act.LastChannelID == "" {
		return
	}
	shortBuf := g.GetShortBuffer()
	if len(shortBuf) == 0 {
		return
	}

	core := r.Store.Core()
	messages := BuildMessagesForLLM(core, g, shortBuf, r.budget, r.Store)
	LogLLMCall("speak", messages, map[string]string{"guild_id": tr.GuildID, "channel_id": act.LastChannelID})
	reply, err := provider.Generate(messages)
	if err != nil {
		log.Printf("[MIND] LLM generate failed for guild %s: %v", tr.GuildID, err)
		return
	}

	now := time.Now()
	if r.Limiter != nil {
		r.Limiter.Record(tr.GuildID, now)
	}
	g.SetLastLLMCallAt(now)
	e := g.GetEmotions()
	g.SetEmotions(BumpFatigueAfterLLM(e, 0.15))

	channelID := act.LastChannelID
	for _, chunk := range splitMessage(reply, 2000) {
		if _, err := session.ChannelMessageSend(channelID, chunk); err != nil {
			log.Printf("[MIND] send failed %s: %v", channelID, err)
			return
		}
		time.Sleep(200 * time.Millisecond)
	}

	g.SetLastSpokeAt(now)
	g.PushMessage(ShortMessage{
		Role:      "assistant",
		Content:   reply,
		ChannelID: channelID,
		At:        time.Now(),
	})
	topic := inferTopicFromReply(reply)
	if isQuestion(reply) {
		g.SetAwaitingReply(now, topic)
		g.SetLastAIIntent("asked_question", topic, now)
	} else {
		g.SetLastAIIntent("comment", topic, now)
	}
	replyPreview := reply
	if len(replyPreview) > 150 {
		replyPreview = replyPreview[:150] + "..."
	}
	log.Printf("[MIND] reply: %s", replyPreview)
}

// maybeIdleReflection rarely generates a proactive thought when idle and engaged.
func (r *Runner) maybeIdleReflection(ctx context.Context, session *discordgo.Session, provider ai.Provider, g *GuildState, guildID string) {
	act := g.GetActivity()
	e := g.GetEmotions()
	if act.Score > ReflectionActivityMax || e.Engagement < ReflectionEngagementMin || act.LastChannelID == "" {
		return
	}
	if !act.LastSpokeAt.IsZero() && time.Since(act.LastSpokeAt) < 3*time.Minute {
		return
	}
	r.reflectionMu.Lock()
	defer r.reflectionMu.Unlock()
	if time.Since(r.lastReflectionAt) < MinReflectionInterval {
		return
	}
	if r.Limiter != nil && !r.Limiter.Allow(guildID, act.LastLLMCallAt, time.Now()) {
		return
	}
	if rand.Float64() >= ReflectionProbability {
		return
	}
	core := r.Store.Core()
	ident := string(core.GetIdentityMD())
	if ident == "" {
		ident = "You are a character."
	}
	prompt := strings.TrimSpace(ident) + "\n\nTask: Generate one short proactive thought, question, or comment to initiate or re-engage conversation. One sentence only. No preamble, no quotes."
	msgs := []ai.Message{{Role: "system", Content: prompt}, {Role: "user", Content: "Now."}}
	LogLLMCall("reflection", msgs, map[string]string{"guild_id": guildID, "channel_id": act.LastChannelID})
	reply, err := provider.Generate(msgs)
	if err != nil {
		return
	}
	reply = strings.TrimSpace(reply)
	if reply == "" {
		return
	}
	r.lastReflectionAt = time.Now()
	if r.Limiter != nil {
		r.Limiter.Record(guildID, r.lastReflectionAt)
	}
	g.SetLastLLMCallAt(r.lastReflectionAt)
	g.SetLastSpokeAt(r.lastReflectionAt)
	e2 := g.GetEmotions()
	g.SetEmotions(BumpFatigueAfterLLM(e2, 0.1))
	for _, chunk := range splitMessage(reply, 2000) {
		if _, err := session.ChannelMessageSend(act.LastChannelID, chunk); err != nil {
			log.Printf("[MIND] reflection send failed: %v", err)
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	g.PushMessage(ShortMessage{Role: "assistant", Content: reply, ChannelID: act.LastChannelID, At: time.Now()})
	topic := inferTopicFromReply(reply)
	if isQuestion(reply) {
		g.SetAwaitingReply(time.Now(), topic)
		g.SetLastAIIntent("asked_question", topic, time.Now())
	} else {
		g.SetLastAIIntent("comment", topic, time.Now())
	}
	log.Printf("[MIND] reflection reply: %s", truncateForLog(reply, 120))
}

func isQuestion(s string) bool {
	s = strings.TrimSpace(s)
	return strings.HasSuffix(s, "?")
}

func inferTopicFromReply(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 60 {
		s = s[:60]
	}
	return s
}

func truncateForLog(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func splitMessage(msg string, limit int) []string {
	var result []string
	for len(msg) > limit {
		cut := strings.LastIndex(msg[:limit], "\n")
		if cut == -1 {
			cut = limit
		}
		result = append(result, strings.TrimSpace(msg[:cut]))
		msg = strings.TrimSpace(msg[cut:])
	}
	if msg != "" {
		result = append(result, msg)
	}
	return result
}
