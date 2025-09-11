package discord

import (
	"fmt"
	"server-domme/internal/command"
	"server-domme/internal/core"
	"server-domme/internal/music/player"
	"server-domme/internal/music/source_resolver"
)

func (b *Bot) registerMusicCommands() {
	play := &command.PlayCommand{Bot: b}
	core.RegisterCommand(
		core.WithGroupAccessCheck()(
			core.WithGuildOnly(play),
		),
	)

	stop := &command.StopCommand{Bot: b}
	core.RegisterCommand(
		core.WithGroupAccessCheck()(
			core.WithGuildOnly(stop),
		),
	)

	next := &command.NextCommand{Bot: b}
	core.RegisterCommand(
		core.WithGroupAccessCheck()(
			core.WithGuildOnly(next),
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
