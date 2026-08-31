package discord

import (
	"context"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/bwmarrin/discordgo"
	"github.com/keshon/server-domme/internal/config"
	"github.com/keshon/server-domme/internal/discord/cmdlogger"
	"github.com/keshon/server-domme/internal/discord/cmdsync"
	"github.com/keshon/server-domme/internal/discord/execguard"
	"github.com/keshon/server-domme/internal/storage"
	"github.com/rs/zerolog"
)

// Bot is the Discord bot. Lifecycle is managed by Run/run; handlers are wired
// in run.
type Bot struct {
	dg        *discordgo.Session
	storage   *storage.Storage
	slashCmds map[string][]*discordgo.ApplicationCommand
	cfg       *config.Config
	mu        sync.RWMutex
	log       zerolog.Logger

	cmdSyncer *cmdsync.Syncer
	cmdLogger *cmdlogger.Logger

	sessionCtx atomic.Value // *sessionCtxHolder
	cmdGuard   atomic.Value // *cmdGuardHolder

	// ready is closed on the first successful connect and never reopened, so a
	// caller can wait for "the bot is usable" without waking on every reconnect.
	ready     chan struct{}
	readyOnce sync.Once
}

// Session returns the current gateway session, or nil before the first connect.
//
// Long-lived services must call this per use rather than capturing the result:
// RunSession builds a fresh *discordgo.Session on every restart, so a captured
// pointer goes stale and its writes silently target a closed connection.
func (b *Bot) Session() *discordgo.Session {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.dg
}

// Ready returns a channel closed once the bot has connected at least once.
// Services that need a live session should wait on it before their first use.
func (b *Bot) Ready() <-chan struct{} {
	return b.ready
}

type sessionCtxHolder struct {
	ctx context.Context
}

type cmdGuardHolder struct {
	g *execguard.Guard
}

var disabledGuard = execguard.New(0, 0)

func (b *Bot) baseSessionContext() context.Context {
	if v := b.sessionCtx.Load(); v != nil {
		if holder, ok := v.(*sessionCtxHolder); ok && holder != nil && holder.ctx != nil {
			return holder.ctx
		}
	}
	return context.Background()
}

func (b *Bot) guard() *execguard.Guard {
	if v := b.cmdGuard.Load(); v != nil {
		if holder, ok := v.(*cmdGuardHolder); ok && holder != nil && holder.g != nil {
			return holder.g
		}
	}
	return disabledGuard
}

func (b *Bot) commandContext() (context.Context, context.CancelFunc) {
	base := b.baseSessionContext()
	return b.guard().Context(base)
}

func (b *Bot) acquireCommandSlot(ctx context.Context) error {
	return b.guard().Acquire(ctx)
}

func (b *Bot) releaseCommandSlot() {
	b.guard().Release()
}

func (b *Bot) isGuildBlacklisted(guildID string) bool {
	return slices.Contains(b.cfg.DiscordGuildBlacklist, guildID)
}
