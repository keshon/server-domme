package settings

import (
	"fmt"

	"github.com/bwmarrin/discordgo"
	"github.com/keshon/server-domme/internal/command/announce"
	"github.com/keshon/server-domme/internal/command/confess"
	"github.com/keshon/server-domme/internal/command/core/commands"
	"github.com/keshon/server-domme/internal/command/discipline"
	"github.com/keshon/server-domme/internal/command/media"
	"github.com/keshon/server-domme/internal/command/task"
	"github.com/keshon/server-domme/internal/command/translate"
	"github.com/keshon/server-domme/internal/discord/cmdadapter"
	"github.com/keshon/server-domme/internal/discord/reply"
	"github.com/keshon/server-domme/internal/storage"
)

type SettingsCommand struct{}

func (c *SettingsCommand) Name() string        { return "settings" }
func (c *SettingsCommand) Description() string { return "Server settings" }
func (c *SettingsCommand) Group() string       { return "core" }
func (c *SettingsCommand) Category() string    { return "⚙️ Settings" }
func (c *SettingsCommand) UserPermissions() []int64 {
	return []int64{discordgo.PermissionAdministrator}
}

func (c *SettingsCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionSubCommandGroup,
				Name:        "announce",
				Description: "Announcement settings",
				Options:     announce.AnnounceChannelOptions(),
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommandGroup,
				Name:        "confess",
				Description: "Confession settings",
				Options:     confess.ConfessChannelOptions(),
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommandGroup,
				Name:        "discipline",
				Description: "Discipline settings",
				Options:     discipline.DisciplineRolesOptions(),
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommandGroup,
				Name:        "media",
				Description: "Media settings",
				Options:     media.MediaSettingsOptions(),
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommandGroup,
				Name:        "task",
				Description: "Task settings",
				Options:     task.TaskSettingsOptions(),
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommandGroup,
				Name:        "translate",
				Description: "Translation settings",
				Options:     translate.TranslateChannelOptions(),
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommandGroup,
				Name:        "commands",
				Description: "Command group management",
				Options:     commands.CommandsSubcommandOptions(),
			},
		},
	}
}

func (c *SettingsCommand) Run(ctx interface{}) error {
	context, ok := ctx.(*cmdadapter.SlashInteractionContext)
	if !ok {
		return nil
	}

	s := context.Session
	e := context.Event
	st := context.Storage

	data := e.ApplicationCommandData()
	if len(data.Options) == 0 {
		return reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "No settings group provided.",
		})
	}

	group := data.Options[0]
	if len(group.Options) == 0 {
		return reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "No subcommand provided.",
		})
	}

	sub := group.Options[0]

	switch group.Name {
	case "announce":
		return announce.RunManageAnnounceChannel(s, e, *st, sub)
	case "confess":
		return confess.RunManageConfessionChannel(s, e, *st, sub)
	case "discipline":
		return discipline.RunManageDisciplineRoles(s, e, *st, sub)
	case "media":
		if err := reply.RespondDeferredEphemeral(s, e); err != nil {
			context.AppLog.Error().Err(err).Msg("settings_media_defer_failed")
			return err
		}
		return media.RunManageMediaSettings(s, e, *st, e.GuildID, sub)
	case "task":
		return task.RunManageTaskSettings(s, e, st, sub)
	case "translate":
		return translate.RunManageTranslateChannel(s, e, *st, sub)
	case "commands":
		return runCommandsSettings(s, e, *st, context.Syncer, sub)
	default:
		return reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Unknown settings group: %s", group.Name),
		})
	}
}

func runCommandsSettings(s *discordgo.Session, e *discordgo.InteractionCreate, st storage.Storage, syncer cmdadapter.CommandSyncer, sub *discordgo.ApplicationCommandInteractionDataOption) error {
	switch sub.Name {
	case "log":
		return commands.RunCmdLog(s, e, st)
	case "status":
		return commands.RunCmdStatus(s, e, st)
	case "enable":
		return commands.RunCmdEnable(s, e, st, syncer, sub)
	case "disable":
		return commands.RunCmdDisable(s, e, st, syncer, sub)
	default:
		return reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Unknown subcommand: %s", sub.Name),
		})
	}
}
