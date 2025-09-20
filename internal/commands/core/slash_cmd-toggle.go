package core

import (
	"fmt"
	"log"
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
func (c *CommandsToggleCommand) RequireDev() bool    { return false }

func (c *CommandsToggleCommand) SlashDefinition() *discordgo.ApplicationCommand {
	groupChoices := []*discordgo.ApplicationCommandOptionChoice{}
	for _, group := range getUniqueGroups() {
		groupChoices = append(groupChoices, &discordgo.ApplicationCommandOptionChoice{
			Name:  group,
			Value: group,
		})
	}
	sort.Slice(groupChoices, func(i, j int) bool {
		return groupChoices[i].Name < groupChoices[j].Name
	})

	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "group",
				Description: "Choose command to toggle",
				Required:    true,
				Choices:     groupChoices,
			},
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "state",
				Description: "Enable or disable the command",
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
	member := event.Member

	data := context.Event.ApplicationCommandData()
	group := data.Options[0].StringValue()
	state := data.Options[1].StringValue()

	if group == "core" && state == "disable" {
		return core.RespondEphemeral(context.Session, context.Event, "You can't disable the `core` group. That's the spine of this whole circus.")
	}

	var err error
	if state == "disable" {
		err = context.Storage.DisableGroup(context.Event.GuildID, group)
		if err != nil {
			return core.RespondEphemeral(context.Session, context.Event, "Failed to disable the command.")
		}
		return core.RespondEphemeral(context.Session, context.Event, fmt.Sprintf("Command `%s` disabled.", group))
	}

	err = context.Storage.EnableGroup(context.Event.GuildID, group)
	if err != nil {
		return core.RespondEphemeral(context.Session, context.Event, "Failed to enable the command.")
	}

	err = core.LogCommand(session, storage, guildID, event.ChannelID, member.User.ID, member.User.Username, c.Name())
	if err != nil {
		log.Println("Failed to log:", err)
	}

	return core.RespondEphemeral(context.Session, context.Event, fmt.Sprintf("Command `%s` enabled.", group))
}

func init() {
	core.RegisterCommand(
		core.ApplyMiddlewares(
			&CommandsToggleCommand{},
			core.WithGroupAccessCheck(),
			core.WithGuildOnly(),
		),
	)
}
