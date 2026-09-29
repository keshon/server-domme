package commands

import (
	"fmt"

	"github.com/keshon/server-domme/internal/discord/adapter"
	"github.com/keshon/server-domme/internal/discord/reply"
)

// RunEnable enables a command group for the guild.
func RunEnable(ctx *adapter.SlashInteractionContext, sub adapter.SlashArgument) error {
	groupOpt, _ := sub.Option("group")
	return runSetGroupState(ctx, groupOpt.StringValue(), true)
}

// RunDisable disables a command group for the guild.
func RunDisable(ctx *adapter.SlashInteractionContext, sub adapter.SlashArgument) error {
	groupOpt, _ := sub.Option("group")
	return runSetGroupState(ctx, groupOpt.StringValue(), false)
}

func runSetGroupState(ctx *adapter.SlashInteractionContext, group string, enabled bool) error {
	if group == "" {
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: "Missing required group option.",
			Color:       reply.EmbedColor,
		})
	}

	if group == "core" && !enabled {
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: "You can't disable the `core` group. It's the backbone of the discord.",
			Color:       reply.EmbedColor,
		})
	}

	embed := &adapter.Embed{
		Footer: "Use /settings commands status to check which commands are disabled.",
		Color:  reply.EmbedColor,
	}

	if enabled {
		if err := ctx.Storage.EnableGroup(ctx.GuildID(), group); err != nil {
			embed.Description = "Failed to enable the group."
			return ctx.RespondEphemeral(embed)
		}
		embed.Description = fmt.Sprintf("Command/group `%s` enabled.", group)
	} else {
		if err := ctx.Storage.DisableGroup(ctx.GuildID(), group); err != nil {
			embed.Description = "Failed to disable the group."
			return ctx.RespondEphemeral(embed)
		}
		embed.Description = fmt.Sprintf("Command/group `%s` disabled.", group)
	}

	if ctx.Syncer != nil {
		_ = ctx.Syncer.SyncGuildCommands(ctx.GuildID())
	}

	return ctx.RespondEphemeral(embed)
}
