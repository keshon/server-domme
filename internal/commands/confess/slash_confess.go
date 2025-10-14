package confess

import (
	"fmt"
	"server-domme/internal/core"
	"server-domme/internal/storage"
	"strings"

	"github.com/bwmarrin/discordgo"
)

type ConfessCommand struct{}

func (c *ConfessCommand) Name() string        { return "confess" }
func (c *ConfessCommand) Description() string { return "Send an anonymous confession" }
func (c *ConfessCommand) Group() string       { return "confess" }
func (c *ConfessCommand) Category() string    { return "🎭 Roleplay" }
func (c *ConfessCommand) UserPermissions() []int64 {
	return []int64{}
}

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

	s := context.Session
	e := context.Event
	storage := context.Storage

	data := e.ApplicationCommandData()

	var message string
	if len(data.Options) > 0 {
		message = strings.TrimSpace(data.Options[0].StringValue())
	}

	if message == "" {
		return core.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "No confession provided.",
		})
	}

	return c.runSendConfession(s, e, *storage, message)
}

func (c *ConfessCommand) runSendConfession(s *discordgo.Session, e *discordgo.InteractionCreate, storage storage.Storage, message string) error {
	confessChannelID, err := storage.GetConfessChannel(e.GuildID)
	if err != nil || confessChannelID == "" {
		// No confession channel set - fallback to current channel
		confessChannelID = e.ChannelID
	}

	embed := &discordgo.MessageEmbed{
		Title:       "📢 Anonymous Confession",
		Description: fmt.Sprintf("> %s", message),
		Color:       core.EmbedColor,
	}

	// Post the confession message to the target channel (not ephemeral)
	_, err = s.ChannelMessageSendEmbed(confessChannelID, embed)
	if err != nil {
		return core.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Failed to send confession: %v", err),
		})
	}

	// Notify the user privately (ephemeral)
	if confessChannelID != e.ChannelID {
		link := fmt.Sprintf("https://discord.com/channels/%s/%s", e.GuildID, confessChannelID)
		core.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Delivered. Nobody saw a thing.\nSee it here: %s", link),
		})
	} else {
		core.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "Delivered. Nobody saw a thing.",
		})
	}

	return nil
}

func init() {
	core.RegisterCommand(
		core.ApplyMiddlewares(
			&ConfessCommand{},
			core.WithGroupAccessCheck(),
			core.WithGuildOnly(),
			core.WithUserPermissionCheck(),
			core.WithCommandLogger(),
		),
	)
}
