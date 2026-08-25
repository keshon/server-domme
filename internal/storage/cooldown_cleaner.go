package storage

import (
	"context"
	"time"

	"github.com/rs/zerolog"
)

// cooldownSweepInterval is how often expired task cooldowns are swept. Cooldowns
// are compared against wall-clock time on read, so this only controls how long
// dead entries linger in the store, never whether a cooldown is enforced.
const cooldownSweepInterval = time.Minute

// RunCooldownCleaner clears expired task cooldowns until ctx is done. It blocks,
// so run it in a goroutine.
func RunCooldownCleaner(ctx context.Context, store *Storage, log zerolog.Logger) {
	ticker := time.NewTicker(cooldownSweepInterval)
	defer ticker.Stop()

	log.Info().Dur("interval", cooldownSweepInterval).Msg("cooldown_cleaner_started")
	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("cooldown_cleaner_stopped")
			return
		case <-ticker.C:
			if err := store.ClearExpiredCooldowns(); err != nil {
				log.Error().Err(err).Msg("cooldown_clear_failed")
			}
		}
	}
}
