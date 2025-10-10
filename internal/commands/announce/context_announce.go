package announce

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

func (c *AnnounceCommand) Name() string { return "Announce" }
func (c *AnnounceCommand) Description() string {
	return "Send a message to the announcement channel (context command)"
}
func (c *AnnounceCommand) Aliases() []string { return []string{} }
func (c *AnnounceCommand) Group() string     { return "announce" }
func (c *AnnounceCommand) Category() string  { return "📢 Utilities" }
func (c *AnnounceCommand) UserPermissions() []int64 {
	return []int64{
		discordgo.PermissionAdministrator,
	}
}
func (c *AnnounceCommand) BotPermissions() []int64 {
	return []int64{
		discordgo.PermissionUseApplicationCommands,
	}
}

func (c *AnnounceCommand) ContextDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name: c.Name(),
		Type: discordgo.MessageApplicationCommand,
	}
}

func (c *AnnounceCommand) Run(ctx interface{}) error {
	context, ok := ctx.(*core.MessageApplicationCommandContext)
	if !ok {
		return nil
	}

	session := context.Session
	event := context.Event
	storage := context.Storage

	guildID := event.GuildID
	channelID := event.ChannelID

	// Deferred ephemeral reply
	if err := core.RespondDeferredEphemeral(session, event); err != nil {
		log.Println("Failed to send deferred response:", err)
		return nil
	}

	// Fetch the target message
	targetID := event.ApplicationCommandData().TargetID
	msg, err := session.ChannelMessage(channelID, targetID)
	if err != nil {
		core.EditResponse(session, event, fmt.Sprintf("Couldn't fetch the message: `%v`", err))
		return nil
	}

	// Validation
	if msg.Author == nil {
		core.EditResponse(session, event, "I won't announce ghost messages.")
		return nil
	}
	if msg.Content == "" && len(msg.Embeds) == 0 && len(msg.Attachments) == 0 {
		core.EditResponse(session, event, "Empty? I'm not announcing tumbleweeds.")
		return nil
	}

	// Fetch announcement channel
	announceChannelID, err := storage.GetSpecialChannel(guildID, "announce")
	if err != nil || announceChannelID == "" {
		core.EditResponse(session, event, "No announcement channel configured. Bother the admin.")
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
		Content: restoreMentions(session, guildID, msg.Content),
		Embeds:  msg.Embeds,
		Files:   files,
	}

	if _, err := session.ChannelMessageSendComplex(announceChannelID, message); err != nil {
		core.EditResponse(session, event, fmt.Sprintf("Couldn't announce it: `%v`", err))
		return nil
	}

	core.EditResponse(session, event, "Announced. Everyone’s watching now.")
	return nil
}

var mentionRegex = regexp.MustCompile(`@(\S+)`)

func restoreMentions(session *discordgo.Session, guildID, content string) string {
	members, err := session.GuildMembers(guildID, "", 1000)
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
	core.RegisterCommand(
		core.ApplyMiddlewares(
			&AnnounceCommand{},
			core.WithGroupAccessCheck(),
			core.WithGuildOnly(),
			core.WithBotPermissionCheck(),
			core.WithUserPermissionCheck(),
			core.WithCommandLogger(),
		),
	)
}
