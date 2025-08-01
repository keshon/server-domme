package commands

import (
	"log"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

func init() {
	Register(&Command{
		Sort:           210,
		Name:           "del-now",
		Description:    "Wipe this channel clean, no mercy shown.",
		Category:       "🧹 Channel Cleanup",
		AdminOnly:      true,
		DCSlashHandler: deleteNowSlashHandler,
		SlashOptions: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "delay",
				Description: "Delay before deletion starts",
				Required:    true,
				Choices: []*discordgo.ApplicationCommandOptionChoice{
					{Name: "Now (no delay)", Value: "0s"},
					{Name: "10 minutes", Value: "10m"},
					{Name: "30 minutes", Value: "30m"},
					{Name: "1 hour", Value: "1h"},
					{Name: "6 hours", Value: "6h"},
					{Name: "1 day", Value: "24h"},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "notify_all",
				Description: "Post a notification message",
				Required:    true,
				Choices: []*discordgo.ApplicationCommandOptionChoice{
					{Name: "Yes (default)", Value: "true"},
					{Name: "No", Value: "false"},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "confirm",
				Description: "Type 'yes' to confirm the action",
				Required:    true,
			},
		},
	})
}

func deleteNowSlashHandler(ctx *SlashContext) {
	s, i, storage := ctx.Session, ctx.InteractionCreate, ctx.Storage
	options := i.ApplicationCommandData().Options
	channelID, guildID := i.ChannelID, i.GuildID

	if !isAdministrator(s, guildID, i.Member) {
		respondEphemeral(s, i, "You must be a server administrator to use this command.")
		return
	}

	if !checkBotPermissions(s, channelID) {
		respondEphemeral(s, i, "Missing permissions to delete messages in this channel.")
		return
	}

	var confirm, delayStr string
	var notifyAll bool
	for _, opt := range options {
		switch opt.Name {
		case "confirm":
			confirm = opt.StringValue()
		case "delay":
			delayStr = opt.StringValue()
		case "notify_all":
			notifyAll = strings.ToLower(opt.StringValue()) != "true"
		}
	}

	if strings.ToLower(confirm) != "yes" {
		respondEphemeral(s, i, "Action not confirmed. Please type 'yes' to proceed.")
		return
	}

	if delayStr == "0s" {
		delayStr = "10s" // Let 'em stew in fear
	}

	dur, err := parseDuration(delayStr)
	if err != nil {
		respondEphemeral(s, i, "Invalid delay format. Use formats like `10m`, `1h`, `1d`.")
		return
	}

	delayUntil := time.Now().Add(dur)
	if err := storage.SetDeletionJob(guildID, channelID, "delayed", delayUntil, notifyAll); err != nil {
		respondEphemeral(s, i, "Failed to schedule deletion: "+err.Error())
		return
	}

	respondEphemeral(s, i, "Message deletion scheduled.\nThis channel will be purged in **"+dur.String()+"**.")

	if notifyAll {
		embed := &discordgo.MessageEmbed{
			Title:       "☢️ Upcoming Nuke Detonation",
			Description: "Countdown initiated.\nAll messages in this channel will be *obliterated* in `" + dur.String() + "`.\nPrepare for impact.",
			Color:       embedColor,
			Image:       &discordgo.MessageEmbedImage{URL: "https://c.tenor.com/qDvLEFO5bAkAAAAd/tenor.gif"},
			Footer:      &discordgo.MessageEmbedFooter{Text: "May your sins be incinerated."},
			Timestamp:   time.Now().Format(time.RFC3339),
		}
		_, _ = s.ChannelMessageSendEmbed(channelID, embed)
	}

	go func() {
		time.Sleep(dur)

		stopChan := make(chan struct{})
		ActiveDeletionsMu.Lock()
		ActiveDeletions[channelID] = stopChan
		ActiveDeletionsMu.Unlock()

		DeleteMessages(s, channelID, nil, nil, stopChan)

		ActiveDeletionsMu.Lock()
		delete(ActiveDeletions, channelID)
		ActiveDeletionsMu.Unlock()

		_ = storage.ClearDeletionJob(guildID, channelID)
	}()

	userID := i.Member.User.ID
	username := i.Member.User.Username
	err = logCommand(s, ctx.Storage, guildID, i.ChannelID, userID, username, "del-now")
	if err != nil {
		log.Println("Failed to log command:", err)
	}
}
