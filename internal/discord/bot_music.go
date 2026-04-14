package discord

import (
	"errors"

	"github.com/bwmarrin/discordgo"
	"github.com/keshon/melodix/pkg/music/player"
	"github.com/keshon/melodix/pkg/music/sources"
)

var ErrVoiceUnavailable = errors.New("voice service unavailable")

// BotVoice is the interface the Discord bot exposes for voice/music commands.
type BotVoice interface {
	GetOrCreatePlayer(guildID string) *player.Player
	FindUserVoiceState(guildID, userID string) (*VoiceState, error)
	Resolve(guildID, input, source, parser string) ([]sources.TrackInfo, error)
	// UpdateGuildMusicStatus creates or edits the guild's music status message so updates work beyond 15 min token expiry.
	UpdateGuildMusicStatus(s *discordgo.Session, i *discordgo.InteractionCreate, guildID string, embed *discordgo.MessageEmbed) error
}

// VoiceState holds minimal voice channel state for a user.
type VoiceState struct {
	ChannelID string
	UserID    string
}

func (b *Bot) GetOrCreatePlayer(guildID string) *player.Player {
	b.mu.RLock()
	v := b.voice
	b.mu.RUnlock()
	if v == nil {
		return nil
	}
	return v.GetOrCreatePlayer(guildID)
}

func (b *Bot) FindUserVoiceState(guildID, userID string) (*VoiceState, error) {
	b.mu.RLock()
	v := b.voice
	b.mu.RUnlock()
	if v == nil {
		return nil, ErrVoiceUnavailable
	}
	vs, err := v.FindUserVoiceState(guildID, userID)
	if err != nil {
		return nil, err
	}
	if vs == nil {
		return nil, nil
	}
	return &VoiceState{ChannelID: vs.ChannelID, UserID: vs.UserID}, nil
}

func (b *Bot) Resolve(guildID, input, source, parser string) ([]sources.TrackInfo, error) {
	b.mu.RLock()
	v := b.voice
	b.mu.RUnlock()
	if v == nil {
		return nil, ErrVoiceUnavailable
	}
	return v.Resolve(guildID, input, source, parser)
}

func (b *Bot) UpdateGuildMusicStatus(s *discordgo.Session, i *discordgo.InteractionCreate, guildID string, embed *discordgo.MessageEmbed) error {
	b.mu.RLock()
	v := b.voice
	b.mu.RUnlock()
	if v == nil {
		return nil
	}
	return v.UpdateGuildMusicStatus(s, i, guildID, embed)
}

