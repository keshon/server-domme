// cmd/discord/main.go
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"server-domme/internal/config"
	"server-domme/internal/discord"
	"server-domme/internal/storage"
)

func main() {
	log.Println("Starting Server Domme Discord bot...")

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

	// Run the bot in a separate goroutine
	errCh := make(chan error, 1)
	go func() {
		if err := discord.StartBot(ctx, cfg.DiscordToken, storage); err != nil {
			errCh <- err
		}
		close(errCh)
	}()

	// Catch system signals for correct termination
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	select {
	case s := <-sig:
		log.Printf("Received signal %s, shutting down...\n", s)
		cancel()
	case err := <-errCh:
		if err != nil {
			log.Println("Discord bot error:", err)
		}
		cancel()
	case <-ctx.Done():
	}

	log.Println("Discord bot exited cleanly")
}

func startCooldownCleaner(storage *storage.Storage) error {
	ticker := time.NewTicker(1 * time.Minute)
	go func() {
		for range ticker.C {
			err := storage.ClearExpiredCooldowns()
			if err != nil {
				log.Println("Error clearing expired cooldowns:", err)
			}
		}
	}()

	return nil
}
