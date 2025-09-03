package command

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"

	"server-domme/internal/core"

	"github.com/bwmarrin/discordgo"
)

type AnnounceCommand struct{}

func (c *AnnounceCommand) Name() string        { return "Announce" }
func (c *AnnounceCommand) Description() string { return "Send a message to the announcement channel" }
func (c *AnnounceCommand) Aliases() []string   { return []string{} }

func (c *AnnounceCommand) Group() string    { return "announce" }
func (c *AnnounceCommand) Category() string { return "📢 Utilities" }

func (c *AnnounceCommand) RequireAdmin() bool { return true }
func (c *AnnounceCommand) RequireDev() bool   { return false }

func (c *AnnounceCommand) ContextDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name: c.Name(),
		Type: discordgo.MessageApplicationCommand,
	}
}

func (c *AnnounceCommand) Run(ctx interface{}) error {
	context, ok := ctx.(*core.MessageApplicationContext)
	if !ok {
		return fmt.Errorf("wrong context type (expected MessageApplicationContext)")
	}

	s := context.Session
	e := context.Event
	st := context.Storage

	guildID := e.GuildID
	channelID := e.ChannelID
	userID := e.Member.User.ID
	username := e.Member.User.Username

	if !core.IsAdministrator(s, guildID, e.Member) {
		core.RespondEphemeral(s, e, "You're not the boss of me.")
		return nil
	}

	err := s.InteractionRespond(e.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags: discordgo.MessageFlagsEphemeral,
		},
	})
	if err != nil {
		log.Println("Failed to send deferred response:", err)
		return nil
	}

	targetMsgID := e.ApplicationCommandData().TargetID
	msg, err := s.ChannelMessage(channelID, targetMsgID)
	if err != nil {
		editResponse(s, e, fmt.Sprintf("Couldn't fetch the message: `%v`", err))
		return nil
	}

	if msg.Author == nil || msg.Author.Bot {
		editResponse(s, e, "I won't announce bot babble or ghost messages.")
		return nil
	}
	if msg.Content == "" && len(msg.Embeds) == 0 && len(msg.Attachments) == 0 {
		editResponse(s, e, "Empty? I'm not announcing tumbleweeds.")
		return nil
	}

	announceChannelID, err := st.GetSpecialChannel(guildID, "announce")
	if err != nil || announceChannelID == "" {
		editResponse(s, e, "No announcement channel configured. Bother the admin.")
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

	message := &discordgo.MessageSend{
		Content: restoreMentions(s, guildID, msg.Content),
		Embeds:  msg.Embeds,
		Files:   files,
	}

	_, err = s.ChannelMessageSendComplex(announceChannelID, message)
	if err != nil {
		editResponse(s, e, fmt.Sprintf("Couldn't announce it: `%v`", err))
		return nil
	}

	editResponse(s, e, "Announced. Everyone's watching now.")

	err = core.LogCommand(s, st, guildID, channelID, userID, username, "announce")
	if err != nil {
		log.Println("Failed to log announce command:", err)
	}

	return nil
}

func editResponse(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	_, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: &content,
	})
	if err != nil {
		log.Println("Failed to edit response:", err)
	}
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

func init() {
	core.RegisterCommand(
		core.WithGroupAccessCheck()(
			core.WithGuildOnly(
				&AnnounceCommand{},
			),
		),
	)
}
