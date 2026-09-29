package help

import (
	"github.com/keshon/buildinfo"
	"github.com/keshon/server-domme/internal/discord/adapter"
	"github.com/keshon/server-domme/internal/discord/reply"
)

type Command struct{}

func (c *Command) Name() string        { return "help" }
func (c *Command) Description() string { return "Get a list of available commands" }
func (c *Command) Group() string       { return "core" }
func (c *Command) Category() string    { return "🕯️ Information" }
func (c *Command) UserPermissions() []int64 {
	return []int64{}
}

func (c *Command) SlashDefinition() *adapter.SlashCommand {
	return &adapter.SlashCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options: []adapter.SlashOption{
			{
				Type:        adapter.OptionSubCommand,
				Name:        "category",
				Description: "View commands grouped by category",
			},
			{
				Type:        adapter.OptionSubCommand,
				Name:        "group",
				Description: "View commands grouped by group",
			},
			{
				Type:        adapter.OptionSubCommand,
				Name:        "flat",
				Description: "View all commands as a flat list",
			},
		},
	}
}

func (c *Command) Run(ctx *adapter.SlashInteractionContext) error {
	if err := ctx.DeferEphemeral(); err != nil {
		ctx.AppLog.Error().Err(err).Msg("help_defer_failed")
		return err
	}

	sub, ok := ctx.FirstOption()
	if !ok {
		return ctx.FollowupEphemeral(&adapter.Embed{
			Description: "No subcommand provided. Use `category`, `group`, or `flat`.",
			Color:       reply.EmbedColor,
		})
	}

	var output string
	switch sub.Name {
	case "group":
		output = runHelpByGroup()
	case "flat":
		output = runHelpFlat()
	default:
		output = runHelpByCategory()
	}

	info := buildinfo.Get()

	return ctx.FollowupEphemeral(&adapter.Embed{
		Title:       info.Project + " Help",
		Description: output,
		Color:       reply.EmbedColor,
	})
}
