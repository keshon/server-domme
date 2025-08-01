package commands

import (
	"fmt"
	"log"
	"strings"
)

const (
	discordMaxMessageLength = 2000
	codeLeftBlockWrapper    = "```md"
	codeRightBlockWrapper   = "```"
)

var maxContentLength = discordMaxMessageLength - len(codeLeftBlockWrapper) - len(codeRightBlockWrapper)

func init() {
	Register(&Command{
		Sort:           400,
		Name:           "log",
		Description:    "Review recent commands and their punishments.",
		Category:       "🏰 Court Administration",
		AdminOnly:      true,
		DCSlashHandler: logSlashHandler,
	})
}

func logSlashHandler(ctx *SlashContext) {
	s, i := ctx.Session, ctx.InteractionCreate
	guildID := i.GuildID

	if !isAdmin(s, i.GuildID, i.Member) {
		respondEphemeral(s, i, "You must be an Admin to use this command, darling.")
		return
	}

	records, err := ctx.Storage.GetCommands(guildID)
	if err != nil {
		respondEphemeral(s, i, fmt.Sprintf("Failed to fetch command logs: %v", err))
		return
	}

	if len(records) == 0 {
		respondEphemeral(s, i, "No command history found. Such a quiet guild, or lazy users.")
		return
	}

	var builder strings.Builder

	header := fmt.Sprintf("%-19s\t%-15s\t%-15s\t%s\n", "# Datetime", "# Username", "# Channel", "# Command")
	builder.WriteString(header)

	for idx := len(records) - 1; idx >= 0; idx-- {
		rec := records[idx]
		entry := fmt.Sprintf(
			"%-19s\t%-15s\t#%-14s\t%s\n",
			rec.Datetime.Format("2006-01-02 15:04:05"),
			rec.Username,
			rec.ChannelName,
			rec.Command,
		)

		if builder.Len()+len(entry) > maxContentLength {
			break
		}

		builder.WriteString(entry)
	}

	respondEphemeral(s, i, codeLeftBlockWrapper+"\n"+builder.String()+codeRightBlockWrapper)

	userID := i.Member.User.ID
	username := i.Member.User.Username
	err = logCommand(s, ctx.Storage, guildID, i.ChannelID, userID, username, "log")
	if err != nil {
		log.Println("Failed to log command:", err)
	}
}
