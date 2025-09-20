// cmd/discord/main.go
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "server-domme/internal/commands" // import all commands to register
	"server-domme/internal/config"
	"server-domme/internal/discord"
	"server-domme/internal/storage"
	v "server-domme/internal/version"
)

func main() {
	log.Printf("[INFO] Starting %v bot...", v.AppName)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := config.New()

	storage, err := storage.New(cfg.StoragePath)
	if err != nil {
		log.Fatal(err)
	}
	defer storage.Close()

	err = startCooldownCleaner(storage)
	if err != nil {
		log.Fatal(err)
	}

	errCh := make(chan error, 1)
	go func() {
		if err := discord.StartBot(ctx, cfg.DiscordToken, storage); err != nil {
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

func startCooldownCleaner(storage *storage.Storage) error {
	ticker := time.NewTicker(1 * time.Minute)
	go func() {
		for range ticker.C {
			err := storage.ClearExpiredCooldowns()
			if err != nil {
				log.Println("[ERR] Error clearing expired cooldowns:", err)
			}
		}
	}()

	return nil
}
