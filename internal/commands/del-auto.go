package commands

import (
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

func init() {
	Register(&Command{
		Sort:           220,
		Name:           "del-auto",
		Description:    "Recurring deletion of messages older than set.",
		Category:       "🧹 Channel Cleanup",
		DCSlashHandler: deleteMessagesRecurringHandler,
		SlashOptions: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "older_than",
				Description: "Delete messages older than this duration (e.g., 10m, 1h, 1d, 1w)",
				Required:    true,
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

func deleteMessagesRecurringHandler(ctx *SlashContext) {
	s, i, storage := ctx.Session, ctx.InteractionCreate, ctx.Storage
	options := i.ApplicationCommandData().Options
	channelID, guildID := i.ChannelID, i.GuildID

	if !isAdmin(s, guildID, i.Member) {
		respondEphemeral(s, i, "You must be a server administrator to use this command.")
		return
	}

	if !checkBotPermissions(s, channelID) {
		respondEphemeral(s, i, "Missing permissions to delete messages in this channel.")
		return
	}

	var confirm, olderThan string
	var notifyAll bool
	for _, opt := range options {
		switch opt.Name {
		case "confirm":
			confirm = opt.StringValue()
		case "older_than":
			olderThan = opt.StringValue()
		case "notify_all":
			notifyAll = strings.ToLower(opt.StringValue()) == "true"
		}
	}

	if strings.ToLower(confirm) != "yes" {
		respondEphemeral(s, i, "Action not confirmed. Please type 'yes' to proceed.")
		return
	}

	dur, err := parseDuration(olderThan)
	if err != nil {
		respondEphemeral(s, i, "Invalid duration format. Use values like `10m`, `2h`, `1d`, `1w` etc.")
		return
	}

	ActiveDeletionsMu.Lock()
	if _, exists := ActiveDeletions[channelID]; exists {
		ActiveDeletionsMu.Unlock()
		respondEphemeral(s, i, "This channel is already undergoing recurring deletion.")
		return
	}
	stopChan := make(chan struct{})
	ActiveDeletions[channelID] = stopChan
	ActiveDeletionsMu.Unlock()

	err = storage.SetDeletionJob(guildID, channelID, "recurring", time.Now(), notifyAll, olderThan)
	if err != nil {
		stopDeletion(channelID)
		respondEphemeral(s, i, "Failed to schedule recurring deletion: "+err.Error())
		return
	}

	respondEphemeral(s, i, "Recurring message deletion started.\nMessages older than **"+dur.String()+"** will be removed.")

	if notifyAll {
		imgURL := "https://ichef.bbci.co.uk/images/ic/1376xn/p05cj1tt.jpg.webp"
		embed := &discordgo.MessageEmbed{
			Title:       "☢️ Recurring Nuke Detonation",
			Description: "This channel is now under a standing nuke order.\nAny messages older than `" + dur.String() + "` will be *systematically erased*.",
			Color:       embedColor,
			Image:       &discordgo.MessageEmbedImage{URL: imgURL},
			Footer:      &discordgo.MessageEmbedFooter{Text: "History has a half-life."},
			Timestamp:   time.Now().Format(time.RFC3339),
		}
		_, _ = s.ChannelMessageSendEmbed(channelID, embed)
	}

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		defer stopDeletion(channelID)

		for {
			select {
			case <-stopChan:
				return
			case <-ticker.C:
				start := time.Now().Add(-dur)
				now := time.Now()
				DeleteMessages(s, channelID, &now, &start, stopChan)
			}
		}
	}()
}
