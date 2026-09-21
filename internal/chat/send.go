package chat

import (
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/keshon/server-domme/internal/mind"
)

// sentMessage is what went out: its id, and the text as people saw it.
type sentMessage struct {
	id   string
	text string
}

// deliver types the reply the way a person would, posts it, and records it as
// hers in the conversation.
//
// What is recorded is what people saw, after Casual: that is what the next
// repeat check compares against and what she will be shown as having said.
func (s *Service) deliver(sess *discordgo.Session, sc mind.Scene, reply string) (sentMessage, error) {
	text := mind.Casual(reply, mind.CasualStyle{SlipChance: s.casualSlips}, s.roll())
	msg := s.outgoing(sc, text)
	sent, err := sess.ChannelMessageSendComplex(sc.ChannelID, msg)
	if err != nil {
		return sentMessage{}, err
	}
	out := sentMessage{text: text}
	if sent != nil {
		out.id = sent.ID
	}
	s.conv.Record(sc.ChannelID, mind.Turn{
		Content:   text,
		At:        s.now(),
		FromBot:   true,
		MessageID: out.id,
		To:        sc.UserID,
	})
	return out, nil
}

// outgoing builds the message as Discord will receive it.
//
// "@Name" she wrote becomes a real mention for anyone in the conversation,
// but only the person she is answering or going to can be notified by it.
// Letting her ping whoever she names would make her an instrument: "tag John
// and call him a butthead" is one message away. Roles, @everyone and @here
// are never parsed.
//
// Anchored as a Discord reply when a bare message would leave people
// guessing what it answers: a late answer, an answer to a reply, or a channel
// where someone else has spoken since the line she is answering.
func (s *Service) outgoing(sc mind.Scene, content string) *discordgo.MessageSend {
	resolved, named := mind.ResolveMentions(content, mentionable(sc))

	// Going after someone has to reach them: if the model did not tag them,
	// the tag goes first.
	if sc.Trigger == mind.TriggerReach && sc.UserID != "" && !strings.Contains(resolved, "<@"+sc.UserID+">") {
		resolved = "<@" + sc.UserID + "> " + resolved
		named = append(named, sc.UserID)
	}

	allowed := &discordgo.MessageAllowedMentions{Parse: []discordgo.AllowedMentionType{}}
	for _, id := range named {
		if id == sc.UserID {
			allowed.Users = []string{id}
		}
	}
	msg := &discordgo.MessageSend{Content: resolved, AllowedMentions: allowed}

	if sc.MessageID != "" && !mind.Initiated(sc.Trigger) &&
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
	}
	return people
}
