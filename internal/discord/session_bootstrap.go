package discord

import (
	"context"
	"errors"

	"github.com/bwmarrin/discordgo"
	"github.com/keshon/server-domme/internal/config"
	"github.com/keshon/server-domme/internal/storage"
	"github.com/rs/zerolog"
)

// NewBot creates a Bot. Register any bot-dependent commands before calling Run.
func NewBot(cfg *config.Config, storage *storage.Storage, log zerolog.Logger) *Bot {
	b := &Bot{
		cfg:       cfg,
		storage:   storage,
		log:       log,
		slashCmds: make(map[string][]*discordgo.ApplicationCommand),
		ready:     make(chan struct{}),
	}
	b.sessionCtx.Store(&sessionCtxHolder{ctx: context.Background()})
	b.cmdGuard.Store(&cmdGuardHolder{g: disabledGuard})
	return b
}

func (b *Bot) configureIntents() {
	b.dg.Identify.Intents = discordgo.IntentsAll
}

// IsSessionUnhealthyError reports whether an error means we should fast-restart the session.
func IsSessionUnhealthyError(err error) bool {
	return errors.Is(err, ErrSessionUnhealthy)
}
