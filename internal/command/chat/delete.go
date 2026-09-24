package chat

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/bwmarrin/discordgo"
	chatsvc "github.com/keshon/server-domme/internal/chat"
	"github.com/keshon/server-domme/internal/discord/cmdadapter"
)

// subDelete is /chat delete.
const subDelete = "delete"

// messageAt reads a Discord message link, which carries the channel the
// message is in: a line she posted somewhere else is deleted from where the
// command is run, without anyone having to go and stand in that channel.
var messageAt = regexp.MustCompile(`channels/\d+/(\d{15,21})/(\d{15,21})`)

// deleteOption is /chat delete: take back one of her messages.
func deleteOption() *discordgo.ApplicationCommandOption {
	return &discordgo.ApplicationCommandOption{
		Type:        discordgo.ApplicationCommandOptionSubCommand,
		Name:        subDelete,
		Description: "Delete one of her messages — and she forgets saying it",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        optMessage,
				Description: "Her message: a link, or the id if it is in this channel",
				Required:    true,
			},
		},
	}
}

// runDelete takes one of her messages back. Hers only — other people's
// messages are /purge's business, and that has an allowlist for a reason.
//
// It goes further than the delete: the message leaves the conversation she
// is in and her memory of saying it goes too. A line nobody can scroll up
// and read should not be what she answers from, and her own words in her
// memory reach the voice — that is how she once quoted someone back at
// themselves ninety minutes later.
func (c *ChatCommand) runDelete(context *cmdadapter.SlashInteractionContext, sub *discordgo.ApplicationCommandInteractionDataOption) error {
	s, e := context.Session, context.Event

	raw := ""
	for _, opt := range sub.Options {
		if opt.Name == optMessage {
			raw = strings.TrimSpace(opt.StringValue())
		}
	}
	channelID, wanted := e.ChannelID, ""
	if m := messageAt.FindStringSubmatch(raw); m != nil {
		channelID, wanted = m[1], m[2]
	} else if m := messageID.FindStringSubmatch(raw); m != nil {
		wanted = m[1]
	}
	if wanted == "" {
		return respond(s, e, "That is not a message id or a link. Right-click her message → Copy Message Link, or Copy Message ID with developer mode on.")
	}

	done, err := c.Service.Unsay(s, e.GuildID, channelID, wanted)
	switch {
	case errors.Is(err, chatsvc.ErrNotHers):
		return respond(s, e, "That message is not hers. This only takes back her own words — for anyone else's, that is `/purge`.")
	case errors.Is(err, chatsvc.ErrNoMessage):
		return respond(s, e, fmt.Sprintf("No message %s in <#%s>. If it is in another channel, paste the message link instead of the id.", wanted, channelID))
	case err != nil:
		return respond(s, e, "Discord refused: "+err.Error())
	}

	var b strings.Builder
	b.WriteString("Deleted.")
	if done.Text != "" {
		b.WriteString("\n> " + oneLineExcerpt(done.Text))
	}
	switch {
	case done.Forgotten > 0 && done.FromConversation:
		b.WriteString("\nIt is out of the conversation here, and she does not remember saying it.")
	case done.Forgotten > 0:
		b.WriteString("\nShe does not remember saying it.")
	case done.FromConversation:
		b.WriteString("\nIt is out of the conversation here. Nothing in her memory to drop.")
	default:
		b.WriteString("\nNothing of it was left in the conversation or in her memory.")
	}
	return respond(s, e, b.String())
}

// oneLineExcerpt is the start of a message on one line, for saying what was
// deleted without pasting the whole of it back.
func oneLineExcerpt(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	if r := []rune(text); len(r) > maxExcerpt {
		return string(r[:maxExcerpt]) + "…"
	}
	return text
}

// maxExcerpt is how much of a deleted message is quoted back.
const maxExcerpt = 120
