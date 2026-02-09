package announce

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"server-domme/internal/discord"
	"server-domme/internal/middleware"

	"server-domme/internal/command"
	"strings"

	"github.com/bwmarrin/discordgo"
)

type AnnounceContextCommand struct{}

func (c *AnnounceContextCommand) Name() string { return "Announce (context command)" }
func (c *AnnounceContextCommand) Description() string {
	return "Send a message on bot's behalf"
}
func (c *AnnounceContextCommand) Group() string    { return "announce" }
func (c *AnnounceContextCommand) Category() string { return "📢 Utilities" }
func (c *AnnounceContextCommand) UserPermissions() []int64 {
	return []int64{
		discordgo.PermissionAdministrator,
	}
}

func (c *AnnounceContextCommand) ContextDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name: c.Name(),
		Type: discordgo.MessageApplicationCommand,
	}
}

func (c *AnnounceContextCommand) Run(ctx interface{}) error {
	context, ok := ctx.(*command.MessageApplicationCommandContext)
	if !ok {
		return nil
	}

	s := context.Session
	e := context.Event
	storage := context.Storage

	guildID := e.GuildID
	channelID := e.ChannelID

	// Deferred ephemeral reply
	if err := discord.RespondDeferredEphemeral(s, e); err != nil {
		log.Println("Failed to send deferred response:", err)
		return nil
	}

	// Fetch the target message
	targetID := e.ApplicationCommandData().TargetID
	msg, err := s.ChannelMessage(channelID, targetID)
	if err != nil {
		discord.EditResponse(s, e, fmt.Sprintf("Couldn't fetch the message: `%v`", err))
		return nil
	}

	// Validation
	if msg.Author == nil {
		discord.EditResponse(s, e, "I won't announce ghost messages.")
		return nil
	}
	if msg.Content == "" && len(msg.Embeds) == 0 && len(msg.Attachments) == 0 {
		discord.EditResponse(s, e, "Empty? I'm not announcing tumbleweeds.")
		return nil
	}

	// Fetch announcement channel
	announceChannelID, err := storage.GetAnnounceChannel(guildID)
	if err != nil || announceChannelID == "" {
		discord.EditResponse(s, e, "No announcement channel configured. Bother the admin.")
		return nil
	}

	// Download attachments
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

	// Send announcement
	message := &discordgo.MessageSend{
		Content: restoreMentions(s, guildID, msg.Content),
		Embeds:  msg.Embeds,
		Files:   files,
	}

	if _, err := s.ChannelMessageSendComplex(announceChannelID, message); err != nil {
		discord.EditResponse(s, e, fmt.Sprintf("Couldn't announce it: `%v`", err))
		return nil
	}

	discord.EditResponse(s, e, "Announced. Everyone’s watching now.")
	return nil
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

	return mentionRegex.ReplaceAllStringFunc(content, func(m string) string {
		name := strings.TrimPrefix(m, "@")
		if id, ok := userMap[strings.ToLower(name)]; ok {
			return fmt.Sprintf("<@%s>", id)
		}
		return m
	})
}

func init() {
	command.RegisterCommand(
		&AnnounceContextCommand{},
		middleware.WithGroupAccessCheck(),
		middleware.WithGuildOnly(),
		middleware.WithUserPermissionCheck(),
		middleware.WithCommandLogger(),
	)
}
