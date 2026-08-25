package storage

import (
	"time"
)

// SetDeletionJob records (or replaces) the purge job for one channel.
func (s *Storage) SetDeletionJob(guildID, channelID, mode string, delayUntil time.Time, silent bool, olderThan ...string) error {
	job := &PurgeJob{
		ChannelID:  channelID,
		GuildID:    guildID,
		Mode:       mode,
		DelayUntil: delayUntil,
		Silent:     silent,
		StartedAt:  time.Now(),
	}
	if len(olderThan) > 0 {
		job.OlderThan = olderThan[0]
	}
	return s.purgeJobs.Put(job)
}

// ClearDeletionJob removes a channel's purge job (idempotent).
func (s *Storage) ClearDeletionJob(guildID, channelID string) error {
	return s.purgeJobs.Delete(guildScopedKey(guildID, channelID))
}

// GetDeletionJobsList returns the guild's purge jobs keyed by channel id.
func (s *Storage) GetDeletionJobsList(guildID string) (map[string]PurgeJob, error) {
	rows := s.purgeJobsByGuild.Find(guildID)
	jobs := make(map[string]PurgeJob, len(rows))
	for _, j := range rows {
		jobs[j.ChannelID] = *j
	}
	return jobs, nil
}

// GetDeletionJob returns one channel's purge job. A channel with no job yields
// the zero job and no error, which is what callers test with job.Mode == "".
func (s *Storage) GetDeletionJob(guildID, channelID string) (PurgeJob, error) {
	job, ok := s.purgeJobs.Get(guildScopedKey(guildID, channelID))
	if !ok {
		return PurgeJob{}, nil
	}
	return *job, nil
}

// AllPurgeJobs returns every stored purge job across all guilds. The scheduler
// uses it once at startup to replay what was pending when the bot last stopped.
func (s *Storage) AllPurgeJobs() []PurgeJob {
	var jobs []PurgeJob
	for j := range s.purgeJobs.All() {
		jobs = append(jobs, *j)
	}
	return jobs
}
