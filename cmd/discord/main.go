// cmd/discord/main.go
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"server-domme/internal/config"
	"server-domme/internal/discord"
	"server-domme/internal/storage"

	"go.uber.org/zap"
)

func main() {
	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatal(err)
	}
	defer logger.Sync()

	logger.Info("Starting Server Domme Discord bot...")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := config.New()

	store, err := storage.New(cfg.StoragePath)
	if err != nil {
		log.Fatal(err)
	}

	// Run the bot in a separate goroutine
	errCh := make(chan error, 1)
	go func() {
		if err := discord.StartBot(ctx, cfg.DiscordToken, store, logger); err != nil {
			errCh <- err
		}
		close(errCh)
	}()

	// Catch system signals for correct termination
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	select {
	case s := <-sig:
		fmt.Printf("Received signal %s, shutting down...\n", s)
		cancel()
	case err := <-errCh:
		if err != nil {
			fmt.Println("Discord bot error:", err)
		}
		cancel()
	case <-ctx.Done():
	}

	fmt.Println("Discord bot exited cleanly")
}
