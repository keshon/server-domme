package storage

func (s *Storage) CreateMediaCategory(guildID string, categoryID string) error {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return err
	}

	record.MediaCategories = append(record.MediaCategories, categoryID)
	s.ds.Add(guildID, record)
	return nil
}

func (s *Storage) RemoveMediaCategory(guildID string, categoryID string) error {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return err
	}

	for i, category := range record.MediaCategories {
		if category == categoryID {
			record.MediaCategories = append(record.MediaCategories[:i], record.MediaCategories[i+1:]...)
			break
		}
	}
	s.ds.Add(guildID, record)
	return nil
}

func (s *Storage) GetMediaCategories(guildID string) ([]string, error) {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return nil, err
	}
	return record.MediaCategories, nil
}

func (s *Storage) SetMediaDefault(guildID string, categoryID string) error {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return err
	}

	record.MediaDefault = categoryID
	s.ds.Add(guildID, record)
	return nil
}

func (s *Storage) ResetMediaDefault(guildID string) error {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return err
	}

	record.MediaDefault = ""
	s.ds.Add(guildID, record)
	return nil
}

func (s *Storage) GetMediaDefault(guildID string) (string, error) {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return "", err
	}
	return record.MediaDefault, nil
}
