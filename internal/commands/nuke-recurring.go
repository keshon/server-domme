// /commands/nuke-recurring.go
package commands

import (
	"time"

	"github.com/bwmarrin/discordgo"
)

func init() {

	Register(&Command{
		Sort:           1000,
		Name:           "nuke-recurring",
		Description:    "Continuously delete older messages (e.g., older than 2h)",
		Category:       "Moderation",
		DCSlashHandler: nukeRecurringHandler,
		SlashOptions: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "older_than",
				Description: "Delete messages older than this duration (e.g., 10m, 1h, 1d)",
				Required:    true,
			},
			{
				Type:        discordgo.ApplicationCommandOptionBoolean,
				Name:        "silent_mode",
				Description: "Don't post a notification message",
				Required:    false,
			},
		},
	})
}

func nukeRecurringHandler(ctx *SlashContext) {
	s, i, storage := ctx.Session, ctx.InteractionCreate, ctx.Storage
	options := i.ApplicationCommandData().Options
	channelID, guildID := i.ChannelID, i.GuildID

	var olderThan string
	var silent bool
	for _, opt := range options {
		switch opt.Name {
		case "older_than":
			olderThan = opt.StringValue()
		case "silent_mode":
			silent = opt.BoolValue()
		}
	}

	dur, err := time.ParseDuration(olderThan)
	if err != nil {
		respondEphemeral(s, i, "Bad format. Try 10m, 2h, 1d, etc.")
		return
	}

	ActiveDeletionsMu.Lock()
	if _, exists := ActiveDeletions[channelID]; exists {
		ActiveDeletionsMu.Unlock()
		respondEphemeral(s, i, "Already nuking, sweetie.")
		return
	}
	stopChan := make(chan struct{})
	ActiveDeletions[channelID] = stopChan
	ActiveDeletionsMu.Unlock()

	// Save to storage
	err = storage.SetNukeJob(guildID, channelID, "recurring", time.Now().Add(dur), silent, olderThan)

	if err != nil {
		respondEphemeral(s, i, "Error setting nuke job: "+err.Error())
		return
	}

	if !silent {
		respond(s, i, "Recurring nuke started. Cleansing every 30s.")
	}

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stopChan:
				return
			case <-ticker.C:
				start := time.Now().Add(-dur)
				now := time.Now()
				DeleteMessages(s, channelID, &start, &now, stopChan)
			}
		}
	}()
}
