package commands

import (
	"fmt"
	"log"
	"strings"

	"github.com/bwmarrin/discordgo"
)

func init() {
	Register(&Command{
		Sort:           402,
		Name:           "log",
		Description:    "Show recent command log",
		Category:       "Administration",
		DCSlashHandler: logSlashHandler,
	})
}

const (
	discordMaxMessageLength = 2000
	codeBlockWrapper        = "```"
)

var maxContentLength = discordMaxMessageLength - len(codeBlockWrapper)*2

func logSlashHandler(ctx *SlashContext) {
	s, i := ctx.Session, ctx.Interaction
	guildID := i.GuildID
	member := i.Member
	hasAdmin := false

	guild, err := s.State.Guild(i.GuildID)
	if err != nil || guild == nil {
		guild, err = s.Guild(i.GuildID)
		if err != nil {
			return
		}
	}

	if i.Member.User.ID == guild.OwnerID {
		hasAdmin = true
	} else {
		for _, r := range member.Roles {
			role, _ := s.State.Role(i.GuildID, r)
			if role != nil && role.Permissions&discordgo.PermissionAdministrator != 0 {
				hasAdmin = true
				break
			}
		}
	}

	if !hasAdmin {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "You’re not wearing the crown, darling. Only Admins may play God here.",
				Flags:   1 << 6,
			},
		})
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

	header := fmt.Sprintf("%-19s\t%-15s\t%-15s\t%s\n", "Datetime", "Username", "Channel", "Command")
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

	content := codeBlockWrapper + "\n" + builder.String() + codeBlockWrapper

	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: content,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
	if err != nil {
		log.Println("Failed to send log message:", err)
	}

	userID := i.Member.User.ID
	username := i.Member.User.Username
	err = logCommand(s, ctx.Storage, guildID, i.ChannelID, userID, username, "log")
	if err != nil {
		log.Println("Failed to log command:", err)
	}
}

func respondEphemeral(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: content,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
}
