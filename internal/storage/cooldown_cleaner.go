package storage

import (
	"context"
	"log"
	"time"
)

// RunCooldownCleaner runs a background goroutine that clears expired task cooldowns
// every minute until ctx is done. Call from main or app lifecycle.
func RunCooldownCleaner(ctx context.Context, store *Storage) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := store.ClearExpiredCooldowns(); err != nil {
				log.Println("[ERR] Error clearing expired cooldowns:", err)
			}
		}
	}
}
