package announce

import (
	"fmt"
	"server-domme/internal/core"
	"server-domme/internal/storage"

	"github.com/bwmarrin/discordgo"
)

type AnnounceCommand struct{}

func (c *AnnounceCommand) Name() string        { return "announce" }
func (c *AnnounceCommand) Description() string { return "Send a message on bot's behalf" }
func (c *AnnounceCommand) Group() string       { return "announce" }
func (c *AnnounceCommand) Category() string    { return "📢 Utilities" }
func (c *AnnounceCommand) UserPermissions() []int64 {
	return []int64{}
}

func (c *AnnounceCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "message_id",
				Description: "The ID of the message to publish",
				Required:    true,
			},
		},
	}
}

func (c *AnnounceCommand) Run(ctx interface{}) error {
	context, ok := ctx.(*core.SlashInteractionContext)
	if !ok {
		return nil
	}

	s := context.Session
	e := context.Event
	st := context.Storage

	data := e.ApplicationCommandData()
	if len(data.Options) == 0 {
		return core.RespondEphemeral(s, e, "Please provide a message ID to announce.")
	}

	messageID := data.Options[0].StringValue()
	return c.runPublishMessage(s, e, *st, messageID)
}

func (c *AnnounceCommand) runPublishMessage(s *discordgo.Session, e *discordgo.InteractionCreate, st storage.Storage, messageID string) error {
	announceChannelID, _ := st.GetAnnounceChannel(e.GuildID)
	if announceChannelID == "" {
		return core.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "Announcement channel is not set. Use `/manage-announce set-channel` first.",
		})
	}

	msg, err := s.ChannelMessage(e.ChannelID, messageID)
	if err != nil {
		return core.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Failed to fetch message: %v", err),
		})
	}

	_, err = s.ChannelMessageSend(announceChannelID, msg.Content)
	if err != nil {
		return core.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Failed to publish message: %v", err),
		})
	}

	return core.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
		Description: fmt.Sprintf("Message successfully published to <#%s>.", announceChannelID),
	})
}

func init() {
	core.RegisterCommand(
		core.ApplyMiddlewares(
			&AnnounceCommand{},
			core.WithGroupAccessCheck(),
			core.WithGuildOnly(),
			core.WithUserPermissionCheck(),
			core.WithCommandLogger(),
		),
	)
}
