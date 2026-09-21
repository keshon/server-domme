package chat

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/keshon/server-domme/internal/discord/cmdadapter"
	"github.com/keshon/server-domme/internal/mind"
	"github.com/keshon/server-domme/internal/storage"
)

// messageID pulls a message id out of a Discord message link, or accepts a
// bare id.
var messageID = regexp.MustCompile(`(\d{15,21})\s*$`)

// runWhy explains one of her decisions in this channel: the latest, or the one
// about a given message — either the message she was deciding about or her
// own reply.
//
// This is the answer to "was that intentional or a bug?" without anyone
// reading the code: what she made of it, what she decided, and what was
// posted.
func (c *ChatCommand) runWhy(
	context *cmdadapter.SlashInteractionContext,
	sub *discordgo.ApplicationCommandInteractionDataOption,
) error {
	s, e := context.Session, context.Event

	var want string
	for _, opt := range sub.Options {
		if opt.Name == optMessage {
			if m := messageID.FindStringSubmatch(strings.TrimSpace(opt.StringValue())); m != nil {
				want = m[1]
			}
		}
	}

	entries := c.Service.Journal(e.GuildID, e.ChannelID)
	if len(entries) == 0 {
		return respond(s, e, "Nothing in her journal for this channel yet. She notes every decision about a message once she is listening here.")
	}

	var entry *storage.MindJournal
	for i := len(entries) - 1; i >= 0; i-- {
		if want == "" || entries[i].MessageID == want || entries[i].ReplyID == want {
			entry = &entries[i]
			break
		}
	}
	if entry == nil {
		return respond(s, e, fmt.Sprintf(
			"Nothing in her journal about that message. She keeps the last %d decisions per channel, "+
				"and only for messages that reached her — tagged, replied to, her name, or carrying on a conversation with her.",
			len(entries)))
	}

	return respond(s, e, explain(*entry))
}

// explain renders one journal entry in plain words: what reached her, what
// she made of it, what she decided and what came of it. The appraisal is
// the model's own words, shown as they were.
func explain(j storage.MindJournal) string {
	var b strings.Builder
	fmt.Fprintf(&b, "**Why** · <t:%d:R>\n", j.At.Unix())

	who := j.Username
	if who == "" {
		who = "someone"
	}
	fmt.Fprintf(&b, "%s\n", triggerWords(j.Trigger, who))
	if j.Excerpt != "" {
		fmt.Fprintf(&b, "> %s\n", j.Excerpt)
	}

	if j.Why != "" {
		fmt.Fprintf(&b, "\n**Her reason** %s\n", j.Why)
	}
	if j.Read != "" {
		fmt.Fprintf(&b, "\n**How she read it** %s\n", j.Read)
	}
	if j.Feel != "" {
		fmt.Fprintf(&b, "**How it landed** %s\n", j.Feel)
	}
	if j.Toward != "" {
		fmt.Fprintf(&b, "**Towards them** %s\n", j.Toward)
	}
	if j.Mood != "" {
		fmt.Fprintf(&b, "**Her mood after** %s\n", j.Mood)
	}
	if j.Act != "" {
		fmt.Fprintf(&b, "\n**Decided** %s", actWords(j.Act))
		if j.Intent != "" {
			fmt.Fprintf(&b, " — %s", j.Intent)
		}
		b.WriteString("\n")
	}

	fmt.Fprintf(&b, "**Outcome** %s", j.Outcome)
	if j.Took > 0 {
		fmt.Fprintf(&b, " in %.1fs", j.Took.Seconds())
	}
	if j.Backend != "" {
		fmt.Fprintf(&b, " via `%s`", j.Backend)
	}
	if j.Reason != "" {
		fmt.Fprintf(&b, " — %s", j.Reason)
	}
	b.WriteString("\n")

	if j.Posted != "" {
		fmt.Fprintf(&b, "**Posted**\n> %s\n", j.Posted)
	}
	return trimForEmbedBody(b.String())
}

func triggerWords(t, who string) string {
	switch mind.Trigger(t) {
	case mind.TriggerMention:
		return "**" + who + "** tagged her"
	case mind.TriggerReply:
		return "**" + who + "** replied to her message"
	case mind.TriggerNamed:
		return "**" + who + "** said her name"
	case mind.TriggerFollowUp:
		return "**" + who + "** carried on talking to her, untagged"
	case mind.TriggerOverheard:
		return "She overheard **" + who + "**"
	case mind.TriggerReach:
		return "She went to **" + who + "** on her own"
	case mind.TriggerStart:
		return "She spoke up on her own"
	default:
		return "**" + who + "** reached her"
	}
}

func actWords(act string) string {
	switch mind.Act(act) {
	case mind.ActReply:
		return "to answer"
	case mind.ActReact:
		return "to react and say nothing"
	case mind.ActIgnore:
		return "to let it go"
	default:
		return act
	}
}

// maxEmbedBody keeps an explanation inside Discord's embed description limit
// of 4096 characters, with room to spare.
const maxEmbedBody = 3900

func trimForEmbedBody(s string) string {
	if r := []rune(s); len(r) > maxEmbedBody {
		return string(r[:maxEmbedBody]) + "…"
	}
	return s
}
