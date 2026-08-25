// cmd/discord/main.go — Discord server-management bot.
package main

import (
	"context"
	"flag"
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
	purgesvc "github.com/keshon/server-domme/internal/purge"
	"github.com/keshon/server-domme/internal/readme"
	shortlinksvc "github.com/keshon/server-domme/internal/shortlink"
	"github.com/keshon/server-domme/internal/storage"
	"github.com/rs/zerolog"
)

func main() {
	info := buildinfo.Get()

	// -readme regenerates README.md from the command registry as a dev step
	// (run from the repo root); the bot never writes files at runtime.
	genReadme := flag.Bool("readme", false, "regenerate README.md from the command registry and exit")
	flag.Parse()
	if *genReadme {
		log := zerolog.New(zerolog.NewConsoleWriter()).With().Timestamp().Logger()
		registerCommands(log)
		if err := readme.UpdateReadme(command.DefaultRegistry, config.CategoryWeights, log); err != nil {
			log.Error().Err(err).Msg("readme_update_failed")
			os.Exit(1)
		}
		return
	}

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

	store, err := storage.NewStorage(cfg.StoragePath, log)
	if err != nil {
		log.Fatal().Err(err).Str("dir", cfg.StoragePath).Msg("storage_init_failed")
	}

	if err := taskcmd.InitFromConfig(cfg, log); err != nil {
		log.Fatal().Err(err).Msg("task_init_failed")
	}
	log.Info().Str("path", cfg.TasksPath).Msg("tasks_initialized")

	bot := discord.NewBot(cfg, store, log)

	registerCommands(log)

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		runSessionLoop(rootCtx, bot, log)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		storage.RunCooldownCleaner(rootCtx, store, log)
	}()

	// The purge scheduler replays stored jobs against the gateway, so it cannot
	// run before the first connect. It resolves the session per purge (see
	// purge.SessionFunc) and therefore survives every later reconnect.
	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-rootCtx.Done():
			return
		case <-bot.Ready():
		}
		purgesvc.RunScheduler(rootCtx, store, bot.Session, log)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := shortlinksvc.RunServer(rootCtx, store, cfg, log); err != nil {
			log.Error().Err(err).Msg("shortlink_server_failed")
		}
	}()

	<-rootCtx.Done()
	log.Info().Msg("shutdown_signal_received")

	wg.Wait()

	if err := store.Close(); err != nil {
		log.Error().Err(err).Msg("storage_close_failed")
	}

	log.Info().Msg("bot_exit")
}

// runSessionLoop keeps one Discord session alive, reconnecting until ctx ends.
// An unhealthy session is retried almost immediately (the connection is known
// bad, so waiting buys nothing); any other failure backs off, and the jitter
// keeps a fleet of bots from reconnecting in lockstep after an outage.
func runSessionLoop(ctx context.Context, bot *discord.Bot, log zerolog.Logger) {
	for {
		var lastErr error
		if err := bot.RunSession(ctx); err != nil {
			lastErr = err
			log.Error().Err(err).Msg("discord_session_end")
		}

		select {
		case <-ctx.Done():
			return
		default:
			delay := 5 * time.Second
			if discord.IsSessionUnhealthyError(lastErr) {
				delay = time.Duration(rand.IntN(200)) * time.Millisecond
			}
			log.Warn().Dur("delay", delay).Msg("discord_session_restart")
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}
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
