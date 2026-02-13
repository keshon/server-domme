package mind

import (
	"context"
	"log"
	"strings"
	"time"

	"server-domme/internal/ai"
	"server-domme/internal/config"

	"github.com/bwmarrin/discordgo"
)

// Runner wires mind Store + Scheduler to Discord and AI. Call IngestMessage from message handler, Start from bot after Open.
type Runner struct {
	Store     *Store
	Scheduler *Scheduler
	budget    TokenBudgetManager
}

// NewRunner creates Runner with default token budget.
func NewRunner(dataRoot string) *Runner {
	store := NewStore(dataRoot)
	return &Runner{
		Store:     store,
		Scheduler: NewScheduler(store, nil),
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
}

// IngestMessage records a message in the guild mind and notifies the scheduler. Call from onMessageCreate for guild messages.
func (r *Runner) IngestMessage(guildID, channelID, userID, username, content string, mentioned bool) {
	if guildID == "" || channelID == "" {
		return
	}
	g := r.Store.Guild(guildID)
	role := "user"
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

// handleTick runs on each scheduler tick. If ShouldSpeak, builds context, calls LLM, sends reply.
func (r *Runner) handleTick(ctx context.Context, session *discordgo.Session, provider ai.Provider, cfg *config.Config, tr TickResult) {
	if !tr.ShouldSpeak {
		return
	}
	g := r.Store.Guild(tr.GuildID)
	act := g.GetActivity()
	if act.LastChannelID == "" {
		return
	}
	shortBuf := g.GetShortBuffer()
	if len(shortBuf) == 0 {
		return
	}

	core := r.Store.Core()
	messages := BuildMessagesForLLM(core, g, shortBuf, r.budget)
	reply, err := provider.Generate(messages)
	if err != nil {
		log.Printf("[MIND] LLM generate failed for guild %s: %v", tr.GuildID, err)
		return
	}

	// Send to last active channel
	channelID := act.LastChannelID
	for _, chunk := range splitMessage(reply, 2000) {
		if _, err := session.ChannelMessageSend(channelID, chunk); err != nil {
			log.Printf("[MIND] send failed %s: %v", channelID, err)
			return
		}
		time.Sleep(200 * time.Millisecond)
	}

	// Record that we spoke and add assistant message to buffer
	g.SetLastSpokeAt(time.Now())
	g.PushMessage(ShortMessage{
		Role:      "assistant",
		Content:   reply,
		ChannelID: channelID,
		At:        time.Now(),
	})
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
