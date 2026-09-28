package discord

import (
	"context"

	"github.com/keshon/server-domme/internal/config"
	"github.com/keshon/server-domme/internal/discord/adapter"
	"github.com/keshon/server-domme/internal/discord/queue"
	"github.com/keshon/server-domme/internal/storage"
	"github.com/rs/zerolog"
)

// NewBot creates a Bot. Register any bot-dependent commands before calling
// RunSession.
func NewBot(cfg *config.Config, storage *storage.Storage, log zerolog.Logger) *Bot {
	b := &Bot{
		cfg:     cfg,
		storage: storage,
		log:     log,
		ready:   make(chan struct{}),
	}
	b.commands = queue.New(log, cfg.CommandParallelism)
	b.setSessionContext(context.Background())
	return b
}

// drainCommands stops accepting commands and waits for the ones already
// running. Call on shutdown.
func (b *Bot) drainCommands() {
	if b.commands == nil {
		return
	}
	// closeWithin already bounds this phase and reports what it took, so the
	// wait here is unbounded on purpose: two budgets for one step means the
	// log names a timeout that is not the one that fired.
	b.commands.Close(context.Background())
	b.log.Info().Msg("commands_drained")
}

// sessionAPI is the neutral surface over the live connection, or nil when
// there is none — which is a normal state between restarts.
func (b *Bot) sessionAPI() adapter.BotAPI {
	c := b.currentConn()
	if c == nil {
		return nil
	}
	return c.API()
}
