package settings

import (
	"fmt"

	"github.com/keshon/server-domme/internal/discord/adapter"
	"github.com/keshon/server-domme/internal/discord/command/announce"
	"github.com/keshon/server-domme/internal/discord/command/confess"
	"github.com/keshon/server-domme/internal/discord/command/core/commands"
	"github.com/keshon/server-domme/internal/discord/command/discipline"
	"github.com/keshon/server-domme/internal/discord/command/knowledge"
	"github.com/keshon/server-domme/internal/discord/command/media"
	"github.com/keshon/server-domme/internal/discord/command/task"
	"github.com/keshon/server-domme/internal/discord/command/translate"
	"github.com/keshon/server-domme/internal/discord/perm"
	"github.com/keshon/server-domme/internal/discord/reply"
)

type SettingsCommand struct{}

func (c *SettingsCommand) Name() string        { return "settings" }
func (c *SettingsCommand) Description() string { return "Server settings" }
func (c *SettingsCommand) Group() string       { return "core" }
func (c *SettingsCommand) Category() string    { return "⚙️ Settings" }
func (c *SettingsCommand) UserPermissions() []int64 {
	return []int64{perm.Administrator}
}

func (c *SettingsCommand) SlashDefinition() *adapter.SlashCommand {
	return &adapter.SlashCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options: []adapter.SlashOption{
			{
				Type:        adapter.OptionSubCommandGroup,
				Name:        "announce",
				Description: "Announcement settings",
				Options:     announce.ManageChannelOptions(),
			},
			{
				Type:        adapter.OptionSubCommandGroup,
				Name:        "confess",
				Description: "Confession settings",
				Options:     confess.ManageChannelOptions(),
			},
			{
				Type:        adapter.OptionSubCommandGroup,
				Name:        "discipline",
				Description: "Discipline settings",
				Options:     discipline.ManageRolesOptions(),
			},
			{
				Type:        adapter.OptionSubCommandGroup,
				Name:        "knowledge",
				Description: "Knowledge base settings",
				Options:     knowledge.ManageOptions(),
			},
			{
				Type:        adapter.OptionSubCommandGroup,
				Name:        "media",
				Description: "Media settings",
				Options:     media.ManageSettingsOptions(),
			},
			{
				Type:        adapter.OptionSubCommandGroup,
				Name:        "task",
				Description: "Task settings",
				Options:     task.ManageSettingsOptions(),
			},
			{
				Type:        adapter.OptionSubCommandGroup,
				Name:        "translate",
				Description: "Translation settings",
				Options:     translate.ManageChannelOptions(),
			},
			{
				Type:        adapter.OptionSubCommandGroup,
				Name:        "commands",
				Description: "Command group management",
				Options:     commands.SubcommandOptions(),
			},
		},
	}
}

func (c *SettingsCommand) Run(ctx *adapter.SlashInteractionContext) error {
	group, ok := ctx.FirstOption()
	if !ok {
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: "No settings group provided.",
			Color:       reply.EmbedColor,
		})
	}

	sub, ok := group.First()
	if !ok {
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: "No subcommand provided.",
			Color:       reply.EmbedColor,
		})
	}

	switch group.Name {
	case "announce":
		return announce.RunManageChannel(ctx, sub)
	case "confess":
		return confess.RunManageChannel(ctx, sub)
	case "task":
		return task.RunManageSettings(ctx, sub)
	case "discipline":
		return discipline.RunManageRoles(ctx, sub)
	case "knowledge":
		return knowledge.RunManage(ctx, sub)
	case "media":
		if err := ctx.DeferEphemeral(); err != nil {
			ctx.AppLog.Error().Err(err).Msg("settings_media_defer_failed")
			return err
		}
		return media.RunManageSettings(ctx, sub)
	case "translate":
		return translate.RunManageChannel(ctx, sub)
	case "commands":
		return runCommandsSettings(ctx, sub)
	default:
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: fmt.Sprintf("Unknown settings group: %s.", group.Name),
			Color:       reply.EmbedColor,
		})
	}
}

func runCommandsSettings(ctx *adapter.SlashInteractionContext, sub adapter.SlashArgument) error {
	switch sub.Name {
	case "log":
		return commands.RunLog(ctx)
	case "status":
		return commands.RunStatus(ctx)
	case "enable":
		return commands.RunEnable(ctx, sub)
	case "disable":
		return commands.RunDisable(ctx, sub)
	default:
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: fmt.Sprintf("Unknown subcommand: %s.", sub.Name),
			Color:       reply.EmbedColor,
		})
	}
}
