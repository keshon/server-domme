package commands

import (
	"fmt"
	"log"
	"strings"

	"github.com/bwmarrin/discordgo"
)

func init() {
	Register(&Command{
		Sort:           50,
		Name:           "confess",
		Description:    "Send an anonymous confession to the channel.",
		Category:       "🎭 Roleplay",
		DCSlashHandler: confessSlashHandler,
		SlashOptions: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "message",
				Description: "What do you need to confess?",
				Required:    true,
			},
		},
	})
}

func confessSlashHandler(ctx *SlashContext) {
	s, i, storage := ctx.Session, ctx.InteractionCreate, ctx.Storage
	options := i.ApplicationCommandData().Options

	var message string
	for _, opt := range options {
		if opt.Name == "message" && opt.Type == discordgo.ApplicationCommandOptionString {
			message = strings.TrimSpace(opt.StringValue())
			break
		}
	}

	if message == "" {
		respondEphemeral(s, i, "You can't confess silence. Try again.")
		return
	}

	confessChannelID, err := storage.GetSpecialChannel(i.GuildID, "confession")
	if err != nil || confessChannelID == "" {
		respondEphemeral(s, i, "No confession channel is configured. Ask a mod to set up a confession channel.")
		return
	}

	embed := &discordgo.MessageEmbed{
		Title:       "📢 Anonymous Confession",
		Description: fmt.Sprintf("> %s", message),
		Color:       embedColor,
	}

	_, err = s.ChannelMessageSendEmbed(confessChannelID, embed)
	if err != nil {
		respondEphemeral(s, i, fmt.Sprintf("Couldn’t send your confession: ```%v```", err))
		return
	}

	if i.ChannelID != confessChannelID {
		link := fmt.Sprintf("https://discord.com/channels/%s/%s", i.GuildID, confessChannelID)
		respondEphemeral(s, i, fmt.Sprintf("Your secret has been dropped into the void.\nSee it echo: %s", link))
	} else {
		respondEphemeral(s, i, "💌 Delivered. Nobody saw a thing.")
	}

	guildID := i.GuildID
	userID := i.Member.User.ID
	username := i.Member.User.Username
	err = logCommand(s, ctx.Storage, guildID, i.ChannelID, userID, username, "confess")
	if err != nil {
		log.Println("Failed to log command:", err)
	}
}
