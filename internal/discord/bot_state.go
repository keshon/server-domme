package discord

import (
	"context"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/keshon/server-domme/internal/config"
	"github.com/keshon/server-domme/internal/discord/adapter"
	"github.com/keshon/server-domme/internal/discord/queue"
	"github.com/keshon/server-domme/internal/storage"
	"github.com/rs/zerolog"
)

// Bot is the Discord bot. Lifecycle is managed by RunSession; handlers are
// wired in session_run.go.
type Bot struct {
	storage *storage.Storage
	cfg     *config.Config
	log     zerolog.Logger

	// commands runs command bodies off the gateway read goroutine. Process
	// lifetime: a command outliving the session it arrived on is a command
	// that cannot answer, not a command to abandon halfway.
	commands *queue.Queue

	// Replaced wholesale when a session opens and cleared when one closes, so
	// a reader gets a live one or the fallback, never a half-torn one.
	sessionCtx atomic.Pointer[context.Context]
	conn       atomic.Pointer[conn]

	// ready is closed on the first successful connect and never reopened, so
	// a caller can wait for "the bot is usable" without waking on every
	// reconnect.
	ready     chan struct{}
	readyOnce sync.Once
}

// slotWaitBudget bounds how long a command waits for a free slot before it
// gives up and says the bot is busy.
//
// It is short because it is measured against Discord's deadline, not ours: an
// interaction must be acknowledged within three seconds of being created, and
// a command that has not started by then cannot answer at all.
const slotWaitBudget = 2 * time.Second

func (b *Bot) setSessionContext(ctx context.Context) { b.sessionCtx.Store(&ctx) }

func (b *Bot) baseSessionContext() context.Context {
	if ctx := b.sessionCtx.Load(); ctx != nil && *ctx != nil {
		return *ctx
	}
	return context.Background()
}

// commandContext is the session's context, cancelled when the session ends.
// It carries no deadline: nothing downstream of here takes a context, so one
// would only be a claim.
func (b *Bot) commandContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(b.baseSessionContext())
}

func (b *Bot) acquireCommandSlot(ctx context.Context) error {
	return b.commands.Acquire(ctx)
}

func (b *Bot) releaseCommandSlot() {
	b.commands.Release()
}

// Ready returns a channel closed once the bot has connected at least once.
// Services that need a live session wait on it before their first use.
func (b *Bot) Ready() <-chan struct{} {
	return b.ready
}

// SessionAPI is the neutral surface over the live connection, or nil between
// sessions. Long-lived services resolve it per use rather than capturing it.
func (b *Bot) SessionAPI() adapter.BotAPI {
	return b.sessionAPI()
}

func (b *Bot) markReady() {
	b.readyOnce.Do(func() { close(b.ready) })
}

func (b *Bot) isGuildBlacklisted(guildID string) bool {
	return slices.Contains(b.cfg.DiscordGuildBlacklist, guildID)
}
