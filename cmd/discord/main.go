// cmd/discord/main.go
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	_ "server-domme/internal/command/announce"
	_ "server-domme/internal/command/ask"
	_ "server-domme/internal/command/chat"
	_ "server-domme/internal/command/confess"
	_ "server-domme/internal/command/core"
	_ "server-domme/internal/command/discipline"
	_ "server-domme/internal/command/media"
	_ "server-domme/internal/command/purge"
	_ "server-domme/internal/command/roll"
	_ "server-domme/internal/command/shortlink"
	_ "server-domme/internal/command/task"
	_ "server-domme/internal/command/translate"

	"server-domme/internal/command"
	"server-domme/internal/command/music"
	"server-domme/internal/command/task"
	"server-domme/internal/config"
	"server-domme/internal/discord"
	"server-domme/internal/middleware"
	"server-domme/internal/storage"
	v "server-domme/internal/version"
)

func main() {
	log.Printf("[INFO] Starting %v bot...", v.AppName)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := config.New()

	store, err := storage.New(cfg.StoragePath)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()

	if err := task.InitFromConfig(cfg); err != nil {
		log.Fatal(err)
	}

	go storage.RunCooldownCleaner(ctx, store)

	bot := discord.NewBot(cfg, store)
	command.RegisterCommand(
		&music.MusicCommand{Bot: bot},
		middleware.WithGroupAccessCheck(),
		middleware.WithGuildOnly(),
		middleware.WithUserPermissionCheck(),
		middleware.WithCommandLogger(),
	)

	errCh := make(chan error, 1)
	go func() {
		if err := bot.Run(ctx); err != nil {
			errCh <- err
		}
		close(errCh)
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	select {
	case s := <-sig:
		log.Printf("[INFO] Received signal %s, shutting down...\n", s)
		cancel()
	case err := <-errCh:
		if err != nil {
			log.Println("[ERR] Discord bot error:", err)
		}
		cancel()
	case <-ctx.Done():
	}

	log.Println("[INFO] Discord bot exited cleanly")
}
