package storage

import "fmt"

func (s *Storage) SetConfessChannel(guildID, channelID string) error {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return err
	}

	record.ConfessChannel = channelID
	return s.ds.Set(guildID, record)
}

func (s *Storage) GetConfessChannel(guildID string) (string, error) {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return "", err
	}

	if record.ConfessChannel == "" {
		return "", fmt.Errorf("no confession channel set")
	}

	return record.ConfessChannel, nil
}

func (s *Storage) RemoveConfessChannel(guildID string) error {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return err
	}

	record.ConfessChannel = ""
	return s.ds.Set(guildID, record)
}
