package discord

import (
	"encoding/json"
	"log"
	"server-domme/internal/command"
	"server-domme/internal/storage"
	st "server-domme/internal/storagetypes"
	"time"

	"github.com/bwmarrin/discordgo"
)

func startScheduledPurgeJobs(storage *storage.Storage, session *discordgo.Session) {
	records := storage.GetRecordsList()

	for _, data := range records {
		jsonData, _ := json.Marshal(data)
		var record st.Record
		err := json.Unmarshal(jsonData, &record)
		if err != nil {
			log.Printf("Error unmarshalling to *Record: %v", err)
			continue
		}

		for _, job := range record.DeletionJobs {
			log.Printf("Found nuke job — Mode: %s | Guild: %s | Channel: %s", job.Mode, job.GuildID, job.ChannelID)

			switch job.Mode {
			case "delayed":
				dur := time.Until(job.DelayUntil)

				if dur <= 0 {
					log.Printf("DelayUntil is in the past — executing delayed nuke immediately for channel %s", job.ChannelID)
					command.DeleteMessages(session, job.ChannelID, nil, nil, nil)

					err := storage.ClearDeletionJob(job.GuildID, job.ChannelID)
					if err != nil {
						log.Printf("Failed to delete nuke job for channel %s: %v", job.ChannelID, err)
					}
				} else {
					log.Printf("Scheduling delayed nuke in %v for channel %s", dur, job.ChannelID)
					go func(job st.DeletionJob) {
						time.Sleep(dur)
						log.Printf("Executing delayed nuke for channel %s", job.ChannelID)
						command.DeleteMessages(session, job.ChannelID, nil, nil, nil)

						err := storage.ClearDeletionJob(job.GuildID, job.ChannelID)
						if err != nil {
							log.Printf("Failed to delete nuke job for channel %s: %v", job.ChannelID, err)
						} else {
							log.Printf("Delayed nuke complete and removed for channel %s", job.ChannelID)
						}
					}(job)
				}

			case "recurring":
				dur, err := time.ParseDuration(job.OlderThan)
				if err != nil {
					log.Printf("Failed to parse OlderThan duration '%s' for channel %s: %v", job.OlderThan, job.ChannelID, err)
					continue
				}

				stopChan := make(chan struct{})
				command.ActiveDeletionsMu.Lock()
				command.ActiveDeletions[job.ChannelID] = stopChan
				command.ActiveDeletionsMu.Unlock()

				log.Printf("Starting recurring nuke for channel %s every 30s (older than %v)", job.ChannelID, dur)

				go func(job st.DeletionJob, d time.Duration) {
					ticker := time.NewTicker(30 * time.Second)
					defer ticker.Stop()

					for {
						select {
						case <-stopChan:
							log.Printf("Stopping recurring nuke for channel %s", job.ChannelID)
							return
						case <-ticker.C:
							start := time.Now().Add(-d)
							now := time.Now()
							log.Printf("Recurring nuke triggered for channel %s", job.ChannelID)
							command.DeleteMessages(session, job.ChannelID, &start, &now, stopChan)
						}
					}
				}(job, dur)

			default:
				log.Printf("Unknown nuke mode '%s' for channel %s", job.Mode, job.ChannelID)
			}
		}
	}
}
