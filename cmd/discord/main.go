// cmd/discord/main.go
package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"
	"sync"
	"time"

	"server-domme/internal/command"
	"server-domme/internal/command/announce"
	"server-domme/internal/command/ask"
	"server-domme/internal/command/confess"
	"server-domme/internal/command/core/about"
	"server-domme/internal/command/core/commands"
	"server-domme/internal/command/core/help"
	"server-domme/internal/command/core/maintenance"
	"server-domme/internal/command/discipline"
	"server-domme/internal/command/media"
	"server-domme/internal/command/music"
	"server-domme/internal/command/purge"
	"server-domme/internal/command/roll"
	"server-domme/internal/command/shortlink"
	taskcmd "server-domme/internal/command/task"
	"server-domme/internal/command/translate"
	"server-domme/internal/config"
	"server-domme/internal/discord"
	"server-domme/internal/middleware"
	"server-domme/internal/storage"

	"github.com/keshon/buildinfo"
)

func registerCommands(bot *discord.Bot) {
	command.Register(
		&about.AboutCommand{},
		middleware.WithGroupAccessCheck(),
		middleware.WithGuildOnly(),
		middleware.WithUserPermissionCheck(),
		middleware.WithCommandLogger(),
	)
	command.Register(
		&help.HelpUnifiedCommand{},
		middleware.WithGroupAccessCheck(),
		middleware.WithGuildOnly(),
		middleware.WithUserPermissionCheck(),
		middleware.WithCommandLogger(),
	)
	command.Register(
		&commands.CommandsCommand{},
		middleware.WithGroupAccessCheck(),
		middleware.WithGuildOnly(),
		middleware.WithUserPermissionCheck(),
		middleware.WithCommandLogger(),
	)
	command.Register(
		&maintenance.MaintenanceCommand{},
		middleware.WithGroupAccessCheck(),
		middleware.WithGuildOnly(),
		middleware.WithUserPermissionCheck(),
		middleware.WithCommandLogger(),
	)

	command.Register(
		&announce.AnnounceCommand{},
		middleware.WithGroupAccessCheck(),
		middleware.WithGuildOnly(),
		middleware.WithUserPermissionCheck(),
		middleware.WithCommandLogger(),
	)
	command.Register(
		&announce.ManageAnnounceCommand{},
		middleware.WithGroupAccessCheck(),
		middleware.WithGuildOnly(),
		middleware.WithUserPermissionCheck(),
		middleware.WithCommandLogger(),
	)
	command.Register(
		&announce.AnnounceContextCommand{},
		middleware.WithGroupAccessCheck(),
		middleware.WithGuildOnly(),
		middleware.WithUserPermissionCheck(),
		middleware.WithCommandLogger(),
	)

	command.Register(
		&ask.AskCommand{},
		middleware.WithGroupAccessCheck(),
		middleware.WithGuildOnly(),
		middleware.WithUserPermissionCheck(),
		middleware.WithCommandLogger(),
	)

	command.Register(
		&confess.ConfessCommand{},
		middleware.WithGroupAccessCheck(),
		middleware.WithGuildOnly(),
		middleware.WithUserPermissionCheck(),
		middleware.WithCommandLogger(),
	)
	command.Register(
		&confess.ManageConfessCommand{},
		middleware.WithGroupAccessCheck(),
		middleware.WithGuildOnly(),
		middleware.WithUserPermissionCheck(),
		middleware.WithCommandLogger(),
	)

	command.Register(
		&discipline.DisciplineCommand{},
		middleware.WithGroupAccessCheck(),
		middleware.WithGuildOnly(),
		middleware.WithUserPermissionCheck(),
		middleware.WithCommandLogger(),
	)
	command.Register(
		&discipline.ManageDisciplineCommand{},
		middleware.WithGroupAccessCheck(),
		middleware.WithGuildOnly(),
		middleware.WithUserPermissionCheck(),
		middleware.WithCommandLogger(),
	)

	command.Register(
		&media.RandomMediaCommand{},
		middleware.WithGroupAccessCheck(),
		middleware.WithGuildOnly(),
		middleware.WithUserPermissionCheck(),
		middleware.WithCommandLogger(),
	)
	command.Register(
		&media.UploadMediaCommand{},
		middleware.WithGroupAccessCheck(),
		middleware.WithGuildOnly(),
		middleware.WithUserPermissionCheck(),
		middleware.WithCommandLogger(),
	)
	command.Register(
		&media.ManageMediaCommand{},
		middleware.WithGroupAccessCheck(),
		middleware.WithGuildOnly(),
		middleware.WithUserPermissionCheck(),
		middleware.WithCommandLogger(),
	)

	command.Register(
		&purge.PurgeCommand{},
		middleware.WithGroupAccessCheck(),
		middleware.WithGuildOnly(),
		middleware.WithUserPermissionCheck(),
		middleware.WithCommandLogger(),
	)
	command.Register(
		&roll.RollCommand{},
		middleware.WithGroupAccessCheck(),
		middleware.WithGuildOnly(),
		middleware.WithUserPermissionCheck(),
		middleware.WithCommandLogger(),
	)
	command.Register(
		&shortlink.ShortlinkCommand{},
		middleware.WithGroupAccessCheck(),
		middleware.WithGuildOnly(),
		middleware.WithUserPermissionCheck(),
		middleware.WithCommandLogger(),
	)

	command.Register(
		&taskcmd.TaskCommand{},
		middleware.WithGroupAccessCheck(),
		middleware.WithGuildOnly(),
		middleware.WithUserPermissionCheck(),
		middleware.WithCommandLogger(),
	)
	command.Register(
		&taskcmd.ManageTaskCommand{},
		middleware.WithGroupAccessCheck(),
		middleware.WithGuildOnly(),
		middleware.WithUserPermissionCheck(),
		middleware.WithCommandLogger(),
	)

	command.Register(
		&translate.ManageTranslateCommand{},
		middleware.WithGroupAccessCheck(),
		middleware.WithGuildOnly(),
		middleware.WithUserPermissionCheck(),
		middleware.WithCommandLogger(),
	)
	command.Register(
		&translate.TranslateOnReaction{},
		middleware.WithGroupAccessCheck(),
		middleware.WithGuildOnly(),
		middleware.WithUserPermissionCheck(),
		middleware.WithCommandLogger(),
	)

	command.Register(
		&music.MusicCommand{Bot: bot},
		middleware.WithGroupAccessCheck(),
		middleware.WithGuildOnly(),
		middleware.WithUserPermissionCheck(),
		middleware.WithCommandLogger(),
	)
}

