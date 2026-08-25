package translate

import (
	"fmt"

	"github.com/bwmarrin/discordgo"
	"github.com/keshon/server-domme/internal/discord/reply"
	"github.com/keshon/server-domme/internal/storage"
)

// RunManageTranslateChannel handles translate channel settings subcommands.
func RunManageTranslateChannel(s *discordgo.Session, e *discordgo.InteractionCreate, storage storage.Storage, sub *discordgo.ApplicationCommandInteractionDataOption) error {
	switch sub.Name {
	case "channel-add":
		return runAddChannel(s, e, storage, sub)
	case "channel-remove":
		return runRemoveChannel(s, e, storage, sub)
	case "channel-list":
		return runListChannels(s, e, storage)
	case "channels-clear":
		return runResetChannels(s, e, storage)
	default:
		return reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "Unknown subcommand provided.",
		})
	}
}

// TranslateChannelOptions returns slash options for translate channel settings.
func TranslateChannelOptions() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionSubCommand,
			Name:        "channel-add",
			Description: "Enable translation reactions in a channel",
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
			Name:        "channel-remove",
			Description: "Disable translation reactions in a channel",
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
			Name:        "channel-list",
			Description: "List translation-enabled channels",
		},
		{
			Type:        discordgo.ApplicationCommandOptionSubCommand,
			Name:        "channels-clear",
			Description: "Remove all translation-enabled channels",
		},
	}
}

func runAddChannel(s *discordgo.Session, e *discordgo.InteractionCreate, storage storage.Storage, sub *discordgo.ApplicationCommandInteractionDataOption) error {
	channelID := sub.Options[0].ChannelValue(s).ID
	if err := storage.AddTranslateChannel(e.GuildID, channelID); err != nil {
		return reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Failed to add channel: `%v`", err),
		})
	}
	return reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
		Description: fmt.Sprintf("<#%s> added to translate reaction channels.", channelID),
	})
}

func runRemoveChannel(s *discordgo.Session, e *discordgo.InteractionCreate, storage storage.Storage, sub *discordgo.ApplicationCommandInteractionDataOption) error {
	channelID := sub.Options[0].ChannelValue(s).ID
	if err := storage.RemoveTranslateChannel(e.GuildID, channelID); err != nil {
		return reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Failed to remove channel: `%v`", err),
		})
	}
	return reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
		Description: fmt.Sprintf("<#%s> removed from translate reaction channels.", channelID),
	})
}

func runListChannels(s *discordgo.Session, e *discordgo.InteractionCreate, storage storage.Storage) error {
	channels, err := storage.GetTranslateChannels(e.GuildID)
	if err != nil {
		return reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Failed to get channels: `%v`", err),
		})
	}

	if len(channels) == 0 {
		return reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "No channels currently configured for translation reactions.",
		})
	}

	desc := "Channels enabled for translation reactions:\n"
	for _, ch := range channels {
		desc += fmt.Sprintf("- <#%s>\n", ch)
	}

	return reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
		Title:       "🌐 Translate Channels",
		Description: desc,
		Color:       reply.EmbedColor,
	})
}

func runResetChannels(s *discordgo.Session, e *discordgo.InteractionCreate, storage storage.Storage) error {
	if err := storage.ResetTranslateChannels(e.GuildID); err != nil {
		return reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Failed to reset channels: `%v`", err),
		})
	}
	return reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
		Description: "All translate reaction channels have been reset.",
	})
}
