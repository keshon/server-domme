package chat

import (
	"time"

	"github.com/keshon/server-domme/internal/mind"
	"github.com/keshon/server-domme/internal/storage"
)

// onHerMind ranks what is on her mind in a guild: everyone she knows and
// everything she remembers there. Read-only for now; see mind.OnHerMind.
func (s *Service) onHerMind(guildID string, now time.Time) []mind.OnMind {
	stored := s.store.MindPeople(guildID)
	people := make([]mind.PersonMind, 0, len(stored))
	for i := range stored {
		people = append(people, personMind(&stored[i], now))
	}
	// Mood from any channel: the mood is the guild's, and so is this list.
	mood := s.drives(guildID, "", now).Mood
	return mind.OnHerMind(people, s.memoriesOf(guildID, ""), mood, now)
}

// personMind is what salience reads from someone's record.
func personMind(p *storage.MindPerson, now time.Time) mind.PersonMind {
	closeness, tension, _ := bondOf(p).Now(now)
	active := p.LastActiveAt
	if p.LastSeen.After(active) {
		active = p.LastSeen
	}
	return mind.PersonMind{
		Name:         p.Username,
		Closeness:    closeness,
		Tension:      tension,
		LastExchange: p.LastExchangeAt,
		LastActive:   active,
		Concerns:     concernsOf(p),
	}
}
