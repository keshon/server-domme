// cmd/discord/main.go
package main

import (
	"context"
	"math/rand/v2"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/keshon/buildinfo"
	"github.com/keshon/command"
	"github.com/keshon/server-domme/internal/applog"
	"github.com/keshon/server-domme/internal/command/announce"
	"github.com/keshon/server-domme/internal/command/ask"
	"github.com/keshon/server-domme/internal/command/confess"
	"github.com/keshon/server-domme/internal/command/core/about"
	"github.com/keshon/server-domme/internal/command/core/help"
	"github.com/keshon/server-domme/internal/command/core/maintenance"
	"github.com/keshon/server-domme/internal/command/discipline"
	"github.com/keshon/server-domme/internal/command/media"
	"github.com/keshon/server-domme/internal/command/purge"
	"github.com/keshon/server-domme/internal/command/roll"
	"github.com/keshon/server-domme/internal/command/settings"
	"github.com/keshon/server-domme/internal/command/shortlink"
	taskcmd "github.com/keshon/server-domme/internal/command/task"
	"github.com/keshon/server-domme/internal/command/translate"
	"github.com/keshon/server-domme/internal/config"
	"github.com/keshon/server-domme/internal/discord"
	"github.com/keshon/server-domme/internal/discord/cmdadapter"
	"github.com/keshon/server-domme/internal/middleware"
	"github.com/keshon/server-domme/internal/storage"
	"github.com/rs/zerolog"
)

func main() {
	info := buildinfo.Get()

	// Root context cancels on SIGINT/SIGTERM.
	rootCtx, stopSignal := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stopSignal()

	cfg, err := config.NewConfig()
	if err != nil {
		_, _ = os.Stderr.WriteString("failed to load config: " + err.Error() + "\n")
		os.Exit(1)
	}

	log := applog.Setup("discord", cfg)
	log.Info().Str("project", info.Project).Msg("bot_starting")

	if cfg.DiscordToken == "" {
		log.Fatal().Msg("config_missing_token")
	}

	store, err := storage.NewStorage(rootCtx, cfg.StoragePath, log)
	if err != nil {
		log.Fatal().Err(err).Msg("storage_init_failed")
	}

	if err := taskcmd.InitFromConfig(cfg); err != nil {
		log.Fatal().Err(err).Msg("task_init_failed")
	}
	log.Println("[INFO] Tasks initialized")
	go storage.RunCooldownCleaner(rootCtx, store)
	log.Println("[INFO] Cooldown cleaner started")

	bot := discord.NewBot(cfg, store, log)

	registerCommands(log)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			var lastErr error
			if err := bot.RunSession(rootCtx); err != nil {
				lastErr = err
				log.Error().Err(err).Msg("discord_session_end")
			}

			select {
			case <-rootCtx.Done():
				return
			default:
				delay := 5 * time.Second
				if discord.IsSessionUnhealthyError(lastErr) {
					delay = time.Duration(rand.IntN(200)) * time.Millisecond
				}
				log.Warn().Dur("delay", delay).Msg("discord_session_restart")
				timer := time.NewTimer(delay)
				select {
				case <-rootCtx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
			}
		}
	}()

	<-rootCtx.Done()
	log.Info().Msg("shutdown_signal_received")

	wg.Wait()

	closeCtx, cancelClose := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelClose()
	if err := store.Close(closeCtx); err != nil {
		log.Error().Err(err).Msg("storage_close_failed")
	}

	log.Info().Msg("bot_exit")
}

func defaultMiddleware(log zerolog.Logger) []command.Middleware {
	return []command.Middleware{
		middleware.WithGroupAccessCheck(),
		middleware.WithGuildOnly(),
		middleware.WithUserPermissionCheck(),
		middleware.WithCommandLogger(log),
	}
}

func registerCommands(log zerolog.Logger) {
	mw := defaultMiddleware(log)
	cmdadapter.Register(&about.About{}, mw...)
	cmdadapter.Register(&help.Help{}, mw...)
	cmdadapter.Register(&settings.SettingsCommand{}, mw...)
	cmdadapter.Register(&maintenance.Maintenance{}, mw...)

	cmdadapter.Register(&announce.AnnounceCommand{}, mw...)
	cmdadapter.Register(&announce.AnnounceContextCommand{}, mw...)

	cmdadapter.Register(&ask.AskCommand{}, mw...)

	cmdadapter.Register(&confess.ConfessCommand{}, mw...)

	cmdadapter.Register(&discipline.DisciplineCommand{}, mw...)

	cmdadapter.Register(&media.RandomMediaCommand{}, mw...)
	cmdadapter.Register(&media.UploadMediaCommand{}, mw...)

	cmdadapter.Register(&purge.PurgeCommand{}, mw...)
	cmdadapter.Register(&roll.RollCommand{}, mw...)
	cmdadapter.Register(&shortlink.ShortlinkCommand{}, mw...)

	cmdadapter.Register(&taskcmd.TaskCommand{}, mw...)

	cmdadapter.Register(&translate.TranslateOnReaction{}, mw...)
}
