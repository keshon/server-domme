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

	session := context.Session
	event := context.Event
	options := event.ApplicationCommandData().Options
	storage := context.Storage

	guildID := event.GuildID
	member := event.Member

	if !core.IsAdministrator(session, event.GuildID, event.Member) {
		return core.RespondEphemeral(session, event, "You must be an Admin to use this command, darling.")
	}

	var kind, channelID string
	for _, opt := range options {
		switch opt.Name {
		case "type":
			kind = opt.StringValue()
		case "channel":
			channelID = opt.ChannelValue(session).ID
		}
	}

	if kind == "" || channelID == "" {
		return core.RespondEphemeral(session, event, "Missing required parameters. Don't make me repeat myself.")
	}

	err := storage.SetSpecialChannel(event.GuildID, kind, channelID)
	if err != nil {
		return core.RespondEphemeral(session, event, fmt.Sprintf("Couldn’t save it: `%s`", err.Error()))
	}

	var confirmation string
	switch kind {
	case "confession":
		confirmation = "💬 Confession channel updated. May secrets drip in silence."
	case "announce":
		confirmation = "📢 Announcement channel set. Don’t disappoint me with boring news."
	default:
		confirmation = fmt.Sprintf("✅ Channel for `%s` set.", kind)
	}

	err = core.RespondEphemeral(session, event, confirmation)
	if err != nil {
		return err
	}

	err = core.LogCommand(session, storage, guildID, event.ChannelID, member.User.ID, member.User.Username, c.Name())
	if err != nil {
		log.Println("Failed to log:", err)
	}

	return nil
}

func init() {
	core.RegisterCommand(
		core.ApplyMiddlewares(
			&SetChannelsCommand{},
			core.WithGroupAccessCheck(),
			core.WithGuildOnly(),
		),
	)
}
