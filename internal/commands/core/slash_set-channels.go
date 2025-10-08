package core

import (
	"fmt"
	"log"
	"server-domme/internal/core"

	"github.com/bwmarrin/discordgo"
)

type SetChannelsCommand struct{}

func (c *SetChannelsCommand) Name() string        { return "set-channels" }
func (c *SetChannelsCommand) Description() string { return "Setup special-purpose channels" }
func (c *SetChannelsCommand) Aliases() []string   { return []string{} }
func (c *SetChannelsCommand) Group() string       { return "core" }
func (c *SetChannelsCommand) Category() string    { return "⚙️ Settings" }
func (c *SetChannelsCommand) RequireAdmin() bool  { return true }
func (c *SetChannelsCommand) RequireDev() bool    { return false }

func (c *SetChannelsCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Type:        discordgo.ChatApplicationCommand,
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "type",
				Description: "What kind of channel are you setting?",
				Required:    true,
				Choices: []*discordgo.ApplicationCommandOptionChoice{
					{Name: "Confession Channel", Value: "confession"},
					{Name: "Announcement Channel", Value: "announce"},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionChannel,
				Name:        "channel",
				Description: "Pick a channel from this server",
				Required:    true,
			},
		},
	}
}

func (c *SetChannelsCommand) Run(ctx interface{}) error {
	context, ok := ctx.(*core.SlashInteractionContext)
	if !ok {
		return nil
	}

	session, event, storage := context.Session, context.Event, context.Storage
	guildID, member := event.GuildID, event.Member

	// Parse command options
	var kind, channelID string
	for _, opt := range event.ApplicationCommandData().Options {
		switch opt.Name {
		case "type":
			kind = opt.StringValue()
		case "channel":
			channelID = opt.ChannelValue(session).ID
		}
	}

	// Validate input
	if kind == "" || channelID == "" {
		return core.RespondEmbedEphemeral(session, event, &discordgo.MessageEmbed{
			Description: "Missing parameters. Don’t make me repeat myself.",
		})
	}

	// Save to storage
	if err := storage.SetSpecialChannel(guildID, kind, channelID); err != nil {
		return core.RespondEmbedEphemeral(session, event, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Failed to set channel: ```%v```", err),
		})
	}

	// Pick response text
	msg := map[string]string{
		"confession": "💬 Confession channel updated. Secrets will flow in silence.",
		"announce":   "📢 Announcement channel set. Don’t disappoint me with boring news.",
	}[kind]
	if msg == "" {
		msg = fmt.Sprintf("✅ Channel for `%s` set.", kind)
	}

	// Log usage
	if err := core.LogCommand(session, storage, guildID, event.ChannelID, member.User.ID, member.User.Username, c.Name()); err != nil {
		log.Println("Failed to log:", err)
	}

	// Send response
	return core.RespondEmbedEphemeral(session, event, &discordgo.MessageEmbed{
		Description: msg,
	})
}

func init() {
	core.RegisterCommand(
		core.ApplyMiddlewares(
			&SetChannelsCommand{},
			core.WithGroupAccessCheck(),
			core.WithGuildOnly(),
			core.WithAccessControl(),
		),
	)
}
func init() {
	core.RegisterCommand(
		core.ApplyMiddlewares(
			&SetChannelsCommand{},
			core.WithGroupAccessCheck(),
			core.WithGuildOnly(),
			core.WithAccessControl(),
		),
	)
}
