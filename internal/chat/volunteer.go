package chat

import (
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/keshon/server-domme/internal/mind"
	"github.com/keshon/server-domme/internal/storage"
)

// maybeVolunteer considers saying something without being asked, after a
// message that was not addressed to her.
//
// Only ever in answer to something a person just did — a regular coming back,
// or someone raising a subject she remembers. Nothing here runs on a timer, so
// a quiet channel stays quiet; see mind.TriggerReturn for why.
func (s *Service) maybeVolunteer(m *discordgo.MessageCreate, name string, person *storage.MindPerson, now time.Time) {
	if !s.store.IsChatProactive(m.GuildID, m.ChannelID) {
		return
	}

	trigger, why := s.reasonToVolunteer(m.GuildID, m.ChannelID, name, person, now)
	if trigger == "" {
		return
	}

	day := s.day(now)
	state := s.store.MindChannelState(m.GuildID, m.ChannelID)
	today := state.Today
	if state.Day != day {
		today = 0
	}

	willing := mind.MayVolunteer(mind.Volunteer{
		Trigger:         trigger,
		Now:             now,
		Enabled:         true,
		Today:           today,
		LastVolunteered: state.VolunteeredAt,
		LastSpokeAt:     s.lastSpokeAt(m.ChannelID),
		EngagedWindow:   s.attention.EngagedWindow,
		Turns:           s.conv.Recent(m.ChannelID),
		UserID:          m.Author.ID,
		Drives:          s.drives(m.GuildID, m.ChannelID, now),
	}, s.roll())
	if !willing {
		return
	}

	// Counted when decided rather than when sent. A remark that then fails to
	// generate still spends the budget, which errs towards her saying less —
	// the right direction for something nobody asked for.
	if err := s.store.MarkVolunteered(m.GuildID, m.ChannelID, day, now); err != nil {
		s.log.Warn().Err(err).Str("guild_id", m.GuildID).Msg("chat_volunteer_record_failed")
		return
	}

	item := mind.Deferred{
		GuildID:      m.GuildID,
		ChannelID:    m.ChannelID,
		MessageID:    m.ID,
		UserID:       m.Author.ID,
		Username:     name,
		Trigger:      trigger,
		FormedAt:     now,
		Volunteering: why,
	}

	select {
	case s.work <- task{item: item}:
		s.log.Info().
			Str("guild_id", m.GuildID).
			Str("channel_id", m.ChannelID).
			Str("trigger", string(trigger)).
			Msg("chat_volunteered")
	default:
		// Not held, unlike an answer. Nobody is waiting on this, and it would
		// arrive after the moment it was about.
		s.log.Debug().Str("guild_id", m.GuildID).Msg("chat_volunteer_dropped_busy")
	}
}

// reasonToVolunteer finds something worth saying unprompted, or nothing.
//
// A returning regular is checked first: noticing a person is a stronger reason
// to speak than a subject coming up, and it is the more human of the two.
func (s *Service) reasonToVolunteer(guildID, channelID, name string, person *storage.MindPerson, now time.Time) (mind.Trigger, string) {
	if person != nil {
		who := mind.Acquaintance{
			Messages: person.Messages,
			LastSeen: person.LastSeen,
			PrevSeen: person.PrevSeen,
		}
		// Only people she knows. Someone who said two things a month ago and
		// has come back is a stranger, and greeting a stranger's return reads
		// as surveillance rather than as recognition.
		if away := who.AwayFor(); away > 0 && who.Familiarity() != mind.FamiliarityNewcomer {
			return mind.TriggerReturn, mind.ReturnDirective(name, away)
		}
	}

	topic := mind.Keywords(s.liveTopic(channelID))
	if memory, ok := mind.Relevant(s.memoriesOf(guildID, channelID), now, topic); ok {
		return mind.TriggerRecall, mind.RecallDirective(memory.Gist)
	}
	return "", ""
}

// day is the calendar day in the community's timezone, which is what the
// daily budget counts in. The host's day would roll over at the wrong hour for
// everyone not in UTC.
func (s *Service) day(now time.Time) string {
	loc := s.location
	if loc == nil {
		loc = time.UTC
	}
	return now.In(loc).Format(time.DateOnly)
}
