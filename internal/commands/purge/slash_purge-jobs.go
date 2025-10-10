package purge

import (
	"server-domme/internal/core"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

type PurgeJobsCommand struct{}

func (c *PurgeJobsCommand) Name() string        { return "purge-jobs" }
func (c *PurgeJobsCommand) Description() string { return "List all active purge jobs" }
func (c *PurgeJobsCommand) Aliases() []string   { return []string{} }
func (c *PurgeJobsCommand) Group() string       { return "purge" }
func (c *PurgeJobsCommand) Category() string    { return "🧹 Cleanup" }
func (c *PurgeJobsCommand) UserPermissions() []int64 {
	return []int64{
		discordgo.PermissionAdministrator,
	}
}
func (c *PurgeJobsCommand) BotPermissions() []int64 {
	return []int64{}
}

func (c *PurgeJobsCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options:     []*discordgo.ApplicationCommandOption{},
	}
}

func (c *PurgeJobsCommand) Run(ctx interface{}) error {
	context, ok := ctx.(*core.SlashInteractionContext)
	if !ok {
		return nil
	}

	session := context.Session
	event := context.Event
	storage := context.Storage

	jobs, err := storage.GetDeletionJobsList(event.GuildID)
	if err != nil || len(jobs) == 0 {
		core.RespondEmbedEphemeral(session, event, &discordgo.MessageEmbed{
			Description: "No active purge jobs found.",
		})
		return nil
	}

	var sb strings.Builder
	sb.WriteString("☢️ **Active Message Purge Jobs**\n\n")
	for _, job := range jobs {
		sb.WriteString("<#" + job.ChannelID + ">\n")
		switch job.Mode {
		case "delayed":
			eta := time.Until(job.DelayUntil).Truncate(time.Second)
			if eta > 0 {
				sb.WriteString("One-time purge of all messages, runs in: `" + eta.String() + "`\n")
			} else {
				sb.WriteString("One-time purge of all messages, overdue: `" + (-eta).String() + "`\n")
			}
		case "recurring":
			sb.WriteString("Recurring purge of messages older than: `" + job.OlderThan + "`\n")
		default:
			sb.WriteString("Unknown mode: " + job.Mode + "\n")
		}
		sb.WriteString("\n")
	}
	sb.WriteString("Note: use `/purge-stop` in any listed channel to cancel the purge.")

	core.RespondEmbedEphemeral(session, event, &discordgo.MessageEmbed{Description: sb.String()})

	return nil
}

func init() {
	core.RegisterCommand(
		core.ApplyMiddlewares(
			&PurgeJobsCommand{},
			core.WithGroupAccessCheck(),
			core.WithGuildOnly(),
			core.WithUserPermissionCheck(),
			core.WithBotPermissionCheck(),
			core.WithCommandLogger(),
		),
	)
}
