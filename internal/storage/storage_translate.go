package storage

import (
	"fmt"
	"slices"
)

// AddTranslateChannel opts a channel into reaction-triggered translation.
func (s *Storage) AddTranslateChannel(guildID string, channelID string) error {
	g := s.guildSettings(guildID)
	if slices.Contains(g.TranslateChannels, channelID) {
		return fmt.Errorf("channel already in translate list")
	}
	g.TranslateChannels = append(g.TranslateChannels, channelID)
	return s.settings.Put(g)
}

// RemoveTranslateChannel opts a channel back out.
func (s *Storage) RemoveTranslateChannel(guildID string, channelID string) error {
	g := s.guildSettings(guildID)
	if len(g.TranslateChannels) == 0 {
		return fmt.Errorf("no translate channels configured")
	}
	updated := slices.DeleteFunc(g.TranslateChannels, func(c string) bool {
		return c == channelID
	})
	if len(updated) == len(g.TranslateChannels) {
		return fmt.Errorf("channel not found in translate list")
	}
	g.TranslateChannels = updated
	return s.settings.Put(g)
}

// GetTranslateChannels lists the guild's translation channels, never nil.
func (s *Storage) GetTranslateChannels(guildID string) ([]string, error) {
	channels := s.guildSettings(guildID).TranslateChannels
	if channels == nil {
		return []string{}, nil
	}
	return channels, nil
}

// ResetTranslateChannels clears every translation channel for the guild.
func (s *Storage) ResetTranslateChannels(guildID string) error {
	g := s.guildSettings(guildID)
	g.TranslateChannels = nil
	return s.settings.Put(g)
}
