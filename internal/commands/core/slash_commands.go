package core

import (
	"fmt"
	"server-domme/internal/core"
	"server-domme/internal/storage"
	"sort"
	"strings"

	"github.com/bwmarrin/discordgo"
)

type CommandsCommand struct{}

func (c *CommandsCommand) Name() string        { return "commands" }
func (c *CommandsCommand) Description() string { return "Manage or inspect commands" }
func (c *CommandsCommand) Group() string       { return "core" }
func (c *CommandsCommand) Category() string    { return "⚙️ Settings" }
func (c *CommandsCommand) UserPermissions() []int64 {
	return []int64{discordgo.PermissionAdministrator}
}

const (
	discordMaxMessageLength = 2000
	codeLeftBlockWrapper    = "```md"
	codeRightBlockWrapper   = "```"
)

var maxContentLength = discordMaxMessageLength - len(codeLeftBlockWrapper) - len(codeRightBlockWrapper)

func (c *CommandsCommand) SlashDefinition() *discordgo.ApplicationCommand {
	groupChoices := []*discordgo.ApplicationCommandOptionChoice{}
	for _, g := range getUniqueGroups() {
		groupChoices = append(groupChoices, &discordgo.ApplicationCommandOptionChoice{Name: g, Value: g})
	}
	sort.Slice(groupChoices, func(i, j int) bool { return groupChoices[i].Name < groupChoices[j].Name })

	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "log",
				Description: "Review recent commands and their punishments",
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "status",
				Description: "Check which command groups are enabled or disabled",
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "toggle",
				Description: "Enable or disable a group of commands",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionString,
						Name:        "group",
						Description: "Choose command group to toggle",
						Required:    true,
						Choices:     groupChoices,
					},
					{
						Type:        discordgo.ApplicationCommandOptionString,
						Name:        "state",
						Description: "Enable or disable",
						Required:    true,
						Choices: []*discordgo.ApplicationCommandOptionChoice{
							{Name: "Enable", Value: "enable"},
							{Name: "Disable", Value: "disable"},
						},
					},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "update",
				Description: "Re-register or update slash commands",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionString,
						Name:        "target",
						Description: "Type a command name to update, or 'all', use /help for a list",
						Required:    true,
					},
				},
			},
		},
	}
}

func (c *CommandsCommand) Run(ctx interface{}) error {
	context, ok := ctx.(*core.SlashInteractionContext)
	if !ok {
		return nil
	}

	session := context.Session
	event := context.Event
	storage := context.Storage

	if len(event.ApplicationCommandData().Options) == 0 {
		return nil
	}

	sub := event.ApplicationCommandData().Options[0]

	switch sub.Name {
	case "log":
		return c.runCmdLog(session, event, *storage)
	case "status":
		return c.runCmdStatus(session, event, *storage)
	case "toggle":
		return c.runCmdToggle(session, event, *storage)
	case "update":
		return c.runCmdUpdate(session, event)
	default:
		return core.RespondEmbedEphemeral(session, event, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Unknown subcommand: %s", sub.Name),
		})
	}
}

func (c *CommandsCommand) runCmdLog(s *discordgo.Session, e *discordgo.InteractionCreate, storage storage.Storage) error {
	guildID := e.GuildID
	member := e.Member

	records, err := storage.GetCommandsHistory(guildID)
	if err != nil {
		return core.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Failed to fetch command logs: %v", err),
		})
	}
	if len(records) == 0 {
		return core.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "No command logs found.",
		})
	}

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("%-19s\t%-15s\t%-12s\t%s\n", "# Datetime", "# Username", "# Channel", "# Command"))

	for i := len(records) - 1; i >= 0; i-- {
		r := records[i]

		username := r.Username
		if r.Command == "confess" && !core.IsDeveloper(member.User.ID) {
			username = "###"
		}

		line := fmt.Sprintf("%-19s\t%-15s\t#%-12s\t/%s\n",
			r.Datetime.Format("2006-01-02 15:04:05"),
			username,
			r.ChannelName,
			r.Command,
		)

		if builder.Len()+len(line) > maxContentLength {
			break
		}
		builder.WriteString(line)
	}

	msg := codeLeftBlockWrapper + "\n" + builder.String() + codeRightBlockWrapper
	return core.RespondEphemeral(s, e, msg)
}

