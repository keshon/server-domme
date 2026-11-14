// cmd/discord/main.go — Discord server-management bot.
package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/keshon/command"
	"github.com/keshon/server-domme/internal/applog"
	"github.com/keshon/server-domme/internal/config"
	"github.com/keshon/server-domme/internal/discord"
	"github.com/keshon/server-domme/internal/discord/command/catalog"
	taskcmd "github.com/keshon/server-domme/internal/discord/command/task"
	"github.com/keshon/server-domme/internal/discord/session"
	purgesvc "github.com/keshon/server-domme/internal/purge"
	"github.com/keshon/server-domme/internal/readme"
	shortlinksvc "github.com/keshon/server-domme/internal/shortlink"
	"github.com/keshon/server-domme/internal/storage"
	"github.com/rs/zerolog"
)

func main() {
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
	log.Info().Str("project", "Server Domme").Msg("bot_starting")

	if cfg.DiscordToken == "" {
		log.Fatal().Msg("config_missing_token")
	}

	store, err := storage.NewStorage(cfg.StoragePath, log)
	if err != nil {
		log.Fatal().Err(err).Str("dir", cfg.StoragePath).Msg("storage_init_failed")
	}

	recovery, err := session.NewRecovery(rootCtx, cfg.StoragePath, cfg.DiscordToken, cfg.DiscordSessionBudget, log)
	if err != nil {
		log.Error().Err(err).Msg("discord_recovery_init_failed")
		_ = store.Close()
		return
	}
	if *checkConn {
		runConnectionCheck(rootCtx, cfg, log, recovery)
		_ = store.Close()
		return
	}

	if err := taskcmd.InitFromConfig(cfg, log); err != nil {
		log.Fatal().Err(err).Msg("task_init_failed")
	}
	log.Info().Str("path", cfg.TasksPath).Msg("tasks_initialized")

	bot := discord.NewBot(cfg, store, log)
	bot.SetRecovery(recovery)

	catalog.Register(log)

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		runSessionLoop(rootCtx, bot, log, recovery)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		storage.RunCooldownCleaner(rootCtx, store, log)
	}()

	// The purge scheduler replays stored jobs against the gateway, so it cannot
	// run before the first connect. It resolves the live connection per purge
	// and therefore survives every later reconnect.
	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-rootCtx.Done():
			return
		case <-bot.Ready():
		}
		purgesvc.RunScheduler(rootCtx, store, bot.SessionAPI, log)
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

// runSessionLoop keeps one Discord session alive within the persistent budget.
func runSessionLoop(ctx context.Context, bot *discord.Bot, log zerolog.Logger, recovery *session.Recovery) {
	var backoff restartBackoff
	for {
		var lastErr error
		started := time.Now()
		err := recovery.WaitUntilEligible(ctx)
		if err == nil {
			started = time.Now()
			err = bot.RunSession(ctx)
		}
		if err != nil {
			lastErr = err
			log.Error().Err(err).Msg("discord_session_end")
		}
		ranFor := time.Since(started)
		if session.IsTerminal(lastErr) || recovery.Fatal() != nil {
			log.Error().Err(lastErr).AnErr("budget_error", recovery.Fatal()).Msg("discord_recovery_stopped_action_required")
			// Stay stopped under restart:always; only a deliberate restart
			// after the configuration/token is corrected should try again.
			<-ctx.Done()
			return
		}

		select {
		case <-ctx.Done():
			return
		default:
			delay := backoff.next(ranFor, discord.IsSessionUnhealthyError(lastErr))
			log.Warn().
				Dur("delay", delay).
				Dur("ran_for", ranFor).
				Int("consecutive_failures", backoff.failures).
				Msg("discord_session_restart")
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
