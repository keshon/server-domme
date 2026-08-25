package storage

import "slices"

// CreateMediaCategory adds a media category to the guild (idempotent).
func (s *Storage) CreateMediaCategory(guildID string, categoryID string) error {
	g := s.guildSettings(guildID)
	if slices.Contains(g.MediaCategories, categoryID) {
		return nil
	}
	g.MediaCategories = append(g.MediaCategories, categoryID)
	return s.settings.Put(g)
}

// RemoveMediaCategory drops a media category, clearing the guild default when
// that is the category being removed — a default pointing at a category that no
// longer exists would send /media looking in a directory nothing writes to.
func (s *Storage) RemoveMediaCategory(guildID string, categoryID string) error {
	g := s.guildSettings(guildID)
	g.MediaCategories = slices.DeleteFunc(g.MediaCategories, func(c string) bool {
		return c == categoryID
	})
	if g.MediaDefault == categoryID {
		g.MediaDefault = ""
	}
	return s.settings.Put(g)
}

// GetMediaCategories lists the guild's media categories.
func (s *Storage) GetMediaCategories(guildID string) ([]string, error) {
	return s.guildSettings(guildID).MediaCategories, nil
}

// SetMediaDefault sets the category /media uses when none is given.
func (s *Storage) SetMediaDefault(guildID string, categoryID string) error {
	g := s.guildSettings(guildID)
	g.MediaDefault = categoryID
	return s.settings.Put(g)
}

// ResetMediaDefault clears the guild's default media category.
func (s *Storage) ResetMediaDefault(guildID string) error {
	g := s.guildSettings(guildID)
	g.MediaDefault = ""
	return s.settings.Put(g)
}

// GetMediaDefault returns the guild's default media category, empty if unset.
func (s *Storage) GetMediaDefault(guildID string) (string, error) {
	return s.guildSettings(guildID).MediaDefault, nil
}
