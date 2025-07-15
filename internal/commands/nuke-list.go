// /commands/nuke-list.go
package commands

import (
	"strings"
	"time"
)

func init() {
	Register(&Command{
		Sort:           999,
		Name:           "nuke-list",
		Description:    "List all nuke jobs in this channel",
		Category:       "Moderation",
		DCSlashHandler: nukeListHandler,
	})
}

func nukeListHandler(ctx *SlashContext) {
	s, i, storage := ctx.Session, ctx.InteractionCreate, ctx.Storage
	guildID := i.GuildID

	jobs, err := storage.GetNukeJobsList(guildID)
	if err != nil {
		respondEphemeral(s, i, "No nuke active here. Drama averted.")
		return
	}

	var msg strings.Builder
	msg.WriteString("**Active Nukes:**\n")
	for _, job := range jobs {
		line := "<#" + job.ChannelID + "> - "
		switch job.Mode {
		case "delayed":
			eta := time.Until(job.DelayUntil).Truncate(time.Second)
			line += "🕐 Delayed (in " + eta.String() + ")"
		case "recurring":
			line += "♻️ Recurring (older than " + job.OlderThan + ")"
		}
		if job.Silent {
			line += " 🤫"
		}
		msg.WriteString(line + "\n")
	}

	respondEphemeral(s, i, msg.String())
}
