package commands

import (
	"fmt"
	"log"

	"github.com/bwmarrin/discordgo"
)

func init() {
	Register(&Command{
		Sort:           900,                                       // low sort to appear early
		Name:           "ping",                                    // command name
		Description:    "Send a ping and receive a teasing pong.", // command description
		Category:       "🕯️ Lore & Insight",                       // command category
		DCSlashHandler: pingSlashHandler,
	})
}

func buildPingMessage(s *discordgo.Session) (string, error) {
	latency := s.HeartbeatLatency().Milliseconds()
	return fmt.Sprintf("🏓 Pong! Response time: `%dms`", latency), nil
}

func pingSlashHandler(ctx *SlashContext) {
	s, i := ctx.Session, ctx.InteractionCreate

	msg, err := buildPingMessage(s)
	if err != nil {
		respondEphemeral(s, i, fmt.Sprintf("Failed to calculate ping: ```%v```", err))
		return
	}

	respondEphemeral(s, i, msg)

	guildID := i.GuildID
	userID := i.Member.User.ID
	username := i.Member.User.Username
	err = logCommand(s, ctx.Storage, guildID, i.ChannelID, userID, username, "ping")
	if err != nil {
		log.Println("Failed to log command:", err)
	}
}
