package storage

import "fmt"

func (s *Storage) SetAnnounceChannel(guildID, channelID string) error {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return err
	}

	record.AnnounceChannel = channelID
	return s.ds.Set(guildID, record)
}

func (s *Storage) GetAnnounceChannel(guildID string) (string, error) {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return "", err
	}

	if record.AnnounceChannel == "" {
		return "", fmt.Errorf("no announce channel set")
	}

	return record.AnnounceChannel, nil
}

func (s *Storage) RemoveAnnounceChannel(guildID string) error {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return err
	}

	record.AnnounceChannel = ""
	return s.ds.Set(guildID, record)
}
