package command

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

type PurgeJobsCommand struct{}

func (c *PurgeJobsCommand) Name() string        { return "purge-jobs" }
func (c *PurgeJobsCommand) Description() string { return "List all active purge jobs" }
func (c *PurgeJobsCommand) Aliases() []string   { return []string{} }

func (c *PurgeJobsCommand) Group() string    { return "purge" }
func (c *PurgeJobsCommand) Category() string { return "🧹 Channel Cleanup" }

func (c *PurgeJobsCommand) RequireAdmin() bool { return true }
func (c *PurgeJobsCommand) RequireDev() bool   { return false }

func (c *PurgeJobsCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options:     []*discordgo.ApplicationCommandOption{},
	}
}

func (c *PurgeJobsCommand) Run(ctx interface{}) error {
	slash, ok := ctx.(*SlashContext)
	if !ok {
		return fmt.Errorf("wrong context type")
	}
	s := slash.Session
	i := slash.Event
	storage := slash.Storage

	jobs, err := storage.GetDeletionJobsList(i.GuildID)
	if err != nil || len(jobs) == 0 {
		respondEphemeral(s, i, "No active purge jobs found in this server.")
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

	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags:   discordgo.MessageFlagsEphemeral,
			Content: sb.String(),
		},
	})

	logErr := logCommand(s, storage, i.GuildID, i.ChannelID, i.Member.User.ID, i.Member.User.Username, "purge-jobs")
	if logErr != nil {
		log.Println("Failed to log command:", logErr)
	}
	return nil
}

func init() {
	Register(
		WithGroupAccessCheck()(
			WithGuildOnly(
				&PurgeJobsCommand{},
			),
		),
	)
}
