package chat

import (
	"regexp"

	"github.com/bwmarrin/discordgo"
)

// Discord's markup that carries ids. Every one of them is rewritten before a
// message becomes part of a conversation, because the conversation is sent to
// a model and a Discord id is not the model's business: it is a stable
// identifier for a real account, and a relay has no reason to see one.
var (
	userMarkup    = regexp.MustCompile(`<@!?(\d+)>`)
	roleMarkup    = regexp.MustCompile(`<@&(\d+)>`)
	channelMarkup = regexp.MustCompile(`<#(\d+)>`)
	emojiMarkup   = regexp.MustCompile(`<a?:(\w+):\d+>`)
	messageLink   = regexp.MustCompile(`https?://(?:\w+\.)?discord(?:app)?\.com/channels/[\d@me]+/\d+(?:/\d+)?`)
	// bareID is a snowflake written out as plain digits, which people do
	// when they paste one. Seventeen digits and up, so a year or a price is
	// never taken for one.
	bareID = regexp.MustCompile(`\b\d{17,20}\b`)
)

// plain renders a message's text the way people in the channel see it, with
// every id replaced by the name it stands for, or by a word when the name
// cannot be resolved. See docs/persona.md.
//
// Her own messages go through here too. What she sent carries real mentions
// (see Service.outgoing), and read back from history after a restart they
// would otherwise put ids in front of the model as her own words.
func plain(sess *discordgo.Session, guildID string, m *discordgo.Message) string {
	names := make(map[string]string, len(m.Mentions))
	for _, u := range m.Mentions {
		if u == nil {
			continue
		}
		var member *discordgo.Member
		if sess != nil && sess.State != nil {
			member, _ = sess.State.Member(guildID, u.ID)
		}
		names[u.ID] = displayNameOf(u, member)
	}
	return plainText(sess, guildID, m.Content, names)
}

// plainText rewrites ids in text, taking user names from names first and the
// session's cache after.
func plainText(sess *discordgo.Session, guildID, text string, names map[string]string) string {
	state := func() *discordgo.State {
		if sess == nil {
			return nil
		}
		return sess.State
	}()

	text = userMarkup.ReplaceAllStringFunc(text, func(tag string) string {
		id := userMarkup.FindStringSubmatch(tag)[1]
		if name := names[id]; name != "" {
			return "@" + name
		}
		if state != nil {
			if member, err := state.Member(guildID, id); err == nil && member != nil {
				return "@" + displayNameOf(member.User, member)
			}
		}
		return "@someone"
	})
	text = roleMarkup.ReplaceAllStringFunc(text, func(tag string) string {
		if state != nil {
			if role, err := state.Role(guildID, roleMarkup.FindStringSubmatch(tag)[1]); err == nil && role != nil {
				return "@" + role.Name
			}
		}
		return "@a role"
	})
	text = channelMarkup.ReplaceAllStringFunc(text, func(tag string) string {
		if state != nil {
			if ch, err := state.Channel(channelMarkup.FindStringSubmatch(tag)[1]); err == nil && ch != nil {
				return "#" + ch.Name
			}
		}
		return "#a channel"
	})
	text = emojiMarkup.ReplaceAllString(text, ":$1:")
	text = messageLink.ReplaceAllString(text, "(a link to a message)")
	return bareID.ReplaceAllString(text, "(an id)")
}
