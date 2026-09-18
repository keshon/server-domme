package chat

import (
	"time"

	"github.com/keshon/server-domme/internal/mind"
)

// receive reads how her last message landed with someone answering it, and
// lets it move her at once: a laugh warms her a little towards them, being
// told it was bad annoys her a little, and either way her next reply to them
// is told. Called on the gateway goroutine; at most one storage write.
func (s *Service) receive(guildID, channelID, userID, name, content string, now time.Time) {
	kind := mind.ReadReception(content)
	if kind == mind.ReceptionNone {
		return
	}

	s.receptionMu.Lock()
	s.receptions[channelID] = mind.Received{UserID: userID, Username: name, Kind: kind, At: now}
	s.receptionMu.Unlock()

	s.appraise(guildID, userID, mind.ReceptionEvent(kind), now)

	s.journalReaction(guildID, channelID, userID, kind)
	switch kind {
	case mind.ReceptionLiked:
		s.count(guildID, countLiked)
	case mind.ReceptionPanned:
		s.count(guildID, countPanned)
	case mind.ReceptionRepeating:
		s.count(guildID, countRepeating)
	}

	s.log.Info().
		Str("guild_id", guildID).
		Str("channel_id", channelID).
		Str("user_id", userID).
		Str("reception", string(kind)).
		Msg("chat_reception")
}

// receptionFor is the instruction for her reply to userID, if a reaction of
// theirs is still current.
func (s *Service) receptionFor(channelID, userID, name string, now time.Time) string {
	s.receptionMu.Lock()
	r, ok := s.receptions[channelID]
	s.receptionMu.Unlock()
	if !ok || !r.Current(userID, now) {
		return ""
	}
	if name == "" {
		name = r.Username
	}
	return mind.ReceptionDirective(name, r.Kind)
}

// receptionUsed forgets a reaction once she has answered it, so one laugh does
// not colour every reply for the next five minutes.
func (s *Service) receptionUsed(channelID, userID string) {
	s.receptionMu.Lock()
	defer s.receptionMu.Unlock()
	if r, ok := s.receptions[channelID]; ok && r.UserID == userID {
		delete(s.receptions, channelID)
	}
}
