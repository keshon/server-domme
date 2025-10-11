package announce

import (
	"fmt"
	"server-domme/internal/core"
	"server-domme/internal/storage"

	"github.com/bwmarrin/discordgo"
)

type AnnounceCommand struct{}

func (c *AnnounceCommand) Name() string { return "announce" }
func (c *AnnounceCommand) Description() string {
	return "Send messages to the announcement channel or manage it"
}
func (c *AnnounceCommand) Group() string    { return "announce" }
func (c *AnnounceCommand) Category() string { return "📢 Utilities" }
func (c *AnnounceCommand) UserPermissions() []int64 {
	return []int64{}
}

func (c *AnnounceCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionSubCommandGroup,
				Name:        "manage",
				Description: "Manage announcement channel",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionSubCommand,
						Name:        "channel",
						Description: "Set or update announcement channel",
						Options: []*discordgo.ApplicationCommandOption{
							{
								Type:        discordgo.ApplicationCommandOptionChannel,
								Name:        "channel",
								Description: "Pick a channel from this server",
								Required:    true,
							},
						},
					},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "publish",
				Description: "Publish a message to the announcement channel",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionString,
						Name:        "message_id",
						Description: "The ID of the message to publish",
						Required:    true,
					},
				},
			},
		},
	}
}

func (c *AnnounceCommand) Run(ctx interface{}) error {
	context, ok := ctx.(*core.SlashInteractionContext)
	if !ok {
		return nil
	}

	session := context.Session
	event := context.Event
	storage := context.Storage

	options := event.ApplicationCommandData().Options
	if len(options) == 0 {
		return core.RespondEphemeral(session, event, "No subcommand provided.")
	}

	first := options[0]
	switch first.Type {
	case discordgo.ApplicationCommandOptionSubCommandGroup:
		if first.Name == "manage" {
			sub := first.Options[0]
			if sub.Name == "channel" {
				return runManageAnnounceChannel(session, event, *storage, sub)
			}
		}
	case discordgo.ApplicationCommandOptionSubCommand:
		if first.Name == "publish" {
			messageID := ""
			if len(first.Options) > 0 {
				messageID = first.Options[0].StringValue()
			}
			if messageID == "" {
				return core.RespondEphemeral(session, event, "Missing message ID.")
			}
			return runPublishMessage(session, event, *storage, messageID)
		}
	}

	return core.RespondEphemeral(session, event, "Unknown subcommand or command structure.")
}

func runManageAnnounceChannel(s *discordgo.Session, e *discordgo.InteractionCreate, storage storage.Storage, sub *discordgo.ApplicationCommandInteractionDataOption) error {
	if !core.IsAdministrator(s, e.Member) {
		return core.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{Description: "You must be an admin to use this command."})
	}

	var channelID string
	for _, opt := range sub.Options {
		if opt.Name == "channel" {
			channelID = opt.ChannelValue(s).ID
		}
	}

	if channelID == "" {
		return core.RespondEphemeral(s, e, "Missing channel parameter.")
	}

	if err := storage.SetAnnounceChannel(e.GuildID, channelID); err != nil {
		return core.RespondEphemeral(s, e, fmt.Sprintf("Failed to set announcement channel: `%v`", err))
	}

	return core.RespondEphemeral(s, e, fmt.Sprintf("Announcement channel updated to <#%s>.", channelID))
}

func runPublishMessage(s *discordgo.Session, e *discordgo.InteractionCreate, storage storage.Storage, messageID string) error {
	announceChannelID, _ := storage.GetAnnounceChannel(e.GuildID)
	if announceChannelID == "" {
		return core.RespondEphemeral(s, e, "Announcement channel is not set. Use `/announce manage channel` first.")
	}

	// Fetch the message from current channel
	msg, err := s.ChannelMessage(e.ChannelID, messageID)
	if err != nil {
		return core.RespondEphemeral(s, e, fmt.Sprintf("Failed to fetch message: %v", err))
	}

	_, err = s.ChannelMessageSend(announceChannelID, msg.Content)
	if err != nil {
		return core.RespondEphemeral(s, e, fmt.Sprintf("Failed to publish message: %v", err))
	}

	return core.RespondEphemeral(s, e, fmt.Sprintf("Message successfully published to <#%s>.", announceChannelID))
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
