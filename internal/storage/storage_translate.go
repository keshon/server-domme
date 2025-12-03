package storage

import (
	"fmt"
)

func (s *Storage) AddTranslateChannel(guildID string, channelID string) error {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return err
	}

	if record.TranslateChannels == nil {
		record.TranslateChannels = []string{}
	}

	// Check if channel already exists
	for _, ch := range record.TranslateChannels {
		if ch == channelID {
			return fmt.Errorf("channel already in translate list")
		}
	}

	record.TranslateChannels = append(record.TranslateChannels, channelID)
	s.ds.Add(guildID, record)
	return nil
}

func (s *Storage) RemoveTranslateChannel(guildID string, channelID string) error {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return err
	}

	if len(record.TranslateChannels) == 0 {
		return fmt.Errorf("no translate channels configured")
	}

	newList := []string{}
	found := false
	for _, ch := range record.TranslateChannels {
		if ch != channelID {
			newList = append(newList, ch)
		} else {
			found = true
		}
	}

	if !found {
		return fmt.Errorf("channel not found in translate list")
	}

	record.TranslateChannels = newList
	s.ds.Add(guildID, record)
	return nil
}

func (s *Storage) GetTranslateChannels(guildID string) ([]string, error) {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return nil, err
	}

	if record.TranslateChannels == nil {
		return []string{}, nil
	}

	return record.TranslateChannels, nil
}

func (s *Storage) ResetTranslateChannels(guildID string) error {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return err
	}

	record.TranslateChannels = []string{}
	s.ds.Add(guildID, record)
	return nil
}
