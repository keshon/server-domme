package storage

import (
	"fmt"
	"time"

	st "server-domme/internal/storagetypes"
)

func (s *Storage) AddShortLink(guildID, userID, original, shortID string) error {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return fmt.Errorf("failed to load guild record: %w", err)
	}

	newLink := st.ShortLink{
		ShortID:  shortID,
		Original: original,
		UserID:   userID,
		Created:  time.Now(),
	}

	record.ShortLinks = append(record.ShortLinks, newLink)
	s.ds.Add(guildID, record)
	return nil
}

func (s *Storage) GetUserShortLinks(guildID, userID string) ([]st.ShortLink, error) {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return nil, fmt.Errorf("failed to load guild record: %w", err)
	}

	var userLinks []st.ShortLink
	for _, link := range record.ShortLinks {
		if link.UserID == userID {
			userLinks = append(userLinks, link)
		}
	}
	return userLinks, nil
}

// ClearUserShortLinks deletes all short links belonging to a specific user.
func (s *Storage) ClearUserShortLinks(guildID, userID string) error {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return fmt.Errorf("failed to load guild record: %w", err)
	}

	filtered := make([]st.ShortLink, 0, len(record.ShortLinks))
	for _, link := range record.ShortLinks {
		if link.UserID != userID {
			filtered = append(filtered, link)
		}
	}

	record.ShortLinks = filtered
	s.ds.Add(guildID, record)
	return nil
}

// DeleteShortLink removes a single short link by its shortID for the specified user.
func (s *Storage) DeleteShortLink(guildID, userID, shortID string) error {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return fmt.Errorf("failed to load guild record: %w", err)
	}

	found := false
	filtered := make([]st.ShortLink, 0, len(record.ShortLinks))
	for _, link := range record.ShortLinks {
		if link.UserID == userID && link.ShortID == shortID {
			found = true
			continue // skip this one (delete it)
		}
		filtered = append(filtered, link)
	}

	if !found {
		return fmt.Errorf("short link with ID '%s' not found", shortID)
	}

	record.ShortLinks = filtered
	s.ds.Add(guildID, record)
	return nil
}
