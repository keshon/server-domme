package chat

import (
	"fmt"
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

	trigger, why, pull, salience := s.reasonToVolunteer(m.GuildID, m.ChannelID, name, person, now)
	if trigger == "" {
		return
	}

	day := s.day(now)
	state := s.store.MindChannelState(m.GuildID, m.ChannelID)
	today := state.Today
	if state.Day != day {
		today = 0
	}

	fatigue := s.fatigue(m.GuildID, now)
	roll := s.roll()
	willing, chance := mind.MayVolunteer(mind.Volunteer{
		Trigger:       trigger,
		Now:           now,
		Enabled:       true,
		Today:         today,
		LastSpokeAt:   s.lastSpokeAt(m.ChannelID),
		EngagedWindow: s.attention.EngagedWindow,
		Turns:         s.conv.Recent(m.ChannelID),
		UserID:        m.Author.ID,
		Drives:        s.drives(m.GuildID, m.ChannelID, now),
		Fatigue:       fatigue,
		Pull:          pull,
		// A greeting is to them, and how they have taken what she started
		// before counts; a remembered subject is to the room.
		WelcomeShift: welcomeShiftFor(trigger, s.welcomeOf(m.GuildID, m.Author.ID, now)),
	}, roll)
	if !willing {
		return
	}

	// Counted when decided rather than when sent; see spendInitiative.
	if err := s.store.MarkVolunteered(m.GuildID, m.ChannelID, day, now); err != nil {
		s.log.Warn().Err(err).Str("guild_id", m.GuildID).Msg("chat_volunteer_record_failed")
		return
	}
	s.spendInitiative(m.GuildID, trigger, now)

	item := mind.Deferred{
		GuildID:      m.GuildID,
		ChannelID:    m.ChannelID,
		MessageID:    m.ID,
		UserID:       m.Author.ID,
		Username:     name,
		Trigger:      trigger,
		FormedAt:     now,
		Volunteering: why,
		Journal: s.journalOpen(storage.MindJournal{
			GuildID: m.GuildID, ChannelID: m.ChannelID, At: now,
			MessageID: m.ID, UserID: m.Author.ID, Username: name,
			Excerpt: m.ContentWithMentionsReplaced(),
			Trigger: string(trigger),
			Rule:    fmt.Sprintf("spoke first (pull %.2f, on her mind %.2f): %s%s", pull, salience, why, initiativeNote(fatigue)),
			Chance:  chance, Roll: roll,
			Mood:    mind.MoodWords(s.drives(m.GuildID, m.ChannelID, now)),
			Outcome: outcomeQueued,
		}),
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

// reasonToVolunteer finds something worth saying unprompted, or nothing, with
// how strongly it draws her and how much what it touches is on her mind.
//
// A returning regular is checked first: noticing a person is a stronger reason
// to speak than a subject coming up, and it is the more human of the two.
func (s *Service) reasonToVolunteer(guildID, channelID, name string, person *storage.MindPerson, now time.Time) (mind.Trigger, string, float64, float64) {
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
			// How much they are on her mind as they walk back in: missed,
			// liked, resented. Read as of just before they spoke, since
			// speaking is what just ended the absence.
			salience, _ := mind.PersonSalience(personMind(person, person.PrevSeen), now)
			pull := mind.ReturnPull(away, who.Familiarity(), salience)
			return mind.TriggerReturn, mind.ReturnDirective(name, away), pull, salience
		}
	}

	topic := mind.Keywords(s.liveTopic(channelID))
	if memory, overlap, ok := mind.Relevant(s.memoriesOf(guildID, channelID), now, topic); ok {
		salience := mind.SubjectSalience(memory, now)
		return mind.TriggerRecall, mind.RecallDirective(memory.Gist), mind.RecallPull(overlap, salience), salience
	}
	return "", "", 0, 0
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

// welcomeShiftFor is how glad the person a remark is aimed at has been of
// her, from neutral, or zero for a remark to the room.
func welcomeShiftFor(t mind.Trigger, welcome float64) float64 {
	if t == mind.TriggerRecall {
		return 0
	}
	return mind.WelcomeShift(welcome)
}
