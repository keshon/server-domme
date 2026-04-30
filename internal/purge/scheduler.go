package purge

import (
	"context"
	"encoding/json"
	"log"

	"time"

	"github.com/keshon/server-domme/internal/command/purge"
	st "github.com/keshon/server-domme/internal/domain"
	"github.com/keshon/server-domme/internal/storage"

	"github.com/bwmarrin/discordgo"
)

// RunScheduler starts scheduled purge jobs (delayed and recurring). Call from the Discord lifecycle.
func RunScheduler(ctx context.Context, store *storage.Storage, session *discordgo.Session) {
	log.Printf("[INFO] Starting purge scheduler...")
	records := store.Records()

	for _, data := range records {
		jsonData, _ := json.Marshal(data)
		var record st.Record
		err := json.Unmarshal(jsonData, &record)
		if err != nil {
			log.Printf("[ERR] Error unmarshalling to *Record: %v", err)
			continue
		}

		for _, job := range record.PurgeJobs {
			log.Printf("[INFO] Found purge job — Mode: %s | Guild: %s | Channel: %s", job.Mode, job.GuildID, job.ChannelID)

			switch job.Mode {
			case "delayed":
				dur := time.Until(job.DelayUntil)

				if dur <= 0 {
					log.Printf("[INFO] DelayUntil is in the past — executing delayed purge immediately for channel %s", job.ChannelID)
					purge.DeleteMessages(session, job.ChannelID, nil, nil, nil)

					err := store.ClearDeletionJob(job.GuildID, job.ChannelID)
					if err != nil {
						log.Printf("[ERR] Failed to delete purge job for channel %s: %v", job.ChannelID, err)
					}
				} else {
					log.Printf("[INFO] Scheduling delayed purge in %v for channel %s", dur, job.ChannelID)
					go func(job st.PurgeJob) {
						timer := time.NewTimer(dur)
						defer timer.Stop()
						select {
						case <-ctx.Done():
							return
						case <-timer.C:
						}
						log.Printf("[INFO] Executing delayed purge for channel %s", job.ChannelID)
						purge.DeleteMessages(session, job.ChannelID, nil, nil, nil)

						err := store.ClearDeletionJob(job.GuildID, job.ChannelID)
						if err != nil {
							log.Printf("[ERR] Failed to delete purge job for channel %s: %v", job.ChannelID, err)
						} else {
							log.Printf("[INFO] Delayed purge complete and removed for channel %s", job.ChannelID)
						}
					}(job)
				}

			case "recurring":
				dur, err := time.ParseDuration(job.OlderThan)
				if err != nil {
					log.Printf("[ERR] Failed to parse OlderThan duration '%s' for channel %s: %v", job.OlderThan, job.ChannelID, err)
					continue
				}

				stopChan := make(chan struct{})
				purge.ActiveDeletionsMu.Lock()
				purge.ActiveDeletions[job.ChannelID] = stopChan
				purge.ActiveDeletionsMu.Unlock()

				log.Printf("[INFO] Starting recurring purge for channel %s every 30s (older than %v)", job.ChannelID, dur)

				go func(job st.PurgeJob, d time.Duration) {
					ticker := time.NewTicker(30 * time.Second)
					defer ticker.Stop()

					for {
						select {
						case <-stopChan:
							log.Printf("[INFO] Stopping recurring purge for channel %s", job.ChannelID)
							return
						case <-ctx.Done():
							log.Printf("[INFO] Stopping recurring purge for channel %s (shutdown)", job.ChannelID)
							return
						case <-ticker.C:
							start := time.Now().Add(-d)
							now := time.Now()
							log.Printf("[INFO] Recurring purge triggered for channel %s", job.ChannelID)
							purge.DeleteMessages(session, job.ChannelID, &start, &now, stopChan)
						}
					}
				}(job, dur)

			default:
				log.Printf("[ERR] Unknown purge mode '%s' for channel %s", job.Mode, job.ChannelID)
			}
		}
	}
}
