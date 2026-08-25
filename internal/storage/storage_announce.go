package storage

import "fmt"

// SetAnnounceChannel sets the channel announcements are posted to.
func (s *Storage) SetAnnounceChannel(guildID, channelID string) error {
	g := s.guildSettings(guildID)
	g.AnnounceChannel = channelID
	return s.settings.Put(g)
}

// GetAnnounceChannel returns the configured announcement channel, or an error
// when the guild has not set one.
func (s *Storage) GetAnnounceChannel(guildID string) (string, error) {
	channelID := s.guildSettings(guildID).AnnounceChannel
	if channelID == "" {
		return "", fmt.Errorf("no announce channel set")
	}
	return channelID, nil
}

// RemoveAnnounceChannel clears the guild's announcement channel.
func (s *Storage) RemoveAnnounceChannel(guildID string) error {
	g := s.guildSettings(guildID)
	g.AnnounceChannel = ""
	return s.settings.Put(g)
}
