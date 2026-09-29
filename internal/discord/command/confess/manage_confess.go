package confess

import (
	"fmt"

	"github.com/keshon/server-domme/internal/discord/adapter"
	"github.com/keshon/server-domme/internal/discord/reply"
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
		{
			Type:        adapter.OptionSubCommand,
			Name:        "ai-check-set",
			Description: "Enable or disable the AI pre-post guard",
			Options: []adapter.SlashOption{
				{
					Type:        adapter.OptionBoolean,
					Name:        "enabled",
					Description: "True checks confessions before posting, false posts as written",
					Required:    true,
				},
			},
		},
		{
			Type:        adapter.OptionSubCommand,
			Name:        "ai-check-show",
			Description: "Show whether the AI pre-post guard is on",
		},
		{
			Type:        adapter.OptionSubCommand,
			Name:        "ai-check-reset",
			Description: "Turn the AI pre-post guard off",
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
				Color:       reply.EmbedColor,
			})
		}
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: fmt.Sprintf("Confession channel has been set to <#%s>.", channelID),
			Color:       reply.EmbedColor,
		})

	case "channel-show":
		channelID, err := ctx.Storage.GetConfessChannel(ctx.GuildID())
		if err != nil {
			return ctx.RespondEphemeral(&adapter.Embed{
				Description: fmt.Sprintf("Failed to get confession channel: `%v`", err),
				Color:       reply.EmbedColor,
			})
		}
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: fmt.Sprintf("Current confession channel is <#%s>.", channelID),
			Color:       reply.EmbedColor,
		})

	case "channel-reset":
		if err := ctx.Storage.RemoveConfessChannel(ctx.GuildID()); err != nil {
			return ctx.RespondEphemeral(&adapter.Embed{
				Description: fmt.Sprintf("Failed to remove confession channel: `%v`", err),
				Color:       reply.EmbedColor,
			})
		}
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: "Confession channel has been removed.",
			Color:       reply.EmbedColor,
		})

	case "ai-check-set":
		opt, _ := sub.Option("enabled")
		enabled := opt.BoolValue()
		if err := ctx.Storage.SetConfessAICheck(ctx.GuildID(), enabled); err != nil {
			return ctx.RespondEphemeral(&adapter.Embed{
				Description: fmt.Sprintf("Failed to set AI guard: `%v`", err),
				Color:       reply.EmbedColor,
			})
		}
		if enabled {
			return ctx.RespondEphemeral(&adapter.Embed{
				Description: "AI guard is on. Risky confessions will be held back before posting.",
				Color:       reply.EmbedColor,
			})
		}
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: "AI guard is off. Confessions post exactly as written.",
			Color:       reply.EmbedColor,
		})

	case "ai-check-show":
		if ctx.Storage.GetConfessAICheck(ctx.GuildID()) {
			return ctx.RespondEphemeral(&adapter.Embed{
				Description: "AI guard is on.",
				Color:       reply.EmbedColor,
			})
		}
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: "AI guard is off.",
			Color:       reply.EmbedColor,
		})

	case "ai-check-reset":
		if err := ctx.Storage.SetConfessAICheck(ctx.GuildID(), false); err != nil {
			return ctx.RespondEphemeral(&adapter.Embed{
				Description: fmt.Sprintf("Failed to reset AI guard: `%v`", err),
				Color:       reply.EmbedColor,
			})
		}
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: "AI guard has been turned off.",
			Color:       reply.EmbedColor,
		})

	default:
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: fmt.Sprintf("Unknown subcommand: %s", sub.Name),
			Color:       reply.EmbedColor,
		})
	}
}
