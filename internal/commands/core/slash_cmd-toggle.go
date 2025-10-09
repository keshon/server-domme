package core

import (
	"fmt"
	"server-domme/internal/core"
	"sort"

	"github.com/bwmarrin/discordgo"
)

type CommandsToggleCommand struct{}

func (c *CommandsToggleCommand) Name() string        { return "cmd-toggle" }
func (c *CommandsToggleCommand) Description() string { return "Enable or disable a group of commands" }
func (c *CommandsToggleCommand) Aliases() []string   { return []string{} }
func (c *CommandsToggleCommand) Group() string       { return "core" }
func (c *CommandsToggleCommand) Category() string    { return "⚙️ Settings" }
func (c *CommandsToggleCommand) RequireAdmin() bool  { return true }
func (c *CommandsToggleCommand) Permissions() []int64 {
	return []int64{
		discordgo.PermissionAdministrator,
	}
}
func (c *CommandsToggleCommand) BotPermissions() []int64 {
	return []int64{
		discordgo.PermissionAdministrator,
	}
}

func (c *CommandsToggleCommand) SlashDefinition() *discordgo.ApplicationCommand {
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
	}
}

func (c *CommandsToggleCommand) Run(ctx interface{}) error {
	context, ok := ctx.(*core.SlashInteractionContext)
	if !ok {
		return nil
	}

	session := context.Session
	event := context.Event
	storage := context.Storage

	guildID := event.GuildID
	data := event.ApplicationCommandData()
	group, state := data.Options[0].StringValue(), data.Options[1].StringValue()

	// Prevent disabling core group
	if group == "core" && state == "disable" {
		core.RespondEmbedEphemeral(session, event, &discordgo.MessageEmbed{
			Description: "You can't disable the `core` group. It's the backbone of the bot.",
		})
		return nil
	}

	// Enable or disable group
	var err error
	embed := &discordgo.MessageEmbed{
		Footer: &discordgo.MessageEmbedFooter{Text: "Use /cmd-status to check which commands are disabled."},
	}

	if state == "disable" {
		err = storage.DisableGroup(guildID, group)
		if err != nil {
			embed.Description = "Failed to disable the group."
			core.RespondEmbedEphemeral(session, event, embed)
			return nil
		}
		embed.Description = fmt.Sprintf("Command/group `%s` disabled.", group)
	} else {
		err = storage.EnableGroup(guildID, group)
		if err != nil {
			embed.Description = "Failed to enable the group."
			core.RespondEmbedEphemeral(session, event, embed)
			return nil
		}
		embed.Description = fmt.Sprintf("Command/group `%s` enabled.", group)
	}

	// Send response
	core.RespondEmbedEphemeral(session, event, embed)

	return nil
}

func init() {
	core.RegisterCommand(
		core.ApplyMiddlewares(
			&CommandsToggleCommand{},
			core.WithGroupAccessCheck(),
			core.WithGuildOnly(),
			core.WithAccessControl(),
			core.WithPermissionCheck(),
			core.WithBotPermissionCheck(),
			core.WithCommandLogger(),
		),
	)
}
