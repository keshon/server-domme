package storage

import (
	"time"

	st "github.com/keshon/server-domme/internal/domain"
)

func (s *Storage) SetDeletionJob(guildID, channelID, mode string, delayUntil time.Time, silent bool, olderThan ...string) error {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return err
	}

	job := st.PurgeJob{
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

	record.PurgeJobs[channelID] = job
	return s.ds.Set(guildID, record)
}

func (s *Storage) ClearDeletionJob(guildID, channelID string) error {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return err
	}
	delete(record.PurgeJobs, channelID)
	return s.ds.Set(guildID, record)
}

func (s *Storage) GetDeletionJobsList(guildID string) (map[string]st.PurgeJob, error) {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return nil, err
	}
	return record.PurgeJobs, nil
}

func (s *Storage) GetDeletionJob(guildID, channelID string) (st.PurgeJob, error) {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return st.PurgeJob{}, err
	}
	return record.PurgeJobs[channelID], nil
}
