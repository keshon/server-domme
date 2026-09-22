package purge

import (
	"context"
	"time"

	"github.com/keshon/server-domme/internal/command/purge"
	"github.com/keshon/server-domme/internal/storage"

	"github.com/bwmarrin/discordgo"
	"github.com/rs/zerolog"
)

// SessionFunc yields the gateway session in use right now.
//
// The scheduler's goroutines outlive any single session — RunSession builds a
// fresh one on every restart — so they resolve the session per purge instead of
// closing over one pointer, which would keep writing to a closed connection.
type SessionFunc func() *discordgo.Session

// RunScheduler replays the stored purge jobs (delayed and recurring) and keeps
// the recurring ones ticking until ctx is cancelled. It returns once every job
// has been scheduled; the work itself continues in the background.
func RunScheduler(ctx context.Context, store *storage.Storage, session SessionFunc, log zerolog.Logger) {
	log.Info().Msg("purge_scheduler_starting")

	for _, job := range store.AllPurgeJobs() {
		jobLog := log.With().
			Str("guild_id", job.GuildID).
			Str("channel_id", job.ChannelID).
			Str("mode", job.Mode).
			Logger()

		// Jobs from before the list of channels purges are allowed in keep
		// running: an administrator set each one up deliberately. Their
		// channel goes on the list, and taking it off stops the job.
		if !store.IsPurgeAllowed(job.GuildID, job.ChannelID) {
			if err := store.SetPurgeAllowed(job.GuildID, job.ChannelID, true); err != nil {
				jobLog.Error().Err(err).Msg("purge_channel_allow_failed")
				continue
			}
			jobLog.Info().Msg("purge_channel_grandfathered")
		}

		switch job.Mode {
		case storage.PurgeModeDelayed:
			scheduleDelayed(ctx, store, session, jobLog, job)
		case storage.PurgeModeRecurring:
			scheduleRecurring(ctx, store, session, jobLog, job)
		default:
			jobLog.Error().Msg("purge_job_mode_unknown")
		}
	}
}

func scheduleDelayed(ctx context.Context, store *storage.Storage, session SessionFunc, log zerolog.Logger, job storage.PurgeJob) {
	dur := time.Until(job.DelayUntil)
	if dur <= 0 {
		log.Info().Msg("purge_delayed_overdue")
		runDelayed(store, session, log, job)
		return
	}

	log.Info().Dur("delay", dur).Msg("purge_delayed_scheduled")
	go func() {
		timer := time.NewTimer(dur)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		runDelayed(store, session, log, job)
	}()
}

func runDelayed(store *storage.Storage, session SessionFunc, log zerolog.Logger, job storage.PurgeJob) {
	s := session()
	if s == nil {
		log.Warn().Msg("purge_skipped_no_session")
		return
	}
	if !store.IsPurgeAllowed(job.GuildID, job.ChannelID) {
		log.Info().Msg("purge_skipped_not_allowed")
		return
	}
	log.Info().Msg("purge_delayed_running")
	purge.DeleteMessages(s, job.ChannelID, nil, nil, nil)

	if err := store.ClearDeletionJob(job.GuildID, job.ChannelID); err != nil {
		log.Error().Err(err).Msg("purge_job_clear_failed")
		return
	}
	log.Info().Msg("purge_delayed_done")
}

func scheduleRecurring(ctx context.Context, store *storage.Storage, session SessionFunc, log zerolog.Logger, job storage.PurgeJob) {
	dur, err := time.ParseDuration(job.OlderThan)
	if err != nil {
		log.Error().Str("older_than", job.OlderThan).Err(err).Msg("purge_older_than_invalid")
		return
	}

	stopChan := make(chan struct{})
	purge.ActiveDeletionsMu.Lock()
	purge.ActiveDeletions[job.ChannelID] = stopChan
	purge.ActiveDeletionsMu.Unlock()

	log.Info().Dur("older_than", dur).Dur("interval", recurringInterval).Msg("purge_recurring_started")

	go func() {
		ticker := time.NewTicker(recurringInterval)
		defer ticker.Stop()

		for {
			select {
			case <-stopChan:
				log.Info().Msg("purge_recurring_stopped")
				return
			case <-ctx.Done():
				log.Info().Msg("purge_recurring_stopped_shutdown")
				return
			case <-ticker.C:
				s := session()
				if s == nil {
					log.Warn().Msg("purge_skipped_no_session")
					continue
				}
				if !store.IsPurgeAllowed(job.GuildID, job.ChannelID) {
					log.Info().Msg("purge_recurring_stopped_not_allowed")
					return
				}
				log.Debug().Msg("purge_recurring_tick")
				purge.DeleteOlderThan(s, job.ChannelID, dur, stopChan)
			}
		}
	}()
}

// recurringInterval is how often a recurring job re-checks its channel. It is
// deliberately much shorter than any sensible OlderThan window, so a message
// crossing the age threshold is removed promptly rather than at the next
// window.
const recurringInterval = 30 * time.Second
