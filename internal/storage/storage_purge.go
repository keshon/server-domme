package storage

import (
	st "server-domme/internal/storagetypes"
	"time"
)

func (s *Storage) SetDeletionJob(guildID, channelID, mode string, delayUntil time.Time, silent bool, olderThan ...string) error {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return err
	}

	job := st.DeletionJob{
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
	s.ds.Add(guildID, record)
	return nil
}

func (s *Storage) ClearDeletionJob(guildID, channelID string) error {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return err
	}
	delete(record.PurgeJobs, channelID)
	s.ds.Add(guildID, record)
	return nil
}

func (s *Storage) GetDeletionJobsList(guildID string) (map[string]st.DeletionJob, error) {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return nil, err
	}
	return record.PurgeJobs, nil
}

func (s *Storage) GetDeletionJob(guildID, channelID string) (st.DeletionJob, error) {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return st.DeletionJob{}, err
	}
	return record.PurgeJobs[channelID], nil
}
