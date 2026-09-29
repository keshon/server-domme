package announce

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/keshon/server-domme/internal/discord/adapter"
	"github.com/keshon/server-domme/internal/discord/reply"
	"github.com/keshon/server-domme/internal/storage"
)

type AnnounceCommand struct{}

func (c *AnnounceCommand) Name() string        { return "announce" }
func (c *AnnounceCommand) Description() string { return "Send a message on bot's behalf" }
func (c *AnnounceCommand) Group() string       { return "announce" }
func (c *AnnounceCommand) Category() string    { return "📢 Utilities" }
func (c *AnnounceCommand) UserPermissions() []int64 {
	return []int64{}
}

func (c *AnnounceCommand) SlashDefinition() *adapter.SlashCommand {
	return &adapter.SlashCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options: []adapter.SlashOption{
			{
				Type:        adapter.OptionString,
				Name:        "message_id",
				Description: "The ID of the message to publish",
				Required:    true,
			},
		},
	}
}

// MenuDefinition exposes the same command as a message context-menu entry, so
// a message is announced from where it sits rather than by pasting its id.
func (c *AnnounceCommand) MenuDefinition() *adapter.SlashCommand {
	return &adapter.SlashCommand{
		Type: adapter.MessageMenuCommand,
		Name: c.Name(),
	}
}

func (c *AnnounceCommand) Run(ctx *adapter.SlashInteractionContext) error {
	messageID := ctx.StringOption("message_id")
	if messageID == "" {
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: "Please provide a message ID to announce.",
			Color:       reply.EmbedColor,
		})
	}
	return publish(ctx.GuildID(), ctx.ChannelID(), messageID, ctx.Storage, ctx.API, ctx.RespondEphemeral)
}

// MessageCommand announces the message the entry was used on.
func (c *AnnounceCommand) MessageCommand(ctx *adapter.MessageCommandContext) error {
	return publish(ctx.GuildID(), ctx.ChannelID(), ctx.TargetMessageID, ctx.Storage, ctx.API, ctx.RespondEphemeral)
}

// publish fetches the message from its channel and reposts it to the guild's
// announcement channel. Both entry points share it: the only difference is
// which message was picked.
func publish(guildID, channelID, messageID string, store *storage.Storage, api adapter.SessionAPI, respond func(*adapter.Embed) error) error {
	announceChannelID, _ := store.GetAnnounceChannel(guildID)
	if announceChannelID == "" {
		return respond(&adapter.Embed{
			Description: "Announcement channel is not set. Use `/settings announce channel-set` first.",
			Color:       reply.EmbedColor,
		})
	}

	msg, err := api.ChannelMessage(channelID, messageID)
	if err != nil {
		return respond(&adapter.Embed{
			Description: fmt.Sprintf("Failed to fetch message: `%v`", err),
			Color:       reply.EmbedColor,
		})
	}
	if msg.Content == "" && len(msg.Embeds) == 0 && len(msg.Attachments) == 0 {
		return respond(&adapter.Embed{
			Description: "There's nothing to announce: the message is empty.",
			Color:       reply.EmbedColor,
		})
	}

	members, _ := api.GuildMembers(guildID)
	msg.Content = restoreMentions(members, msg.Content)

	if err := api.ForwardMessage(announceChannelID, msg); err != nil {
		return respond(&adapter.Embed{
			Description: fmt.Sprintf("Couldn't announce it: `%v`", err),
			Color:       reply.EmbedColor,
		})
	}

	return respond(&adapter.Embed{
		Description: fmt.Sprintf("Message successfully published to <#%s>.", announceChannelID),
		Color:       reply.EmbedColor,
	})
}

var mentionRegex = regexp.MustCompile(`@(\S+)`)

func restoreMentions(members []adapter.GuildMember, content string) string {
	userMap := make(map[string]string)
	for _, m := range members {
		userMap[strings.ToLower(m.Username)] = m.UserID
		if m.Nick != "" {
			userMap[strings.ToLower(m.Nick)] = m.UserID
		}
		if m.GlobalName != "" {
			userMap[strings.ToLower(m.GlobalName)] = m.UserID
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
