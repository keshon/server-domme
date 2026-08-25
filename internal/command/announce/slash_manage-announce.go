package announce

import (
	"fmt"

	"github.com/keshon/server-domme/internal/discord/reply"
	"github.com/keshon/server-domme/internal/storage"

	"github.com/bwmarrin/discordgo"
)

// RunManageAnnounceChannel handles announce channel settings subcommands.
func RunManageAnnounceChannel(s *discordgo.Session, e *discordgo.InteractionCreate, st storage.Storage, sub *discordgo.ApplicationCommandInteractionDataOption) error {
	switch sub.Name {
	case "channel-set":
		channel := sub.Options[0].ChannelValue(s)
		if channel == nil {
			return reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: "Invalid channel.",
			})
		}

		if err := st.SetAnnounceChannel(e.GuildID, channel.ID); err != nil {
			return reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: fmt.Sprintf("Failed to set announcement channel: `%v`", err),
			})
		}

		return reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Announcement channel updated to <#%s>.", channel.ID),
		})

	case "channel-show":
		channelID, err := st.GetAnnounceChannel(e.GuildID)
		if err != nil || channelID == "" {
			return reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: "No announcement channel set.",
			})
		}
		return reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Current announcement channel is <#%s>.", channelID),
		})

	case "channel-reset":
		if err := st.SetAnnounceChannel(e.GuildID, ""); err != nil {
			return reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: fmt.Sprintf("Failed to reset announcement channel: `%v`", err),
			})
		}

		return reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "Announcement channel has been reset.",
		})

	default:
		return reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "Unknown subcommand.",
		})
	}
}

// AnnounceChannelOptions returns slash options for announce channel settings.
func AnnounceChannelOptions() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionSubCommand,
			Name:        "channel-set",
			Description: "Set the announcement channel",
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
			Description: "Show the current announcement channel",
		},
		{
			Type:        discordgo.ApplicationCommandOptionSubCommand,
			Name:        "channel-reset",
			Description: "Remove the announcement channel",
		},
	}
}
