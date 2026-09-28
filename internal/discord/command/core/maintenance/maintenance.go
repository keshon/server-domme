package maintenance

import (
	"fmt"

	"github.com/keshon/server-domme/internal/discord/adapter"
	"github.com/keshon/server-domme/internal/discord/perm"
)

type Command struct{}

func (c *Command) Name() string        { return "maintenance" }
func (c *Command) Description() string { return "Bot maintenance commands" }
func (c *Command) Group() string       { return "core" }
func (c *Command) Category() string    { return "🛠️ Maintenance" }
func (c *Command) UserPermissions() []int64 {
	return []int64{perm.Administrator}
}

func (c *Command) SlashDefinition() *adapter.SlashCommand {
	return &adapter.SlashCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options: []adapter.SlashOption{
			{
				Type:        adapter.OptionSubCommand,
				Name:        "ping",
				Description: "Check bot latency",
			},
			{
				Type:        adapter.OptionSubCommand,
				Name:        "export-data",
				Description: "Export the current server database as JSON",
			},
			{
				Type:        adapter.OptionSubCommand,
				Name:        "status",
				Description: "Retrieve guild statistics",
			},
			{
				Type:        adapter.OptionSubCommand,
				Name:        "sync",
				Description: "Re-register slash commands",
			},
		},
	}
}

func (c *Command) Run(ctx *adapter.SlashInteractionContext) error {
	sub, ok := ctx.FirstOption()
	if !ok {
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: "No subcommand provided.",
		})
	}

	switch sub.Name {
	case "ping":
		return runPing(ctx)
	case "export-data":
		return runExportData(ctx)
	case "status":
		return runStatus(ctx)
	case "sync":
		return runSync(ctx)
	default:
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: fmt.Sprintf("Unknown subcommand: %s", sub.Name),
		})
	}
}
