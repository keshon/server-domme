package announce

import (
	"fmt"

	"github.com/keshon/server-domme/internal/discord/adapter"
	"github.com/keshon/server-domme/internal/discord/reply"
)

// ManageChannelOptions returns the settings options for announce channel
// management. Wired as the announce group under /settings.
func ManageChannelOptions() []adapter.SlashOption {
	return []adapter.SlashOption{
		{
			Type:        adapter.OptionSubCommand,
			Name:        "channel-set",
			Description: "Set the announcement channel",
			Options: []adapter.SlashOption{
				{
					Type:        adapter.OptionChannel,
					Name:        "channel",
					Description: "Pick a channel from this server",
					Required:    true,
				},
			},
		},
		{
			Type:        adapter.OptionSubCommand,
			Name:        "channel-show",
			Description: "Show the current announcement channel",
		},
		{
			Type:        adapter.OptionSubCommand,
			Name:        "channel-reset",
			Description: "Remove the announcement channel",
		},
	}
}

// RunManageChannel handles the announce settings subcommands.
func RunManageChannel(ctx *adapter.SlashInteractionContext, sub adapter.SlashArgument) error {
	switch sub.Name {
	case "channel-set":
		channelOpt, _ := sub.Option("channel")
		channelID := channelOpt.StringValue()
		if channelID == "" {
			return ctx.RespondEphemeral(&adapter.Embed{
				Description: "Invalid channel.",
				Color:       reply.EmbedColor,
			})
		}
		if err := ctx.Storage.SetAnnounceChannel(ctx.GuildID(), channelID); err != nil {
			return ctx.RespondEphemeral(&adapter.Embed{
				Description: fmt.Sprintf("Failed to set announcement channel: `%v`", err),
				Color:       reply.EmbedColor,
			})
		}
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: fmt.Sprintf("Announcement channel updated to <#%s>.", channelID),
			Color:       reply.EmbedColor,
		})

	case "channel-show":
		channelID, err := ctx.Storage.GetAnnounceChannel(ctx.GuildID())
		if err != nil || channelID == "" {
			return ctx.RespondEphemeral(&adapter.Embed{
				Description: "No announcement channel set.",
				Color:       reply.EmbedColor,
			})
		}
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: fmt.Sprintf("Current announcement channel is <#%s>.", channelID),
			Color:       reply.EmbedColor,
		})

	case "channel-reset":
		if err := ctx.Storage.SetAnnounceChannel(ctx.GuildID(), ""); err != nil {
			return ctx.RespondEphemeral(&adapter.Embed{
				Description: fmt.Sprintf("Failed to reset announcement channel: `%v`", err),
				Color:       reply.EmbedColor,
			})
		}
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: "Announcement channel has been reset.",
			Color:       reply.EmbedColor,
		})

	default:
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: "Unknown subcommand.",
			Color:       reply.EmbedColor,
		})
	}
}
