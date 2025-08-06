package command

import (
	"fmt"
	"sort"

	"github.com/bwmarrin/discordgo"
)

type CommandsToggleCommand struct{}

func (c *CommandsToggleCommand) Name() string        { return "commands-toggle" }
func (c *CommandsToggleCommand) Description() string { return "Enable or disable a group of commands" }
func (c *CommandsToggleCommand) Aliases() []string   { return []string{} }
func (c *CommandsToggleCommand) Group() string       { return "core" }
func (c *CommandsToggleCommand) Category() string    { return "⚙️ Settings" }
func (c *CommandsToggleCommand) RequireAdmin() bool  { return true }
func (c *CommandsToggleCommand) RequireDev() bool    { return false }

func (c *CommandsToggleCommand) SlashDefinition() *discordgo.ApplicationCommand {
	groupChoices := []*discordgo.ApplicationCommandOptionChoice{}
	seen := map[string]struct{}{}
	for _, cmd := range All() {
		group := cmd.Group()
		if group == "" {
			continue
		}
		if _, ok := seen[group]; ok {
			continue
		}
		groupChoices = append(groupChoices, &discordgo.ApplicationCommandOptionChoice{
			Name:  group,
			Value: group,
		})
		seen[group] = struct{}{}
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
				Description: "Choose command group",
				Required:    true,
				Choices:     groupChoices,
			},
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "state",
				Description: "Enable or disable the group",
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
	slash, ok := ctx.(*SlashContext)
	if !ok {
		return fmt.Errorf("invalid context")
	}

	data := slash.Event.ApplicationCommandData()
	group := data.Options[0].StringValue()
	state := data.Options[1].StringValue()

	var err error
	if state == "disable" {
		err = slash.Storage.DisableGroup(slash.Event.GuildID, group)
		if err != nil {
			return respondEphemeral(slash.Session, slash.Event, "Failed to disable the group.")
		}
		return respondEphemeral(slash.Session, slash.Event, fmt.Sprintf("Group `%s` disabled.", group))
	}

	err = slash.Storage.EnableGroup(slash.Event.GuildID, group)
	if err != nil {
		return respondEphemeral(slash.Session, slash.Event, "Failed to enable the group.")
	}
	return respondEphemeral(slash.Session, slash.Event, fmt.Sprintf("Group `%s` enabled.", group))
}

func init() {
	Register(WithGuildOnly(WithGroupAccessCheck()(&CommandsToggleCommand{})))
}
