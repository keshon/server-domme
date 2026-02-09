package translate

import (
	"fmt"

	"server-domme/internal/discord"
	"server-domme/internal/command"
	"server-domme/internal/middleware"
	"server-domme/internal/storage"

	"github.com/bwmarrin/discordgo"
)

// Need to add at least one channel or translation wont work

type ManageTranslateCommand struct{}

func (c *ManageTranslateCommand) Name() string        { return "manage-translate" }
func (c *ManageTranslateCommand) Description() string { return "Translate settings" }
func (c *ManageTranslateCommand) Group() string       { return "translate" }
func (c *ManageTranslateCommand) Category() string    { return "⚙️ Settings" }
func (c *ManageTranslateCommand) UserPermissions() []int64 {
	return []int64{discordgo.PermissionAdministrator}
}

func (c *ManageTranslateCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "set-channel",
				Description: "Add a channel to the translate list",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionChannel,
						Name:        "channel",
						Description: "Select a channel to enable translation reactions",
						Required:    true,
					},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "reset-channel",
				Description: "Remove a channel from the translate list",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionChannel,
						Name:        "channel",
						Description: "Select a channel to remove from translation reactions",
						Required:    true,
					},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "list-channels",
				Description: "List all channels enabled for translation reactions",
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "reset-all-channels",
				Description: "Reset all channels for translation reactions",
			},
		},
	}
}

func (c *ManageTranslateCommand) Run(ctx interface{}) error {
	context, ok := ctx.(*command.SlashInteractionContext)
	if !ok {
		return nil
	}

	s, e, storage := context.Session, context.Event, context.Storage

	options := e.ApplicationCommandData().Options
	if len(options) == 0 {
		return discord.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "No subcommand provided.",
		})
	}

	sub := options[0]
	switch sub.Name {
	case "set-channel":
		return runAddChannel(s, e, *storage, sub)
	case "reset-channel":
		return runRemoveChannel(s, e, *storage, sub)
	case "list-channels":
		return runListChannels(s, e, *storage)
	case "reset-all-channels":
		return runResetChannels(s, e, *storage)
	default:
		return discord.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "Unknown subcommand provided.",
		})
	}
}

func runAddChannel(s *discordgo.Session, e *discordgo.InteractionCreate, storage storage.Storage, sub *discordgo.ApplicationCommandInteractionDataOption) error {
	channelID := sub.Options[0].ChannelValue(s).ID
	if err := storage.AddTranslateChannel(e.GuildID, channelID); err != nil {
		return discord.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Failed to add channel: `%v`", err),
		})
	}
	return discord.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
		Description: fmt.Sprintf("<#%s> added to translate reaction channels.", channelID),
	})
}

func runRemoveChannel(s *discordgo.Session, e *discordgo.InteractionCreate, storage storage.Storage, sub *discordgo.ApplicationCommandInteractionDataOption) error {
	channelID := sub.Options[0].ChannelValue(s).ID
	if err := storage.RemoveTranslateChannel(e.GuildID, channelID); err != nil {
		return discord.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Failed to remove channel: `%v`", err),
		})
	}
	return discord.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
		Description: fmt.Sprintf("<#%s> removed from translate reaction channels.", channelID),
	})
}

func runListChannels(s *discordgo.Session, e *discordgo.InteractionCreate, storage storage.Storage) error {
	channels, err := storage.GetTranslateChannels(e.GuildID)
	if err != nil {
		return discord.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Failed to get channels: `%v`", err),
		})
	}

	if len(channels) == 0 {
		return discord.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "No channels currently configured for translation reactions.",
		})
	}

	desc := "Channels enabled for translation reactions:\n"
	for _, ch := range channels {
		desc += fmt.Sprintf("- <#%s>\n", ch)
	}

	return discord.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
		Title:       "🌐 Translate Channels",
		Description: desc,
		Color:       discord.EmbedColor,
	})
}

func runResetChannels(s *discordgo.Session, e *discordgo.InteractionCreate, storage storage.Storage) error {
	if err := storage.ResetTranslateChannels(e.GuildID); err != nil {
		return discord.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Failed to reset channels: `%v`", err),
		})
	}
	return discord.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
		Description: "All translate reaction channels have been reset.",
	})
}

func init() {
	command.RegisterCommand(
		&ManageTranslateCommand{},
		middleware.WithGroupAccessCheck(),
		middleware.WithGuildOnly(),
		middleware.WithUserPermissionCheck(),
		middleware.WithCommandLogger(),
	)
}
