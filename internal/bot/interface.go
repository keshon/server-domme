package bot

import "server-domme/internal/music/player"

type BotVoice interface {
	GetOrCreatePlayer(guildID string) *player.Player
	FindUserVoiceState(guildID, userID string) (*VoiceState, error)
}
