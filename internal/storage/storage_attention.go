package storage

import (
	"fmt"
	"time"

	"github.com/keshon/datastore"
)

// updatePerson applies change to one person's record, creating it if there is
// none — every attention write is a small edit of the same row.
func (s *Storage) updatePerson(guildID, userID string, at time.Time, change func(*MindPerson)) error {
	if guildID == "" || userID == "" {
		return fmt.Errorf("storage: person needs a guild and a user")
	}
	return s.db.Update(func(tx *datastore.Tx) error {
		col := datastore.In(tx, s.mindPeople)
		p, ok := col.Get(guildScopedKey(guildID, userID))
		if !ok {
			p = &MindPerson{GuildID: guildID, UserID: userID, FirstSeen: at}
		}
		change(p)
		return col.Put(p)
	})
}

// SetMindAttention records how much reaching out someone agrees to; empty
// withdraws it. Withdrawing also clears the count of unanswered reaches, so
// agreeing again starts afresh.
func (s *Storage) SetMindAttention(guildID, userID, level string, at time.Time) error {
	err := s.updatePerson(guildID, userID, at, func(p *MindPerson) {
		p.Attention = level
		if level == "" {
			p.Unanswered = 0
		}
	})
	if err != nil {
		return fmt.Errorf("storage: set attention: %w", err)
	}
	return nil
}

// ExchangeMindPerson records that someone spoke to her, in which channel. It
// is what missing them is measured from, and it answers any reach she made.
func (s *Storage) ExchangeMindPerson(guildID, userID, channelID string, at time.Time) error {
	err := s.updatePerson(guildID, userID, at, func(p *MindPerson) {
		p.LastExchangeAt = at
		p.LastChatChannel = channelID
		p.Unanswered = 0
	})
	if err != nil {
		return fmt.Errorf("storage: record exchange: %w", err)
	}
	return nil
}

// ActiveMindPerson records that someone was active somewhere in the server.
// A timestamp only — nothing about where or what — and only kept for people
// who opted in, since it is what lets her notice being ignored.
func (s *Storage) ActiveMindPerson(guildID, userID string, at time.Time) error {
	err := s.updatePerson(guildID, userID, at, func(p *MindPerson) { p.LastActiveAt = at })
	if err != nil {
		return fmt.Errorf("storage: record activity: %w", err)
	}
	return nil
}

// MarkReached records that she reached out to someone. The day's count starts
// again on a new day; unanswered goes up until they speak to her.
func (s *Storage) MarkReached(guildID, userID, day string, at time.Time) error {
	err := s.updatePerson(guildID, userID, at, func(p *MindPerson) {
		if p.ReachDay != day {
			p.ReachDay, p.ReachToday = day, 0
		}
		p.ReachToday++
		p.ReachedAt = at
		p.Unanswered++
	})
	if err != nil {
		return fmt.Errorf("storage: mark reached: %w", err)
	}
	return nil
}

// AttentionSeekers lists the people in a guild who have opted in.
func (s *Storage) AttentionSeekers(guildID string) []MindPerson {
	var out []MindPerson
	for _, p := range s.mindPeopleByGuild.Find(guildID) {
		if p.Attention != "" {
			out = append(out, *p)
		}
	}
	return out
}

// SetChatAttentionOff switches reaching out off, or back on, for a guild.
func (s *Storage) SetChatAttentionOff(guildID string, off bool) error {
	g := s.guildSettings(guildID)
	g.ChatAttentionOff = off
	return s.settings.Put(g)
}

// IsChatAttentionOff reports whether an administrator has switched reaching
// out off for a guild.
func (s *Storage) IsChatAttentionOff(guildID string) bool {
	return s.guildSettings(guildID).ChatAttentionOff
}
