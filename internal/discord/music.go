package discord

import (
	"fmt"
	"server-domme/internal/commands/music"
	"server-domme/internal/core"
	"server-domme/internal/music/player"
	"server-domme/internal/music/source_resolver"
)

func (b *Bot) registerMusicCommands() {

	core.RegisterCommand(
		core.ApplyMiddlewares(
			&music.PlayCommand{Bot: b},
			core.WithGroupAccessCheck(),
			core.WithGuildOnly(),
			core.WithAccessControl(),
			core.WithCommandLogger(),
		),
	)

	core.RegisterCommand(
		core.ApplyMiddlewares(
			&music.StopCommand{Bot: b},
			core.WithGroupAccessCheck(),
			core.WithGuildOnly(),
			core.WithAccessControl(),
			core.WithCommandLogger(),
		),
	)

	core.RegisterCommand(
		core.ApplyMiddlewares(
			&music.NextCommand{Bot: b},
			core.WithGroupAccessCheck(),
			core.WithGuildOnly(),
			core.WithAccessControl(),
			core.WithCommandLogger(),
		),
	)

}

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

func (b *Bot) FindUserVoiceState(guildID, userID string) (*core.VoiceState, error) {
	guild, err := b.dg.State.Guild(guildID)
	if err != nil {
		return nil, fmt.Errorf("error retrieving guild: %w", err)
	}

	for _, vs := range guild.VoiceStates {
		if vs.UserID == userID {
			return &core.VoiceState{
				ChannelID: vs.ChannelID,
				UserID:    vs.UserID,
			}, nil
		}
	}
	return nil, fmt.Errorf("user not in any voice channel")
}
