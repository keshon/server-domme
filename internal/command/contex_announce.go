package command

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

type AnnounceCommand struct{}

func (c *AnnounceCommand) Name() string        { return "announce (context)" }
func (c *AnnounceCommand) Description() string { return "Send a message to the announcement channel" }
func (c *AnnounceCommand) Aliases() []string   { return []string{} }

func (c *AnnounceCommand) Group() string    { return "announce" }
func (c *AnnounceCommand) Category() string { return "📢 Utilities" }

func (c *AnnounceCommand) RequireAdmin() bool { return true }
func (c *AnnounceCommand) RequireDev() bool   { return false }

func (c *AnnounceCommand) ContextType() discordgo.ApplicationCommandType {
	return discordgo.MessageApplicationCommand
}

func (c *AnnounceCommand) Run(ctx interface{}) error {
	slash, ok := ctx.(*SlashContext)
	if !ok {
		return fmt.Errorf("wrong context type")
	}

	session := slash.Session
	event := slash.Event
	storage := slash.Storage

	guildID := event.GuildID
	channelID := event.ChannelID

	userID := event.Member.User.ID
	username := event.Member.User.Username

	if !isAdministrator(session, guildID, event.Member) {
		respondEphemeral(session, event, "You're not the boss of me.")
		return nil
	}

	err := session.InteractionRespond(event.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags: discordgo.MessageFlagsEphemeral,
		},
	})
	if err != nil {
		log.Println("Failed to send deferred response:", err)
		return nil
	}

	target := event.ApplicationCommandData().TargetID
	msg, err := session.ChannelMessage(channelID, target)
	if err != nil {
		editResponse(session, event, fmt.Sprintf("Couldn't fetch the message: `%v`", err))
		return nil
	}

	if msg.Author == nil || msg.Author.Bot {
		editResponse(session, event, "I won't announce bot babble or ghost messages.")
		return nil
	}
	if msg.Content == "" && len(msg.Embeds) == 0 && len(msg.Attachments) == 0 {
		editResponse(session, event, "Empty? I'm not announcing tumbleweeds.")
		return nil
	}

	announceChannelID, err := storage.GetSpecialChannel(guildID, "announce")
	if err != nil || announceChannelID == "" {
		editResponse(session, event, "No announcement channel configured. Bother the admin.")
		return nil
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
		Content: restoreMentions(session, guildID, msg.Content),
		Files:   files,
		Embeds:  msg.Embeds,
	}

	_, err = session.ChannelMessageSendComplex(announceChannelID, msgSend)
	if err != nil {
		editResponse(session, event, fmt.Sprintf("Couldn't announce it: `%v`", err))
		return nil
	}

	editResponse(session, event, "Announced. Everyone's watching now.")

	err = logCommand(session, storage, guildID, channelID, userID, username, "announce")
	if err != nil {
		log.Println("Failed to log /announce:", err)
	}

	return nil
}

var mentionRegex = regexp.MustCompile(`@(\S+)`)

func restoreMentions(s *discordgo.Session, guildID, content string) string {
	members, err := s.GuildMembers(guildID, "", 1000)
	if err != nil {
		return content
	}

	userMap := map[string]string{}
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

	return mentionRegex.ReplaceAllStringFunc(content, func(m string) string {
		name := strings.TrimPrefix(m, "@")
		if id, ok := userMap[strings.ToLower(name)]; ok {
			return fmt.Sprintf("<@%s>", id)
		}
		return m
	})
}

func editResponse(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	_, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: &content,
	})
	if err != nil {
		log.Println("Failed to edit response:", err)
	}
}

func init() {
	Register(
		WithGroupAccessCheck()(
			WithGuildOnly(
				&AnnounceCommand{},
			),
		),
	)
}
