package chat

import (
	"time"

	"github.com/keshon/server-domme/internal/mind"
	"github.com/keshon/server-domme/internal/storage"
)

// appraise applies one event to her bond with someone and returns the bond as
// it stands afterwards.
//
// The single way anything changes how she feels about a person: pestering, a
// brush-off, a laugh, a pan, a remembered conversation, how they took it when
// she came to them. Read, changed and written in one transaction, so two
// events landing together cannot overwrite each other, and the event is kept
// as the last thing that moved her so /chat about can say why she is the way
// she is with them.
func (s *Service) appraise(guildID, userID string, e mind.Event, now time.Time) mind.Bond {
	var after mind.Bond
	if e == "" || userID == "" {
		return after
	}
	err := s.store.UpdateMindPerson(guildID, userID, now, func(p *storage.MindPerson) {
		after = bondOf(p).Apply(e, now)
		setBond(p, after)
		p.LastEvent, p.LastEventAt = string(e), now
	})
	if err != nil {
		s.log.Warn().Err(err).Str("guild_id", guildID).Str("event", string(e)).Msg("chat_appraisal_failed")
		return after
	}
	s.log.Info().
		Str("guild_id", guildID).
		Str("user_id", userID).
		Str("event", string(e)).
		Msg("chat_appraised")
	return after
}

// bondOf reads the bond out of a person's record.
func bondOf(p *storage.MindPerson) mind.Bond {
	if p == nil {
		return mind.Bond{}
	}
	return mind.Bond{
		Closeness: p.Closeness, ClosenessAt: p.ClosenessAt,
		Tension: p.Tension, TensionAt: p.TensionAt,
		Welcome: p.Welcome, WelcomeAt: p.WelcomeAt,
	}
}

func setBond(p *storage.MindPerson, b mind.Bond) {
	p.Closeness, p.ClosenessAt = b.Closeness, b.ClosenessAt
	p.Tension, p.TensionAt = b.Tension, b.TensionAt
	p.Welcome, p.WelcomeAt = b.Welcome, b.WelcomeAt
}

// feelConversation carries a remembered conversation's tone into her bond
// with each person who was in it. Attribution is the hard half: with company,
// a conversation that went badly does not say who made it go badly, so it
// moves nobody; a warm one is shared out thinly. See mind.ConversationEvent.
func (s *Service) feelConversation(guildID string, people []string, tone mind.Tone, at time.Time) {
	e := mind.ConversationEvent(tone, len(people) == 1)
	for _, userID := range people {
		s.appraise(guildID, userID, e, at)
	}
}
