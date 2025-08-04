package commands

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/bwmarrin/discordgo"
)

func init() {
	Register(&Command{
		Sort:             80,
		Name:             "Announce",
		Description:      "Send a message to the announcement channel",
		Category:         "📢 Utilities",
		ContextType:      discordgo.MessageApplicationCommand,
		DCContextHandler: announceMessageHandler,
	})
}
func announceMessageHandler(ctx *SlashContext) {
	s, i, storage := ctx.Session, ctx.InteractionCreate, ctx.Storage
	userID := i.Member.User.ID
	username := i.Member.User.Username
	guildID := i.GuildID
	channelID := i.ChannelID

	target := i.ApplicationCommandData().TargetID
	msg, err := s.ChannelMessage(channelID, target)
	if err != nil {
		respondEphemeral(s, i, fmt.Sprintf("Couldn’t fetch the message: `%v`", err))
		return
	}

	if msg.Author == nil || msg.Author.Bot {
		respondEphemeral(s, i, "I won’t announce bot babble or ghost messages.")
		return
	}

	if msg.Content == "" && len(msg.Embeds) == 0 && len(msg.Attachments) == 0 {
		respondEphemeral(s, i, "Empty? I’m not announcing tumbleweeds.")
		return
	}

	announceChannelID, err := storage.GetSpecialChannel(guildID, "announce")
	if err != nil {
		respondEphemeral(s, i, "No announcement channel configured. Bother the admin.")
		return
	}

	var files []*discordgo.File
	for _, att := range msg.Attachments {
		resp, err := http.Get(att.URL)
		if err != nil {
			log.Printf("Failed to download attachment %s: %v", att.URL, err)
			continue
		}
		defer resp.Body.Close()

		data, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Printf("Failed to read attachment %s: %v", att.URL, err)
			continue
		}

		files = append(files, &discordgo.File{
			Name:   att.Filename,
			Reader: bytes.NewReader(data),
		})
	}

	msgSend := &discordgo.MessageSend{
		Content: restoreMentions(s, guildID, msg.Content, msg.Mentions),
		Files:   files,
		Embeds:  msg.Embeds,
	}

	_, err = s.ChannelMessageSendComplex(announceChannelID, msgSend)
	if err != nil {
		respondEphemeral(s, i, fmt.Sprintf("Couldn’t announce it: `%v`", err))
		return
	}

	respondEphemeral(s, i, "Announced. Everyone’s watching now.")

	err = logCommand(s, storage, guildID, channelID, userID, username, "announce")
	if err != nil {
		log.Println("Failed to log command:", err)
	}
}

func restoreMentions(s *discordgo.Session, guildID string, content string, mentions []*discordgo.User) string {
	for _, u := range mentions {
		member, err := s.GuildMember(guildID, u.ID)
		if err != nil {
			continue
		}

		displayNames := []string{"@" + u.Username}

		if member.Nick != "" {
			displayNames = append(displayNames, "@"+member.Nick)
		}

		if u.GlobalName != "" && u.GlobalName != u.Username {
			displayNames = append(displayNames, "@"+u.GlobalName)
		}

		for _, name := range displayNames {
			if strings.Contains(content, name) {
				content = strings.ReplaceAll(content, name, fmt.Sprintf("<@%s>", u.ID))
			}
		}
	}
	return content
}
