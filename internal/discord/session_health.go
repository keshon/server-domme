package discord

import (
	"context"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/keshon/server-domme/internal/discord/watchdog"
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

// lastHeartbeatAck reads the session's last heartbeat ACK under the lock that
// actually guards it.
//
// Do not reach for dg.HeartbeatLatency() here instead: it reads LastHeartbeatAck
// together with LastHeartbeatSent, and upstream discordgo guards those two with
// different locks (the Session mutex and wsMutex), so that accessor is a data
// race. The ack alone is also the better signal — a latency is the last
// *completed* exchange, so on a dead connection it goes stale and then negative
// rather than growing.
func lastHeartbeatAck(dg *discordgo.Session) time.Time {
	dg.RLock()
	defer dg.RUnlock()
	return dg.LastHeartbeatAck
}

func (b *Bot) startSessionHealthWatchers(
	sessionCtx context.Context,
	dg *discordgo.Session,
	tracker *watchdog.Tracker,
	notifyUnhealthy func(),
) {
	go watchdog.NewWSSilence(
		tracker,
		b.cfg.WSSilenceTimeout,
		// No latency source: the only safe accessor upstream offers is the ACK
		// timestamp below, and the watchdog decides on staleness, not latency.
		nil,
		func(meta watchdog.WSSilenceMeta) {
			b.log.Warn().
				Dur("since_last_ws", meta.SinceLastWS).
				Dur("since_last_heartbeat_ack", meta.SinceLastHeartbeatAck).
				Dur("timeout", meta.Timeout).
				Msg("gateway_silent")
			notifyUnhealthy()
		},
		watchdog.WSSilenceOptions{
			SettleDelay: 15 * time.Second,
			Tick:        10 * time.Second,
			LastHeartbeatAck: func() time.Time {
				return lastHeartbeatAck(dg)
			},
		},
	).Run(sessionCtx)

	go func() {
		select {
		case <-sessionCtx.Done():
			return
		case <-time.After(15 * time.Second):
		}

		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		fails := 0

		for {
			select {
			case <-sessionCtx.Done():
				return
			case <-ticker.C:
				ack := lastHeartbeatAck(dg)
				if ack.IsZero() {
					// Connected but not yet ACKed: a probe now would report a
					// failure that says nothing about the session.
					b.log.Debug().Msg("heartbeat_ack_pending")
					continue
				}
				sinceAck := time.Since(ack)
				if _, err := dg.User("@me"); err != nil {
					fails++
					b.log.Warn().Int("fails", fails).Err(err).Msg("api_probe_failed")
					if fails >= 3 {
						b.log.Warn().Int("fails", fails).Msg("api_probe_threshold")
						notifyUnhealthy()
						return
					}
				} else {
					if fails > 0 {
						b.log.Info().Int("fails", fails).Msg("api_probe_recovered")
					}
					fails = 0
					b.log.Debug().Dur("since_last_heartbeat_ack", sinceAck).Msg("heartbeat_ack")
				}
			}
		}
	}()
}
