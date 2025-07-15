// /commands/nuke-now.go
package commands

import (
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

func init() {
	Register(&Command{
		Sort:           999,
		Name:           "nuke-now",
		Description:    "Immediately delete all messages in this channel (with optional delay)",
		Category:       "Moderation",
		DCSlashHandler: nukeNowHandler,
		SlashOptions: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "confirm",
				Description: "Type 'yes' to confirm the action",
				Required:    true,
			},
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "delay",
				Description: "Delay before deletion starts (e.g., 10m, 1h, 1d)",
				Required:    false,
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

func nukeNowHandler(ctx *SlashContext) {
	s, i, storage := ctx.Session, ctx.InteractionCreate, ctx.Storage
	options := i.ApplicationCommandData().Options
	channelID := i.ChannelID
	guildID := i.GuildID

	if !checkBotPermissions(s, channelID) {
		respondEphemeral(s, i, "I'm gagged — no permissions to delete messages here.")
		return
	}

	if !isAdmin(s, guildID, i.Member) {
		respondEphemeral(s, i, "No crown, no command. You're not an admin.")
		return
	}

	var confirm, delayStr string
	var silent bool
	for _, opt := range options {
		switch opt.Name {
		case "confirm":
			confirm = opt.StringValue()
		case "delay":
			delayStr = opt.StringValue()
		case "silent_mode":
			silent = opt.BoolValue()
		}
	}

	if strings.ToLower(confirm) != "yes" {
		respondEphemeral(s, i, "You must type 'yes' to confirm. Consent, darling.")
		return
	}

	if delayStr != "" {
		dur, err := time.ParseDuration(delayStr)
		if err != nil {
			respondEphemeral(s, i, "That delay format is tragic. Use 10m, 1h, 1d, etc.")
			return
		}
		delayUntil := time.Now().Add(dur)

		storage.SetNukeJob(guildID, channelID, "delayed", delayUntil, silent)
		if err != nil {
			respondEphemeral(s, i, "Error setting nuke job: "+err.Error())
			return
		}

		if !silent {
			respond(s, i, "Scheduled nuke in "+dur.String()+". Hide your sins.")
		}

		go func() {
			time.Sleep(dur)
			DeleteMessages(s, channelID, nil, nil, nil)
			storage.ClearNukeJob(guildID, channelID)
		}()
		return
	}

	if !silent {
		respond(s, i, "Immediate obliteration underway.")
	}
	go DeleteMessages(s, channelID, nil, nil, nil)
}
