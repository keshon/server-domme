package core

import (
	"fmt"
	"log"
	"server-domme/internal/core"

	"github.com/bwmarrin/discordgo"
)

type PingCommand struct{}

func (c *PingCommand) Name() string        { return "ping" }
func (c *PingCommand) Description() string { return "Check bot latency" }
func (c *PingCommand) Aliases() []string   { return []string{} }
func (c *PingCommand) Group() string       { return "ping" }
func (c *PingCommand) Category() string    { return "🛠️ Maintenance" }
func (c *PingCommand) RequireAdmin() bool  { return false }
func (c *PingCommand) RequireDev() bool    { return false }

func (c *PingCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Type:        discordgo.ChatApplicationCommand,
	}
}

func (c *PingCommand) Run(ctx interface{}) error {
	context, ok := ctx.(*core.SlashInteractionContext)
	if !ok {
		return nil
	}

	session := context.Session
	event := context.Event
	storage := context.Storage

	guildID := event.GuildID
	member := event.Member

	err := core.LogCommand(session, storage, guildID, event.ChannelID, member.User.ID, member.User.Username, c.Name())
	if err != nil {
		log.Println("Failed to log:", err)
	}

	latency := session.HeartbeatLatency().Milliseconds()

	return core.RespondEphemeral(session, event, fmt.Sprintf("🏓 Pong! Latency: %dms", latency))
}

func init() {
	core.RegisterCommand(
		core.ApplyMiddlewares(
			&PingCommand{},
			core.WithGroupAccessCheck(),
			core.WithGuildOnly(),
		),
	)
}
