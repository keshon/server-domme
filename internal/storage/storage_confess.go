package storage

import "fmt"

// SetConfessChannel sets the channel confessions are relayed to.
func (s *Storage) SetConfessChannel(guildID, channelID string) error {
	g := s.guildSettings(guildID)
	g.ConfessChannel = channelID
	return s.settings.Put(g)
}

// GetConfessChannel returns the configured confession channel, or an error when
// the guild has not set one.
func (s *Storage) GetConfessChannel(guildID string) (string, error) {
	channelID := s.guildSettings(guildID).ConfessChannel
	if channelID == "" {
		return "", fmt.Errorf("storage: no confession channel set")
	}
	return channelID, nil
}

// RemoveConfessChannel clears the guild's confession channel.
func (s *Storage) RemoveConfessChannel(guildID string) error {
	g := s.guildSettings(guildID)
	g.ConfessChannel = ""
	return s.settings.Put(g)
}

// SetConfessAICheck enables or disables the optional AI pre-post guard.
func (s *Storage) SetConfessAICheck(guildID string, enabled bool) error {
	g := s.guildSettings(guildID)
	g.ConfessAICheck = enabled
	return s.settings.Put(g)
}

// GetConfessAICheck reports whether the AI guard runs before posting.
// Default off: unset means post exactly as written.
func (s *Storage) GetConfessAICheck(guildID string) bool {
	return s.guildSettings(guildID).ConfessAICheck
}
