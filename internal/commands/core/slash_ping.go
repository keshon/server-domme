package core

import (
	"fmt"
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
func (c *PingCommand) Permissions() []int64 {
	return []int64{
		discordgo.PermissionSendMessages,
	}
}
func (c *PingCommand) BotPermissions() []int64 {
	return []int64{
		discordgo.PermissionSendMessages,
	}
}

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

	session, event := context.Session, context.Event
	latency := session.HeartbeatLatency().Milliseconds()

	// Send response
	core.RespondEmbedEphemeral(session, event, &discordgo.MessageEmbed{
		Title:       "Pong!",
		Description: fmt.Sprintf("Latency: %dms", latency),
	})

	return nil
}

func init() {
	core.RegisterCommand(
		core.ApplyMiddlewares(
			&PingCommand{},
			core.WithGroupAccessCheck(),
			core.WithGuildOnly(),
			core.WithAccessControl(),
			core.WithPermissionCheck(),
			core.WithBotPermissionCheck(),
			core.WithCommandLogger(),
		),
	)
}
