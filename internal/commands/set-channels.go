package commands

import (
	"fmt"
	"log"

	"github.com/bwmarrin/discordgo"
)

func init() {
	Register(&Command{
		Sort:           411,
		Name:           "set-channels",
		Description:    "Designate special-purpose channels.",
		Category:       "🏰 Court Administration",
		AdminOnly:      true,
		DCSlashHandler: setChannelHandler,
		SlashOptions: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "type",
				Description: "What kind of channel are you setting?",
				Required:    true,
				Choices: []*discordgo.ApplicationCommandOptionChoice{
					{Name: "Confession channel drop point", Value: "confession"},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionChannel,
				Name:        "channel",
				Description: "Pick a channel from this server",
				Required:    true,
			},
		},
	})
}

func setChannelHandler(ctx *SlashContext) {
	s, i, storage := ctx.Session, ctx.InteractionCreate, ctx.Storage
	options := i.ApplicationCommandData().Options

	if !isAdministrator(s, i.GuildID, i.Member) {
		respondEphemeral(s, i, "You must be an Admin to use this command, darling.")
		return
	}

	var kind, channelID string
	for _, opt := range options {
		switch opt.Name {
		case "type":
			kind = opt.StringValue()
		case "channel":
			channelID = opt.ChannelValue(s).ID
		}
	}

	if kind == "" || channelID == "" {
		respondEphemeral(s, i, "Missing required parameters. Don't make me repeat myself.")
		return
	}

	err := storage.SetSpecialChannel(i.GuildID, kind, channelID)
	if err != nil {
		respondEphemeral(s, i, fmt.Sprintf("Couldn’t save it: `%s`", err.Error()))
		return
	}

	var confirmation string
	switch kind {
	case "confession":
		confirmation = "💬 Confession channel updated. May secrets drip in silence."
	default:
		confirmation = fmt.Sprintf("✅ Channel for `%s` set.", kind)
	}

	respondEphemeral(s, i, confirmation)

	err = logCommand(s, storage, i.GuildID, i.ChannelID, i.Member.User.ID, i.Member.User.Username, "set-channels")
	if err != nil {
		log.Println("Failed to log command:", err)
	}
}
