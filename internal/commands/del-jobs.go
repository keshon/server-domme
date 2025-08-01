// /commands/nuke-list.go
package commands

import (
	"log"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

func init() {
	Register(&Command{
		Sort:           200,
		Name:           "del-jobs",
		Description:    "List all active deletion jobs in this realm.",
		Category:       "🧹 Channel Cleanup",
		AdminOnly:      true,
		DCSlashHandler: deletionJobsSlashHandler,
	})
}

func deletionJobsSlashHandler(ctx *SlashContext) {
	s, i, storage := ctx.Session, ctx.InteractionCreate, ctx.Storage
	guildID := i.GuildID

	if !isAdministrator(s, guildID, i.Member) {
		respondEphemeral(s, i, "You must be a server administrator to use this command.")
		return
	}

	jobs, err := storage.GetDeletionJobsList(guildID)
	if err != nil || len(jobs) == 0 {
		respondEphemeral(s, i, "No active deletion jobs found in this server.")
		return
	}

	var builder strings.Builder
	builder.WriteString("☢️ **Active Message Deletion Jobs**\n\n")

	for _, job := range jobs {
		builder.WriteString("<#" + job.ChannelID + ">\n")

		switch job.Mode {
		case "delayed":
			eta := time.Until(job.DelayUntil).Truncate(time.Second)
			if eta > 0 {
				builder.WriteString("  One-time deletion of all messages, runs in: `" + eta.String() + "`\n")
			} else {
				builder.WriteString("  One-time deletion of all messages, overdue: `" + (-eta).String() + "`\n")
			}
		case "recurring":
			builder.WriteString("  Recurring deletion of messages older than: `" + job.OlderThan + "`\n")
		default:
			builder.WriteString("  Unknown mode: " + job.Mode + "\n")
		}

		builder.WriteString("\n")
	}

	builder.WriteString("\nNote: use `/del-stop` in any given channel from the list to cancel deletion job.")

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags:   1 << 6,
			Content: builder.String(),
		},
	})

	userID := i.Member.User.ID
	username := i.Member.User.Username
	err = logCommand(s, ctx.Storage, guildID, i.ChannelID, userID, username, "del-jobs")
	if err != nil {
		log.Println("Failed to log command:", err)
	}
}
