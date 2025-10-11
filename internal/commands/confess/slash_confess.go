package confess

import (
	"fmt"
	"server-domme/internal/core"
	"server-domme/internal/storage"
	"strings"

	"github.com/bwmarrin/discordgo"
)

type ConfessCommand struct{}

func (c *ConfessCommand) Name() string { return "confess" }
func (c *ConfessCommand) Description() string {
	return "Send an anonymous confession or manage the confession channel"
}
func (c *ConfessCommand) Group() string    { return "confess" }
func (c *ConfessCommand) Category() string { return "🎭 Roleplay" }
func (c *ConfessCommand) UserPermissions() []int64 {
	return []int64{}
}

func (c *ConfessCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionSubCommandGroup,
				Name:        "manage",
				Description: "Manage confession channel",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionSubCommand,
						Name:        "set-channel",
						Description: "Set the confession channel",
						Options: []*discordgo.ApplicationCommandOption{
							{
								Type:        discordgo.ApplicationCommandOptionChannel,
								Name:        "channel",
								Description: "Pick a channel from this server",
								Required:    true,
							},
						},
					},
					{
						Type:        discordgo.ApplicationCommandOptionSubCommand,
						Name:        "list-channel",
						Description: "Show the currently configured confession channel",
					},
					{
						Type:        discordgo.ApplicationCommandOptionSubCommand,
						Name:        "reset-channel",
						Description: "Remove the confession channel",
					},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "send",
				Description: "Send an anonymous confession",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionString,
						Name:        "message",
						Description: "What do you need to confess?",
						Required:    true,
					},
				},
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

	options := e.ApplicationCommandData().Options
	if len(options) == 0 {
		return core.RespondEphemeral(s, e, "No subcommand provided.")
	}

	first := options[0]
	switch first.Type {
	case discordgo.ApplicationCommandOptionSubCommandGroup:
		if first.Name == "manage" && len(first.Options) > 0 {
			sub := first.Options[0] // this is now set/list/remove
			return runManageConfessionChannel(s, e, *storage, sub)
		}
	case discordgo.ApplicationCommandOptionSubCommand:
		if first.Name == "send" {
			message := ""
			if len(first.Options) > 0 && first.Options[0].Name == "message" {
				message = strings.TrimSpace(first.Options[0].StringValue())
			}
			if message == "" {
				return core.RespondEphemeral(s, e, "You can't confess silence. Try again.")
			}
			return runSendConfession(s, e, *storage, message)
		}
	}

	return core.RespondEphemeral(s, e, "Unknown subcommand or command structure.")
}

// ----- Manage confession channel -----
func runManageConfessionChannel(s *discordgo.Session, e *discordgo.InteractionCreate, storage storage.Storage, sub *discordgo.ApplicationCommandInteractionDataOption) error {
	if !core.IsAdministrator(s, e.Member) {
		return core.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{Description: "You must be an admin to use this command."})
	}

	switch sub.Name {
	case "set-channel":
		channelID := sub.Options[0].ChannelValue(s).ID
		if err := storage.SetConfessChannel(e.GuildID, channelID); err != nil {
			return core.RespondEphemeral(s, e, fmt.Sprintf("Failed to set confession channel: `%v`", err))
		}
		return core.RespondEphemeral(s, e, fmt.Sprintf("✅ Confession channel updated to <#%s>.", channelID))

	case "list-channel":
		channelID, err := storage.GetConfessChannel(e.GuildID)
		if err != nil {
			return core.RespondEphemeral(s, e, "No confession channel is currently set.")
		}
		return core.RespondEphemeral(s, e, fmt.Sprintf("Current confession channel is <#%s>.", channelID))

	case "reset-channel":
		if err := storage.RemoveConfessChannel(e.GuildID); err != nil {
			return core.RespondEphemeral(s, e, fmt.Sprintf("Failed to remove confession channel: `%v`", err))
		}
		return core.RespondEphemeral(s, e, "✅ Confession channel has been removed.")

	default:
		return core.RespondEphemeral(s, e, fmt.Sprintf("Unknown subcommand: %s", sub.Name))
	}
}

func runSendConfession(s *discordgo.Session, e *discordgo.InteractionCreate, storage storage.Storage, message string) error {
	confessChannelID, err := storage.GetConfessChannel(e.GuildID)
	if err != nil || confessChannelID == "" {
		// No confession channel set → fallback to current channel
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
		return core.RespondEphemeral(s, e, fmt.Sprintf("Failed to send confession: %v", err))
	}

	// Notify the user privately (ephemeral)
	if confessChannelID != e.ChannelID {
		link := fmt.Sprintf("https://discord.com/channels/%s/%s", e.GuildID, confessChannelID)
		core.RespondEphemeral(s, e, fmt.Sprintf("Delivered. Nobody saw a thing.\nSee it here: %s", link))
	} else {
		core.RespondEphemeral(s, e, "💌 Delivered. Nobody saw a thing.")
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
