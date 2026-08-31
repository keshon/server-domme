package announce

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/keshon/server-domme/internal/discord/cmdadapter"
	"github.com/keshon/server-domme/internal/discord/reply"
	"github.com/rs/zerolog"
)

// attachmentFetchTimeout bounds one attachment download. Discord's CDN is the
// only host reached here, and a stalled fetch would otherwise hold a command
// slot open for as long as the connection stays half-open.
const attachmentFetchTimeout = 30 * time.Second

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
	context, ok := ctx.(*cmdadapter.MessageApplicationCommandContext)
	if !ok {
		return nil
	}

	s := context.Session
	e := context.Event
	storage := context.Storage

	guildID := e.GuildID
	channelID := e.ChannelID

	log := context.AppLog

	// Deferred ephemeral reply
	if err := reply.RespondDeferredEphemeral(s, e); err != nil {
		log.Warn().Err(err).Msg("announce_defer_failed")
		return nil
	}

	// Fetch the target message
	targetID := e.ApplicationCommandData().TargetID
	msg, err := s.ChannelMessage(channelID, targetID)
	if err != nil {
		editResponse(log, s, e, fmt.Sprintf("Couldn't fetch the message: `%v`", err))
		return nil
	}

	// Validation
	if msg.Author == nil {
		editResponse(log, s, e, "I won't announce ghost messages.")
		return nil
	}
	if msg.Content == "" && len(msg.Embeds) == 0 && len(msg.Attachments) == 0 {
		editResponse(log, s, e, "Empty? I'm not announcing tumbleweeds.")
		return nil
	}

	// Fetch announcement channel
	announceChannelID, err := storage.GetAnnounceChannel(guildID)
	if err != nil || announceChannelID == "" {
		editResponse(log, s, e, "No announcement channel configured. Bother the admin.")
		return nil
	}

	// Download attachments
	var files []*discordgo.File
	for _, att := range msg.Attachments {
		data, err := fetchAttachment(att.URL)
		if err != nil {
			log.Warn().Str("url", att.URL).Err(err).Msg("announce_attachment_fetch_failed")
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
		editResponse(log, s, e, fmt.Sprintf("Couldn't announce it: `%v`", err))
		return nil
	}

	editResponse(log, s, e, "Announced. Everyone’s watching now.")
	return nil
}

// editResponse replaces the deferred reply, logging rather than propagating a
// failure. By this point the announcement has already happened or already
// failed, so a lost edit only costs the user the confirmation — surfacing it as
// a command error would report the wrong outcome.
func editResponse(log zerolog.Logger, s *discordgo.Session, e *discordgo.InteractionCreate, content string) {
	if err := reply.EditResponse(s, e, content); err != nil {
		log.Warn().Err(err).Msg("announce_edit_response_failed")
	}
}

// fetchAttachment downloads one attachment in full.
//
// It reads the body into memory rather than streaming it onward because the
// re-send needs an io.Reader it can replay, and it closes per call: the loop
// that calls this used to `defer` inside itself, holding every response body
// open until the whole command returned.
func fetchAttachment(url string) ([]byte, error) {
	client := &http.Client{Timeout: attachmentFetchTimeout}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("announce: attachment fetch: unexpected status %s", resp.Status)
	}
	return io.ReadAll(resp.Body)
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
