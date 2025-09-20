package command

import (
	"server-domme/internal/core"

	"github.com/bwmarrin/discordgo"
)

type CommandUpdate struct{}

func (c *CommandUpdate) Name() string        { return "commands-update" }
func (c *CommandUpdate) Description() string { return "Re-register or update slash commands" }
func (c *CommandUpdate) Aliases() []string   { return []string{} }
func (c *CommandUpdate) Group() string       { return "core" }
func (c *CommandUpdate) Category() string    { return "⚙️ Settings" }
func (c *CommandUpdate) RequireAdmin() bool  { return true }
func (c *CommandUpdate) RequireDev() bool    { return true }

func (c *CommandUpdate) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "target",
				Description: "Type a command name to update, or 'all', use /help for a list",
				Required:    true,
			},
		},
	}
}

func (c *CommandUpdate) Run(ctx interface{}) error {
	context, ok := ctx.(*core.SlashInteractionContext)
	if !ok {
		return nil
	}

	target := context.Event.ApplicationCommandData().Options[0].StringValue()

	core.PublishSystemEvent(core.SystemEvent{
		Type:    core.SystemEventRefreshCommands,
		GuildID: context.Event.GuildID,
		Target:  target,
	})

	return core.RespondEphemeral(context.Session, context.Event, "Command update requested.")
}

func init() {
	core.RegisterCommand(
		core.ApplyMiddlewares(
			&CommandUpdate{},
			core.WithGroupAccessCheck(),
			core.WithGuildOnly(),
		),
	)
}
