package discord

import (
	"sync"
	"time"
)

func (b *Bot) makeSessionUnhealthyNotifier(disconnected chan struct{}) func() {
	var restartOnce sync.Once
	var unhealthyMu sync.Mutex
	var unhealthyCount int
	var unhealthyWindowStart time.Time

	return func() {
		mode := b.cfg.DiscordUnhealthyMode
		switch mode {
		case "ignore":
			return
		case "restart-session", "":
		default:
			b.log.Warn().Str("mode", mode).Msg("discord_unhealthy_mode_unknown")
		}

		grace := b.cfg.DiscordUnhealthyGrace
		if grace < 0 {
			grace = 0
		}
		window := b.cfg.DiscordUnhealthyWindow
		if window <= 0 {
			window = time.Minute
		}

		shouldRestart := true
		if grace > 0 {
			now := time.Now()
			unhealthyMu.Lock()
			if unhealthyWindowStart.IsZero() || now.Sub(unhealthyWindowStart) > window {
				unhealthyWindowStart = now
				unhealthyCount = 0
			}
			unhealthyCount++
			if unhealthyCount <= grace {
				shouldRestart = false
			}
			unhealthyMu.Unlock()
		}

		if !shouldRestart {
			return
		}

		restartOnce.Do(func() {
			b.log.Warn().Msg("discord_session_unhealthy")
			close(disconnected)
		})
	}
}

// graceCanEscalate reports whether DISCORD_UNHEALTHY_GRACE can ever be
// exceeded. The silence watcher repeats its signal on the first tick after each
// timeout, and the count below resets when a signal lands outside the window,
// so the window has to hold grace+1 signals spaced that far apart.
func graceCanEscalate(grace int, window, timeout, tick time.Duration) bool {
	if grace <= 0 || timeout <= 0 {
		return true
	}
	return time.Duration(grace)*(timeout+tick) <= window
}
