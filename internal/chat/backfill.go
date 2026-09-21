package chat

import (
	"github.com/bwmarrin/discordgo"
	"github.com/keshon/server-domme/internal/mind"
)

// backfillLimit is how many messages to read when first entering a channel.
//
// Slightly above the conversation buffer's own cap, because bots and empty
// messages are dropped on the way in and the survivors should still fill it.
// 50 is also Discord's maximum for one request, so this stays a single call.
const backfillLimit = 50

// backfill seeds a channel's conversation from Discord's own history.
//
// The buffer is empty at startup, so without this she knows only what has been
// said since the process came up: a redeploy mid-conversation leaves her
// answering a question whose subject she never saw, and being tagged into a
// discussion that has been running for twenty minutes shows her the tag alone.
// Everyone else in the channel can scroll up. This is her doing the same.
//
// It costs one REST call per channel per process, and no model call at all,
// which is the whole reason it is the first thing worth adding: every other
// way of giving her context spends a backend request she may not get.
//
// Called from the worker rather than from Observe. Observe runs on the gateway
// goroutine and must not block on the network — see Service.Observe.
func (s *Service) backfill(sess *discordgo.Session, channelID string) {
	if sess == nil || channelID == "" || !s.conv.NeedsSeed(channelID) {
		return
	}

	messages, err := sess.ChannelMessages(channelID, backfillLimit, "", "", "")
	if err != nil {
		// Seed nothing, but still mark the channel done. Missing
		// READ_MESSAGE_HISTORY fails identically every time, and retrying on
		// every reply would spend a request per message to keep learning it.
		s.conv.Seed(channelID, nil)
		s.log.Debug().
			Err(err).
			Str("channel_id", channelID).
			Msg("chat_backfill_failed")
		return
	}

	turns := s.historyToTurns(sess, messages)
	s.conv.Seed(channelID, turns)
	s.log.Debug().
		Str("channel_id", channelID).
		Int("fetched", len(messages)).
		Int("kept", len(turns)).
		Msg("chat_backfilled")
}

// historyToTurns converts Discord messages into conversation turns, oldest
// first.
//
// Discord returns newest first, which is the reverse of how the buffer and the
// prompt read, so the order is flipped here rather than anywhere later.
func (s *Service) historyToTurns(sess *discordgo.Session, messages []*discordgo.Message) []mind.Turn {
	self := selfID(sess)
	var guildID string
	if len(messages) > 0 && messages[0] != nil && sess != nil && sess.State != nil {
		if ch, err := sess.State.Channel(messages[0].ChannelID); err == nil && ch != nil {
			guildID = ch.GuildID
		}
	}

	turns := make([]mind.Turn, 0, len(messages))
	for i := len(messages) - 1; i >= 0; i-- {
		m := messages[i]
		if m == nil || m.Author == nil {
			continue
		}

		// Her own messages are kept and marked, so she can see what she
		// already said and not repeat it. Other bots are dropped: their
		// embeds and command output are noise in a transcript, and answering
		// them is how two bots end up talking to each other.
		fromBot := m.Author.ID == self
		if m.Author.Bot && !fromBot {
			continue
		}

		content := plain(sess, guildID, m)
		if content == "" {
			// An attachment, embed or sticker with no text. There is nothing
			// for a language model to read in it.
			continue
		}

		turns = append(turns, mind.Turn{
			UserID:    m.Author.ID,
			Username:  displayNameOf(m.Author, m.Member),
			Content:   content,
			At:        m.Timestamp,
			MessageID: m.ID,
			FromBot:   fromBot,
			Mentioned: mentions(m, self),
			Tagged:    tagged(sess, guildID, m, self),
		})
	}
	return turns
}

// tagged is who a message mentioned, other than her and other bots, by the
// names people see.
func tagged(sess *discordgo.Session, guildID string, m *discordgo.Message, self string) []mind.Person {
	var out []mind.Person
	for _, u := range m.Mentions {
		if u == nil || u.ID == self || u.Bot {
			continue
		}
		var member *discordgo.Member
		if sess != nil && sess.State != nil {
			member, _ = sess.State.Member(guildID, u.ID)
		}
		out = append(out, mind.Person{ID: u.ID, Name: displayNameOf(u, member)})
	}
	return out
}

// mentions reports whether a historical message addressed the bot.
func mentions(m *discordgo.Message, self string) bool {
	if self == "" {
		return false
	}
	for _, u := range m.Mentions {
		if u != nil && u.ID == self {
			return true
		}
	}
	return false
}
