package translate

import (
	"fmt"

	"github.com/keshon/server-domme/internal/discord/adapter"
	"github.com/keshon/server-domme/internal/discord/reply"
)

// ManageChannelOptions returns the settings options for translate channel
// management. Wired as the translate group under /settings.
func ManageChannelOptions() []adapter.SlashOption {
	return []adapter.SlashOption{
		{
			Type:        adapter.OptionSubCommand,
			Name:        "channel-add",
			Description: "Enable translation reactions in a channel",
			Options: []adapter.SlashOption{
				{
					Type:        adapter.OptionChannel,
					Name:        "channel",
					Description: "Select a channel to enable translation reactions",
					Required:    true,
				},
			},
		},
		{
			Type:        adapter.OptionSubCommand,
			Name:        "channel-remove",
			Description: "Disable translation reactions in a channel",
			Options: []adapter.SlashOption{
				{
					Type:        adapter.OptionChannel,
					Name:        "channel",
					Description: "Select a channel to remove from translation reactions",
					Required:    true,
				},
			},
		},
		{
			Type:        adapter.OptionSubCommand,
			Name:        "channel-list",
			Description: "List translation-enabled channels",
		},
		{
			Type:        adapter.OptionSubCommand,
			Name:        "channels-clear",
			Description: "Remove all translation-enabled channels",
		},
	}
}

// RunManageChannel handles the translate settings subcommands.
func RunManageChannel(ctx *adapter.SlashInteractionContext, sub adapter.SlashArgument) error {
	switch sub.Name {
	case "channel-add":
		return runAddChannel(ctx, sub)
	case "channel-remove":
		return runRemoveChannel(ctx, sub)
	case "channel-list":
		return runListChannels(ctx)
	case "channels-clear":
		return runResetChannels(ctx)
	default:
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: "Unknown subcommand provided.",
			Color:       reply.EmbedColor,
		})
	}
}

func runAddChannel(ctx *adapter.SlashInteractionContext, sub adapter.SlashArgument) error {
	channelOpt, _ := sub.Option("channel")
	channelID := channelOpt.StringValue()
	if err := ctx.Storage.AddTranslateChannel(ctx.GuildID(), channelID); err != nil {
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: fmt.Sprintf("Failed to add channel: `%v`", err),
			Color:       reply.EmbedColor,
		})
	}
	return ctx.RespondEphemeral(&adapter.Embed{
		Description: fmt.Sprintf("<#%s> added to translate reaction channels.", channelID),
		Color:       reply.EmbedColor,
	})
}

func runRemoveChannel(ctx *adapter.SlashInteractionContext, sub adapter.SlashArgument) error {
	channelOpt, _ := sub.Option("channel")
	channelID := channelOpt.StringValue()
	if err := ctx.Storage.RemoveTranslateChannel(ctx.GuildID(), channelID); err != nil {
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: fmt.Sprintf("Failed to remove channel: `%v`", err),
			Color:       reply.EmbedColor,
		})
	}
	return ctx.RespondEphemeral(&adapter.Embed{
		Description: fmt.Sprintf("<#%s> removed from translate reaction channels.", channelID),
		Color:       reply.EmbedColor,
	})
}

func runListChannels(ctx *adapter.SlashInteractionContext) error {
	channels, err := ctx.Storage.GetTranslateChannels(ctx.GuildID())
	if err != nil {
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: fmt.Sprintf("Failed to get channels: `%v`", err),
			Color:       reply.EmbedColor,
		})
	}

	if len(channels) == 0 {
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: "No channels currently configured for translation reactions.",
			Color:       reply.EmbedColor,
		})
	}

	desc := "Channels enabled for translation reactions:\n"
	for _, ch := range channels {
		desc += fmt.Sprintf("- <#%s>\n", ch)
	}

	return ctx.RespondEphemeral(&adapter.Embed{
		Title:       "🌐 Translate Channels",
		Description: desc,
		Color:       reply.EmbedColor,
	})
}

func runResetChannels(ctx *adapter.SlashInteractionContext) error {
	if err := ctx.Storage.ResetTranslateChannels(ctx.GuildID()); err != nil {
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: fmt.Sprintf("Failed to reset channels: `%v`", err),
			Color:       reply.EmbedColor,
		})
	}
	return ctx.RespondEphemeral(&adapter.Embed{
		Description: "All translate reaction channels have been reset.",
		Color:       reply.EmbedColor,
	})
}