func (c *CommandsCommand) runCmdStatus(s *discordgo.Session, e *discordgo.InteractionCreate, storage storage.Storage) error {
	guildID := e.GuildID

	disabledGroups, _ := storage.GetDisabledGroups(guildID)
	disabledMap := make(map[string]bool)
	for _, g := range disabledGroups {
		disabledMap[g] = true
	}

	var enabled, disabled []string
	for _, group := range getUniqueGroups() {
		if disabledMap[group] {
			disabled = append(disabled, fmt.Sprintf("`%s`", group))
		} else {
			enabled = append(enabled, fmt.Sprintf("`%s`", group))
		}
	}

	if len(disabled) == 0 {
		disabled = []string{"_none_"}
	}
	if len(enabled) == 0 {
		enabled = []string{"_none_"}
	}

	embed := &discordgo.MessageEmbed{
		Title:       "Commands Status",
		Description: "Commands are grouped (e.g., purge, core, translate). Use `/help group` to view or `/commands toggle` to manage. Core group can't be disabled.",
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Disabled", Value: strings.Join(disabled, ", "), Inline: false},
			{Name: "Enabled", Value: strings.Join(enabled, ", "), Inline: false},
		},
	}
	return core.RespondEmbedEphemeral(s, e, embed)
}

func (c *CommandsCommand) runCmdToggle(s *discordgo.Session, e *discordgo.InteractionCreate, storage storage.Storage) error {
	data := e.ApplicationCommandData()

	subOptions := data.Options[0].Options

	var group, state string
	for _, opt := range subOptions {
		switch opt.Name {
		case "group":
			group = opt.StringValue()
		case "state":
			state = opt.StringValue()
		}
	}

	if group == "core" && state == "disable" {
		return core.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "You can't disable the `core` group. It's the backbone of the bot.",
		})
	}

	var err error
	embed := &discordgo.MessageEmbed{
		Footer: &discordgo.MessageEmbedFooter{Text: "Use /commands status to check which commands are disabled."},
	}

	if state == "disable" {
		err = storage.DisableGroup(e.GuildID, group)
		if err != nil {
			embed.Description = "Failed to disable the group."
			return core.RespondEmbedEphemeral(s, e, embed)
		}
		embed.Description = fmt.Sprintf("Command/group `%s` disabled.", group)
	} else {
		err = storage.EnableGroup(e.GuildID, group)
		if err != nil {
			embed.Description = "Failed to enable the group."
			return core.RespondEmbedEphemeral(s, e, embed)
		}
		embed.Description = fmt.Sprintf("Command/group `%s` enabled.", group)
	}

	return core.RespondEmbedEphemeral(s, e, embed)
}

func (c *CommandsCommand) runCmdUpdate(s *discordgo.Session, e *discordgo.InteractionCreate) error {
	subOptions := e.ApplicationCommandData().Options[0].Options

	var target string
	if len(subOptions) > 0 {
		target = subOptions[0].StringValue()
	}

	core.PublishSystemEvent(core.SystemEvent{
		Type:    core.SystemEventRefreshCommands,
		GuildID: e.GuildID,
		Target:  target,
	})

	return core.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
		Description: "Command update requested — it may take some time to apply.",
	})
}

func getUniqueGroups() []string {
	set := map[string]struct{}{}
	for _, cmd := range core.AllCommands() {
		group := cmd.Group()
		if group != "" {
			set[group] = struct{}{}
		}
	}
	var result []string
	for group := range set {
		result = append(result, group)
	}
	sort.Strings(result)
	return result
}

func init() {
	core.RegisterCommand(
		core.ApplyMiddlewares(
			&CommandsCommand{},
			core.WithGroupAccessCheck(),
			core.WithGuildOnly(),
			core.WithUserPermissionCheck(),
			core.WithCommandLogger(),
		),
	)
}