func main() {
	info := buildinfo.Get()

	log.Printf("[INFO] Starting %v bot...", info.Project)

	// Root context cancels on SIGINT/SIGTERM.
	rootCtx, stopSignal := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stopSignal()

	cfg, err := config.New()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	store, err := storage.New(rootCtx, cfg.StoragePath)
	if err != nil {
		log.Fatal(err)
	}

	if err := taskcmd.InitFromConfig(cfg); err != nil {
		log.Fatal(err)
	}
	log.Println("[INFO] Tasks initialized")
	go storage.RunCooldownCleaner(rootCtx, store)
	log.Println("[INFO] Cooldown cleaner started")

	bot := discord.NewBot(cfg, store)
	registerCommands(bot)

	// Start Discord session with auto-reconnect loop
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			if err := bot.RunSession(rootCtx); err != nil {
				log.Println("[ERR] Discord session ended:", err)
			}

			select {
			case <-rootCtx.Done():
				return
			default:
				log.Println("[WARN] Restarting session in 5s...")
				timer := time.NewTimer(5 * time.Second)
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
	log.Println("[INFO] Shutdown signal received, stopping bot...")

	// Wait for the session loop goroutine to exit.
	wg.Wait()

	// Timebox storage shutdown so Ctrl+C always returns to the shell.
	closeCtx, cancelClose := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelClose()
	if err := store.Close(closeCtx); err != nil {
		log.Printf("[ERR] Storage close error: %v", err)
	}
	log.Println("[INFO] Discord bot exited cleanly")
}
