package confess

import (
	"fmt"

	"github.com/keshon/server-domme/internal/discord/adapter"
)

// ManageChannelOptions returns the settings options for confess channel
// management. Wired as the confess group under /settings.
func ManageChannelOptions() []adapter.SlashOption {
	return []adapter.SlashOption{
		{
			Type:        adapter.OptionSubCommand,
			Name:        "channel-set",
			Description: "Set the confession channel",
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
			Description: "Show the current confession channel",
		},
		{
			Type:        adapter.OptionSubCommand,
			Name:        "channel-reset",
			Description: "Remove the confession channel",
		},
	}
}

// RunManageChannel handles the confess settings subcommands.
func RunManageChannel(ctx *adapter.SlashInteractionContext, sub adapter.SlashArgument) error {
	switch sub.Name {
	case "channel-set":
		channelOpt, _ := sub.Option("channel")
		channelID := channelOpt.StringValue()
		if err := ctx.Storage.SetConfessChannel(ctx.GuildID(), channelID); err != nil {
			return ctx.RespondEphemeral(&adapter.Embed{
				Description: fmt.Sprintf("Failed to set confession channel: `%v`", err),
			})
		}
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: fmt.Sprintf("Confession channel has been set to <#%s>.", channelID),
		})

	case "channel-show":
		channelID, err := ctx.Storage.GetConfessChannel(ctx.GuildID())
		if err != nil {
			return ctx.RespondEphemeral(&adapter.Embed{
				Description: fmt.Sprintf("Failed to get confession channel: `%v`", err),
			})
		}
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: fmt.Sprintf("Current confession channel is <#%s>.", channelID),
		})

	case "channel-reset":
		if err := ctx.Storage.RemoveConfessChannel(ctx.GuildID()); err != nil {
			return ctx.RespondEphemeral(&adapter.Embed{
				Description: fmt.Sprintf("Failed to remove confession channel: `%v`", err),
			})
		}
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: "Confession channel has been removed.",
		})

	default:
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: fmt.Sprintf("Unknown subcommand: %s", sub.Name),
		})
	}
}
