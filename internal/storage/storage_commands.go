package storage

import st "server-domme/internal/storagetypes"

func (s *Storage) DisableGroup(guildID, group string) error {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return err
	}

	for _, g := range record.CommandsDisabled {
		if g == group {
			return nil
		}
	}

	record.CommandsDisabled = append(record.CommandsDisabled, group)
	s.ds.Add(guildID, record)
	return nil
}

func (s *Storage) EnableGroup(guildID, group string) error {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return err
	}

	updated := make([]string, 0, len(record.CommandsDisabled))
	for _, g := range record.CommandsDisabled {
		if g != group {
			updated = append(updated, g)
		}
	}
	record.CommandsDisabled = updated
	s.ds.Add(guildID, record)
	return nil
}

func (s *Storage) IsGroupDisabled(guildID, group string) (bool, error) {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return false, err
	}
	for _, g := range record.CommandsDisabled {
		if g == group {
			return true, nil
		}
	}
	return false, nil
}

func (s *Storage) GetDisabledGroups(guildID string) ([]string, error) {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return nil, err
	}
	return record.CommandsDisabled, nil
}

func (s *Storage) GetCommandsHistory(guildID string) ([]st.CommandHistory, error) {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return nil, err
	}

	return record.CommandsHistory, nil
}
