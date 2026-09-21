package chat

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/bwmarrin/discordgo"
	"github.com/keshon/server-domme/internal/mind"
)

// Typing pace. People type at a few characters a second; a reply that
// arrives the instant it is written — or two messages in the same second —
// is the rhythm of a machine. These are what a message waits after its
// typing indicator, less whatever the model already took.
const (
	typeBase    = time.Second
	typePerChar = 60 * time.Millisecond
	// typeMax keeps a long message from being held past Discord's own
	// typing indicator, which lasts about ten seconds.
	typeMax = 8 * time.Second
)

// typingTime is how long a person would take to type text.
func typingTime(text string) time.Duration {
	return min(typeBase+time.Duration(utf8.RuneCountInString(text))*typePerChar, typeMax)
}

// maxParts is how many messages one reply may go out as.
const maxParts = 2

// parts splits a reply on its first blank line into at most two messages, the
// way people send two thoughts in a row. Anything with a code block goes out
// whole: a split there breaks the block.
func parts(text string) []string {
	if strings.Contains(text, "```") {
		return []string{text}
	}
	var out []string
	for _, p := range strings.SplitN(strings.TrimSpace(text), "\n\n", maxParts) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return []string{text}
	}
	return out
}

// sentMessage is what went out: its id, and the text as people saw it.
type sentMessage struct {
	id   string
	text string
}

// deliver types the reply the way a person would, posts it, and records it as
// hers in the conversation. since is when she started writing it: the time
// the model took counts towards the typing.
//
// A reply of two paragraphs goes out as two messages, the second typed after
// the first is seen. Only the first is anchored as a reply: a second message
// quoting the same line again would split one exchange across two anchors.
//
// What is recorded is what people saw, after Casual: that is what the next
// repeat check compares against and what she will be shown as having said.
// The returned text is the whole of it; the id is the last message's.
func (s *Service) deliver(ctx context.Context, sess *discordgo.Session, sc mind.Scene, reply string, since time.Time) (sentMessage, error) {
	var out sentMessage
	var said []string
	for i, part := range parts(reply) {
		text := mind.Casual(part, mind.CasualStyle{SlipChance: s.casualSlips}, s.roll())
		if i > 0 {
			since = s.now()
			if err := sess.ChannelTyping(sc.ChannelID); err != nil {
				s.log.Debug().Err(err).Str("channel_id", sc.ChannelID).Msg("chat_typing_failed")
			}
		}
		if !s.pause(ctx, typingTime(text)-s.now().Sub(since)) {
			return out, ctx.Err()
		}
		msg := s.outgoing(sc, text)
		if i > 0 {
			msg.Reference = nil
		}
		sent, err := sess.ChannelMessageSendComplex(sc.ChannelID, msg)
		if err != nil {
			if i > 0 {
				// The first part is out and is hers; only the rest is lost.
				break
			}
			return sentMessage{}, err
		}
		if sent != nil {
			out.id = sent.ID
		}
		said = append(said, text)
		s.conv.Record(sc.ChannelID, mind.Turn{
			Content:   text,
			At:        s.now(),
			FromBot:   true,
			MessageID: out.id,
			To:        sc.UserID,
		})
	}
	out.text = strings.Join(said, "\n")
	return out, nil
}

// pause waits d, or until ctx ends, and reports whether to go on. The wait
// itself is replaceable so tests do not sleep; see Service.sleep.
func (s *Service) pause(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	return s.sleep(ctx, d)
}

// sleep waits d unless ctx ends first.
func sleep(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// outgoing builds the message as Discord will receive it.
//
// "@Name" she wrote becomes a real mention for anyone in the conversation,
// and so does a name she opens a sentence by calling — "Big N, welcome" —
// since that is how people tag each other and models leave the @ off. Only
// the person she is answering can be notified, and whoever they tagged in
// what she is answering: those were pinged by them already. Letting her ping
// anyone else she names would make her an instrument — "tag John and call
// him a butthead" is one message away. Roles, @everyone and @here are never
// parsed.
//
// Anchored as a Discord reply when a bare message would leave people
// guessing what it answers: a late answer, an answer to a reply, or a channel
// where someone else has spoken since the line she is answering.
func (s *Service) outgoing(sc mind.Scene, content string) *discordgo.MessageSend {
	people := mentionable(sc)
	resolved, named := mind.ResolveMentions(mind.TagVocatives(content, people), people)

	// Going after someone has to reach them: if the model did not tag them,
	// the tag goes first.
	if sc.Trigger == mind.TriggerReach && sc.UserID != "" && !strings.Contains(resolved, "<@"+sc.UserID+">") {
		resolved = "<@" + sc.UserID + "> " + resolved
		named = append(named, sc.UserID)
	}

	may := notifiable(sc)
	allowed := &discordgo.MessageAllowedMentions{Parse: []discordgo.AllowedMentionType{}}
	for _, id := range named {
		if may[id] {
			allowed.Users = append(allowed.Users, id)
		}
	}
	msg := &discordgo.MessageSend{Content: resolved, AllowedMentions: allowed}

	if sc.MessageID != "" && !mind.Initiated(sc.Trigger) && sc.Trigger != mind.TriggerThen &&
		(sc.Late > 0 || sc.Trigger == mind.TriggerReply || mind.NeedsAnchor(sc.Turns, sc.MessageID, sc.UserID)) {
		msg.Reference = &discordgo.MessageReference{
			MessageID: sc.MessageID,
			ChannelID: sc.ChannelID,
			GuildID:   sc.GuildID,
		}
	}
	return msg
}

// mentionable is everyone she could name: the people in the conversation, by
// the names it shows her, and the person the scene is about.
func mentionable(sc mind.Scene) []mind.Person {
	seen := make(map[string]bool)
	var people []mind.Person
	add := func(id, name string) {
		if id == "" || name == "" || seen[id+"\x00"+name] {
			return
		}
		seen[id+"\x00"+name] = true
		people = append(people, mind.Person{ID: id, Name: name})
	}
	add(sc.UserID, sc.Username)
	for _, t := range sc.Turns {
		if !t.FromBot {
			add(t.UserID, t.Username)
		}
		for _, p := range t.Tagged {
			add(p.ID, p.Name)
		}
	}
	return people
}

// notifiable is who a mention of hers may notify: the person she is
// answering, and whoever they tagged in the lines she is answering — the
// ones since she last spoke.
func notifiable(sc mind.Scene) map[string]bool {
	may := map[string]bool{}
	if sc.UserID != "" {
		may[sc.UserID] = true
	}
	if mind.Initiated(sc.Trigger) {
		return may
	}
	for i := len(sc.Turns) - 1; i >= 0 && !sc.Turns[i].FromBot; i-- {
		if sc.Turns[i].UserID != sc.UserID {
			continue
		}
		for _, p := range sc.Turns[i].Tagged {
			may[p.ID] = true
		}
	}
	return may
}
