package storage

import "fmt"

func (s *Storage) SetAnnounceChannel(guildID, channelID string) error {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return err
	}

	record.AnnounceChannel = channelID
	s.ds.Add(guildID, record)
	return nil
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
	s.ds.Add(guildID, record)
	return nil
}
