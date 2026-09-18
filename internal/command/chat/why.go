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
// reading the code: which rule applied, the odds and the roll, her state, the
// instructions she was given, what the model returned and what was posted.
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

// explain renders one journal entry in plain words.
func explain(j storage.MindJournal) string {
	var b strings.Builder
	fmt.Fprintf(&b, "**Why** · <t:%d:R>\n", j.At.Unix())

	who := j.Username
	if who == "" {
		who = "someone"
	}
	fmt.Fprintf(&b, "**%s** %s", who, triggerWords(j.Trigger))
	if j.Closer {
		b.WriteString(" — a message that closes the topic")
	}
	b.WriteString("\n")
	if j.Excerpt != "" {
		fmt.Fprintf(&b, "> %s\n", j.Excerpt)
	}

	fmt.Fprintf(&b, "\n**Decision** %s\n", decisionWords(j))
	if j.Mood != "" || j.Attitude != "" {
		fmt.Fprintf(&b, "**Her state** %s", j.Mood)
		if j.Attitude != "" {
			fmt.Fprintf(&b, " · towards them: %s", j.Attitude)
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

	if len(j.Told) > 0 {
		b.WriteString("\n**She was told**\n")
		for _, line := range j.Told {
			b.WriteString("- " + line + "\n")
		}
	}
	// The label and the thought are both in what the model returned, tags
	// and all, which reads more plainly than the same text pulled out into
	// sections of its own. Shown apart only when there is no raw reply to
	// read them in.
	rawShown := j.Raw != "" && strings.TrimSpace(j.Raw) != strings.TrimSpace(j.Posted)
	if rawShown {
		fmt.Fprintf(&b, "\n**The model returned**\n```\n%s\n```\n", strings.ReplaceAll(j.Raw, "```", "'''"))
		if j.Perceived != "" || j.Thought != "" {
			b.WriteString("-# <tone> is shadow perception, recorded and not acted on; " +
				"<inner> is her first reaction, which changes nothing\n")
		}
	} else {
		if j.Perceived != "" {
			fmt.Fprintf(&b, "\n**Read their message as** %s\n-# shadow: recorded, not acted on\n", j.Perceived)
		}
		if j.Thought != "" {
			fmt.Fprintf(&b, "\n**Her first reaction** (from the model — changes nothing)\n> %s\n", j.Thought)
		}
	}
	if j.Posted != "" {
		fmt.Fprintf(&b, "**Posted**\n> %s\n", j.Posted)
	}
	if j.Reaction != "" {
		fmt.Fprintf(&b, "\n**How it landed** %s\n", mind.ReceptionWords(mind.Reception(j.Reaction)))
	}
	return trimForEmbedBody(b.String())
}

func triggerWords(t string) string {
	switch mind.Trigger(t) {
	case mind.TriggerMention:
		return "tagged her"
	case mind.TriggerReply:
		return "replied to her message"
	case mind.TriggerNamed:
		return "spoke to her by name"
	case mind.TriggerAbout:
		return "talked about her"
	case mind.TriggerFollowUp:
		return "carried on talking to her, untagged"
	case mind.TriggerReturn:
		return "came back after a long time away"
	case mind.TriggerRecall:
		return "raised something she remembers"
	case mind.TriggerAfterthought:
		return "got a short reply from her, and she had a second thought"
	default:
		return "reached her"
	}
}

func decisionWords(j storage.MindJournal) string {
	switch j.Rule {
	case mind.RuleFirstApproach:
		return "always answers someone's first approach"
	case mind.RuleIgnoredLast:
		return "had let their last approach go, so this one is always answered"
	case mind.RuleCloser, mind.RuleOdds:
		verdict := "answer"
		if j.Outcome == "stayed quiet" {
			verdict = "stay quiet"
		}
		kind := "odds"
		if j.Rule == mind.RuleCloser {
			kind = "odds for a closer"
		}
		return fmt.Sprintf("%s %.0f%%, rolled %.0f → %s", kind, j.Chance*100, j.Roll*100, verdict)
	case "":
		return "none recorded"
	default:
		return j.Rule
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
