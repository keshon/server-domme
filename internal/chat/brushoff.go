package chat

import (
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/keshon/server-domme/internal/mind"
)

// maxRememberedSnubs bounds the set of questions already counted as ignored.
// It only has to outlive BrushOffWindow, so clearing it wholesale when full
// costs at most one question counted twice.
const maxRememberedSnubs = 512

// noticeSnub raises her irritation with someone who ignored her question to
// talk to somebody else. It is called before the message is recorded, since
// it asks what the conversation looked like just before.
//
// One storage write on the gateway goroutine, the same as the irritation from
// pestering. See mind.BrushedOff for why only this shape counts.
func (s *Service) noticeSnub(sess *discordgo.Session, m *discordgo.MessageCreate, now time.Time) {
	questionID, ok := mind.BrushedOff(s.conv.Recent(m.ChannelID), mind.Snub{
		Speaker:        m.Author.ID,
		ElsewhereAimed: s.aimedElsewhere(sess, m),
		Now:            now,
	})
	if !ok || !s.firstSnub(questionID) {
		return
	}

	level := s.irritationWith(m.GuildID, m.Author.ID, now) + mind.BrushOffStep
	if level > 1 {
		level = 1
	}
	if err := s.store.IrritateMindPerson(m.GuildID, m.Author.ID, level, now); err != nil {
		s.log.Warn().Err(err).Str("guild_id", m.GuildID).Msg("chat_irritation_record_failed")
		return
	}
	s.log.Info().
		Str("guild_id", m.GuildID).
		Str("channel_id", m.ChannelID).
		Str("user_id", m.Author.ID).
		Msg("chat_brushed_off")
}

// aimedElsewhere reports whether a message is plainly for someone other than
// her: a Discord reply to another person's message, or an @mention of someone
// else, and in either case not also aimed at her.
func (s *Service) aimedElsewhere(sess *discordgo.Session, m *discordgo.MessageCreate) bool {
	self := selfID(sess)
	if s.repliesToHer(m, self) {
		return false
	}
	other := false
	for _, u := range m.Mentions {
		if u == nil {
			continue
		}
		if u.ID == self {
			return false
		}
		if u.ID != m.Author.ID {
			other = true
		}
	}
	return other || (m.MessageReference != nil && m.MessageReference.MessageID != "")
}

// firstSnub reports whether this question has not been counted yet, and marks
// it counted.
func (s *Service) firstSnub(questionID string) bool {
	s.snubMu.Lock()
	defer s.snubMu.Unlock()
	if s.snubbed[questionID] {
		return false
	}
	if len(s.snubbed) >= maxRememberedSnubs {
		s.snubbed = make(map[string]bool)
	}
	s.snubbed[questionID] = true
	return true
}
