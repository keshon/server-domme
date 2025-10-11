package storage

import "fmt"

func (s *Storage) SetConfessChannel(guildID, channelID string) error {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return err
	}

	record.ConfessChannel = channelID
	s.ds.Add(guildID, record)
	return nil
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
	s.ds.Add(guildID, record)
	return nil
}
