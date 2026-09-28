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
	taskcmd "github.com/keshon/server-domme/internal/command/task"
	"github.com/keshon/server-domme/internal/config"
	"github.com/keshon/server-domme/internal/discord"
	"github.com/keshon/server-domme/internal/discord/command/catalog"
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
	checkConn := flag.Bool("check", false, "connect, report what the gateway sees, and exit without registering or sending anything")
	flag.Parse()
	if *genReadme {
		log := zerolog.New(zerolog.NewConsoleWriter()).With().Timestamp().Logger()
		catalog.Register(log)
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

	if *checkConn {
		runConnectionCheck(rootCtx, cfg, log)
		return
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

	catalog.Register(log)

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

	// TODO(melodix-stack): re-enable once purge is ported to the disgo
	// session API. The scheduler replays stored jobs against the gateway, so
	// it waits on bot.Ready() before its first use and resolves the live
	// connection per purge.

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
