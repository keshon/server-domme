package commands

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"

	"github.com/bwmarrin/discordgo"
)

func init() {
	Register(&Command{
		Sort:             80,
		Name:             "Announce",
		Description:      "Send a message to the announcement channel",
		AdminOnly:        true,
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

	if !isAdministrator(s, guildID, i.Member) {
		respondEphemeral(s, i, "You're not the boss of me.")
		return
	}

	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags: discordgo.MessageFlagsEphemeral,
		},
	})

	if err != nil {
		log.Println("Failed to send deferred response:", err)
		return
	}

	target := i.ApplicationCommandData().TargetID
	msg, err := s.ChannelMessage(channelID, target)
	if err != nil {
		editResponse(s, i, fmt.Sprintf("Couldn't fetch the message: `%v`", err))
		return
	}

	if msg.Author == nil || msg.Author.Bot {
		editResponse(s, i, "I won't announce bot babble or ghost messages.")
		return
	}

	if msg.Content == "" && len(msg.Embeds) == 0 && len(msg.Attachments) == 0 {
		editResponse(s, i, "Empty? I'm not announcing tumbleweeds.")
		return
	}

	announceChannelID, err := storage.GetSpecialChannel(guildID, "announce")
	if err != nil {
		editResponse(s, i, "No announcement channel configured. Bother the admin.")
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
		Content: restoreMentions(s, guildID, msg.Content),
		Files:   files,
	}

	if len(msg.Embeds) > 0 {
		msgSend.Embeds = msg.Embeds
	}

	_, err = s.ChannelMessageSendComplex(announceChannelID, msgSend)
	if err != nil {
		editResponse(s, i, fmt.Sprintf("Couldn't announce it: `%v`", err))
		return
	}

	editResponse(s, i, "Announced. Everyone's watching now.")

	err = logCommand(s, storage, guildID, channelID, userID, username, "announce")
	if err != nil {
		log.Println("Failed to log command:", err)
	}
}

var mentionRegex = regexp.MustCompile(`@(\S+)`)

func restoreMentions(s *discordgo.Session, guildID, content string) string {
	members, err := s.GuildMembers(guildID, "", 1000)
	if err != nil {
		return content
	}

	userMap := make(map[string]string)

	for _, m := range members {
		u := m.User
		userMap[strings.ToLower(u.Username)] = u.ID
		if m.Nick != "" {
			userMap[strings.ToLower(m.Nick)] = u.ID
		}
		if u.GlobalName != "" {
			userMap[strings.ToLower(u.GlobalName)] = u.ID
		}
	}

	content = mentionRegex.ReplaceAllStringFunc(content, func(m string) string {
		name := strings.TrimPrefix(m, "@")
		if id, ok := userMap[strings.ToLower(name)]; ok {
			return fmt.Sprintf("<@%s>", id)
		}
		return m
	})

	return content
}

func editResponse(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	_, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: &content,
	})
	if err != nil {
		log.Println("Failed to edit response:", err)
	}
}
