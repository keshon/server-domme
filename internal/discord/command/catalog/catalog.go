// Package catalog is the bot's command list: every command it offers, in one
// place, with the middleware each runs through. The binary registers it to
// serve commands, and tooling registers it to describe them.
package catalog

import (
	"github.com/keshon/command"
	"github.com/rs/zerolog"

	"github.com/keshon/server-domme/internal/discord/adapter"
	"github.com/keshon/server-domme/internal/discord/command/ask"
	"github.com/keshon/server-domme/internal/discord/command/confess"
	"github.com/keshon/server-domme/internal/discord/command/core/about"
	"github.com/keshon/server-domme/internal/discord/command/roll"
	"github.com/keshon/server-domme/internal/discord/middleware"
)

// Register adds every command to command.DefaultRegistry.
//
// Commands register here as they are rewritten to the disgo adapter, commit
// by commit, until the discordgo cmdadapter is gone.
func Register(log zerolog.Logger) {
	mw := defaultMiddleware(log)
	adapter.Register(&about.Command{}, mw...)
	adapter.Register(&roll.RollCommand{}, mw...)
	adapter.Register(&ask.AskCommand{}, mw...)
	adapter.Register(&confess.ConfessCommand{}, mw...)
}

func defaultMiddleware(log zerolog.Logger) []command.Middleware {
	return []command.Middleware{
		middleware.WithGroupAccessCheck(),
		middleware.WithGuildOnly(),
		middleware.WithUserPermissionCheck(),
		middleware.WithCommandLogger(log),
	}
}
