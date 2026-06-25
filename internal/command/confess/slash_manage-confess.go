package confess

import (
	"fmt"

	"github.com/keshon/server-domme/internal/discord/discordreply"
	"github.com/keshon/server-domme/internal/storage"

	"github.com/bwmarrin/discordgo"
)

// RunManageConfessionChannel handles confess channel settings subcommands.
func RunManageConfessionChannel(s *discordgo.Session, e *discordgo.InteractionCreate, storage storage.Storage, sub *discordgo.ApplicationCommandInteractionDataOption) error {
	switch sub.Name {
	case "channel-set":
		channelID := sub.Options[0].ChannelValue(s).ID
		if err := storage.SetConfessChannel(e.GuildID, channelID); err != nil {
			return discordreply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: fmt.Sprintf("Failed to set confession channel: `%v`", err),
			})
		}
		return discordreply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Confession channel has been set to <#%s>.", channelID),
		})

	case "channel-show":
		channelID, err := storage.GetConfessChannel(e.GuildID)
		if err != nil {
			return discordreply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: fmt.Sprintf("Failed to get confession channel: `%v`", err),
			})
		}
		return discordreply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Current confession channel is <#%s>.", channelID),
		})

	case "channel-reset":
		if err := storage.RemoveConfessChannel(e.GuildID); err != nil {
			return discordreply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: fmt.Sprintf("Failed to remove confession channel: `%v`", err),
			})
		}
		return discordreply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "Confession channel has been removed.",
		})

	default:
		return discordreply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Unknown subcommand: %s", sub.Name),
		})
	}
}

// ConfessChannelOptions returns slash options for confess channel settings.
func ConfessChannelOptions() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionSubCommand,
			Name:        "channel-set",
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
			Name:        "channel-show",
			Description: "Show the current confession channel",
		},
		{
			Type:        discordgo.ApplicationCommandOptionSubCommand,
			Name:        "channel-reset",
			Description: "Remove the confession channel",
		},
	}
}
