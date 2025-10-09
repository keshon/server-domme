package confess

import (
	"fmt"
	"server-domme/internal/core"
	"strings"

	"github.com/bwmarrin/discordgo"
)

type ConfessCommand struct{}

func (c *ConfessCommand) Name() string        { return "confess" }
func (c *ConfessCommand) Description() string { return "Send an anonymous confession" }
func (c *ConfessCommand) Aliases() []string   { return []string{} }
func (c *ConfessCommand) Group() string       { return "confess" }
func (c *ConfessCommand) Category() string    { return "🎭 Roleplay" }
func (c *ConfessCommand) RequireAdmin() bool  { return false }
func (c *ConfessCommand) RequireDev() bool    { return false }

func (c *ConfessCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "message",
				Description: "What do you need to confess?",
				Required:    true,
			},
		},
	}
}

func (c *ConfessCommand) Run(ctx interface{}) error {
	context, ok := ctx.(*core.SlashInteractionContext)
	if !ok {
		return nil
	}

	session := context.Session
	event := context.Event
	storage := context.Storage

	var message string
	for _, opt := range event.ApplicationCommandData().Options {
		if opt.Name == "message" {
			message = strings.TrimSpace(opt.StringValue())
		}
	}

	if message == "" {
		core.RespondEphemeral(session, event, "You can't confess silence. Try again.")
		return nil
	}

	confessChannelID, err := storage.GetSpecialChannel(event.GuildID, "confession")
	if err != nil || confessChannelID == "" {
		core.RespondEphemeral(session, event, "No confession channel is configured. Ask a mod to set it up.")
		return nil
	}

	embed := &discordgo.MessageEmbed{
		Title:       "📢 Anonymous Confession",
		Description: fmt.Sprintf("> %s", message),
		Color:       core.EmbedColor,
	}

	err = core.RespondEmbedEphemeral(session, event, embed)
	if err != nil {
		return core.RespondEmbedEphemeral(session, event, &discordgo.MessageEmbed{Description: fmt.Sprintf("Failed to send confession: %v", err)})
	}

	if event.ChannelID != confessChannelID {
		link := fmt.Sprintf("https://discord.com/channels/%s/%s", event.GuildID, confessChannelID)
		core.RespondEphemeral(session, event, fmt.Sprintf("Delivered. Nobody saw a thing.\nSee it here: %s", link))
		return nil
	} else {
		core.RespondEphemeral(session, event, "💌 Delivered. Nobody saw a thing.")
		return nil
	}
}

func init() {
	core.RegisterCommand(
		core.ApplyMiddlewares(
			&ConfessCommand{},
			core.WithGroupAccessCheck(),
			core.WithGuildOnly(),
			core.WithAccessControl(),
			core.WithCommandLogger(),
		),
	)
}
