// Package catalog is the bot's command list: every command it offers, in one
// place, with the middleware each runs through. The binary registers it to
// serve commands, and tooling registers it to describe them.
package catalog

import (
	"github.com/keshon/command"
	"github.com/rs/zerolog"

	"github.com/keshon/server-domme/internal/discord/adapter"
	"github.com/keshon/server-domme/internal/discord/command/announce"
	"github.com/keshon/server-domme/internal/discord/command/ask"
	"github.com/keshon/server-domme/internal/discord/command/confess"
	"github.com/keshon/server-domme/internal/discord/command/core/about"
	"github.com/keshon/server-domme/internal/discord/command/core/help"
	"github.com/keshon/server-domme/internal/discord/command/core/maintenance"
	"github.com/keshon/server-domme/internal/discord/command/discipline"
	"github.com/keshon/server-domme/internal/discord/command/media"
	"github.com/keshon/server-domme/internal/discord/command/purge"
	"github.com/keshon/server-domme/internal/discord/command/roll"
	"github.com/keshon/server-domme/internal/discord/command/settings"
	"github.com/keshon/server-domme/internal/discord/command/shortlink"
	"github.com/keshon/server-domme/internal/discord/command/summarize"
	"github.com/keshon/server-domme/internal/discord/command/task"
	"github.com/keshon/server-domme/internal/discord/command/translate"
	"github.com/keshon/server-domme/internal/discord/command/welcome"
	"github.com/keshon/server-domme/internal/discord/middleware"
)

// Register adds every command to command.DefaultRegistry.
//
// Commands register here as they are rewritten to the disgo adapter, commit
// by commit, until the discordgo cmdadapter is gone.
func Register(log zerolog.Logger) {
	mw := defaultMiddleware(log)
	adapter.Register(&about.Command{}, mw...)
	adapter.Register(&help.Command{}, mw...)
	adapter.Register(&settings.SettingsCommand{}, mw...)
	adapter.Register(&maintenance.Command{}, mw...)
	adapter.Register(&discipline.DisciplineCommand{}, mw...)
	adapter.Register(&media.RandomMediaCommand{}, mw...)
	adapter.Register(&media.UploadMediaCommand{}, mw...)
	adapter.Register(&purge.PurgeCommand{}, mw...)
	adapter.Register(&shortlink.ShortlinkCommand{}, mw...)
	adapter.Register(&summarize.Command{}, mw...)
	adapter.Register(&task.TaskCommand{}, mw...)
	adapter.Register(&roll.RollCommand{}, mw...)
	adapter.Register(&ask.AskCommand{}, mw...)
	adapter.Register(&confess.ConfessCommand{}, mw...)
	adapter.Register(&announce.AnnounceCommand{}, mw...)
	adapter.Register(&translate.TranslateOnReaction{}, mw...)
	adapter.Register(&welcome.WelcomeCommand{}, mw...)
}

func defaultMiddleware(log zerolog.Logger) []command.Middleware {
	return []command.Middleware{
		middleware.WithGroupAccessCheck(),
		middleware.WithGuildOnly(),
		middleware.WithUserPermissionCheck(),
		middleware.WithCommandLogger(log),
	}
}
