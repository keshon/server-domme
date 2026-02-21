package discord

import (
	"fmt"

	"server-domme/internal/music/player"
	"server-domme/internal/music/source_resolver"
)

// BotVoice is the interface the Discord bot exposes for voice/music commands.
type BotVoice interface {
	GetOrCreatePlayer(guildID string) *player.Player
	FindUserVoiceState(guildID, userID string) (*VoiceState, error)
}

// VoiceState holds minimal voice channel state for a user.
type VoiceState struct {
	ChannelID string
	UserID    string
}

// GetOrCreatePlayer returns an existing player for the guild or creates a new one.
func (b *Bot) GetOrCreatePlayer(guildID string) *player.Player {
	b.mu.Lock()
	defer b.mu.Unlock()

	if p, ok := b.players[guildID]; ok {
		return p
	}
	if b.sourceResolver == nil {
		b.sourceResolver = source_resolver.New()
	}
	p := player.New(b.dg, guildID, b.storage, b.sourceResolver)
	b.players[guildID] = p
	return p
}

// FindUserVoiceState returns the voice channel a user is currently in, or an error if none.
func (b *Bot) FindUserVoiceState(guildID, userID string) (*VoiceState, error) {
	guild, err := b.dg.State.Guild(guildID)
	if err != nil {
		return nil, fmt.Errorf("error retrieving guild: %w", err)
	}
	for _, vs := range guild.VoiceStates {
		if vs.UserID == userID {
			return &VoiceState{ChannelID: vs.ChannelID, UserID: vs.UserID}, nil
		}
	}
	return nil, fmt.Errorf("user not in any voice channel")
}
