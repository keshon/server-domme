package chat

import (
	"strings"
	"time"

	"github.com/keshon/server-domme/internal/mind"
	"github.com/keshon/server-domme/internal/storage"
)

// The journal: one entry per decision about a message, completed as the
// reply resolves. See storage.MindJournal and /chat why.
//
// Written from the same places that make the decisions, so it records what
// happened rather than what the code was meant to do — the questions it
// exists for ("why did she ignore me?", "why that line?") are exactly the
// ones where those two differ.

// Journal outcomes, as /chat why shows them.
const (
	outcomeQueued   = "queued"
	outcomeAnswered = "answered"
	outcomeSilent   = "stayed quiet"
	outcomeDeclined = "declined"
	outcomeDropped  = "dropped"
	outcomeHeld     = "held for later"
	outcomeJoined   = "part of an answer already coming"
)

// Day counts, as /chat status shows them.
const (
	countAnswered     = "answered"
	countSilent       = "silent"
	countDeclined     = "declined"
	countVolunteered  = "volunteered"
	countAfterthought = "afterthought"
	countRepeat       = "repeat caught"
	countEcho         = "echo caught"
	countRelayFailed  = "relay failed"
	countLiked        = "liked"
	countPanned       = "panned"
	countRepeating    = "told repeating"
)

// Excerpt lengths. Short on purpose: enough to recognise the moment, not a
// transcript of the channel.
const (
	journalExcerpt = 160
	journalReply   = 300
)

// journalOpen records a decision and returns the entry's id, or 0 when it
// could not be written — a lost journal entry is not worth failing a reply
// over.
func (s *Service) journalOpen(j storage.MindJournal) uint64 {
	j.Excerpt = excerpt(j.Excerpt, journalExcerpt)
	id, err := s.store.AddMindJournal(j)
	if err != nil {
		s.log.Debug().Err(err).Str("guild_id", j.GuildID).Msg("chat_journal_write_failed")
		return 0
	}
	return id
}

// journalUpdate completes or amends an entry.
func (s *Service) journalUpdate(guildID string, id uint64, change func(*storage.MindJournal)) {
	if id == 0 {
		return
	}
	if err := s.store.UpdateMindJournal(guildID, id, change); err != nil {
		s.log.Debug().Err(err).Str("guild_id", guildID).Msg("chat_journal_write_failed")
	}
}

// count adds one to today's count of an event.
func (s *Service) count(guildID, event string) {
	if err := s.store.CountMindEvent(guildID, s.day(time.Now()), event); err != nil {
		s.log.Debug().Err(err).Str("guild_id", guildID).Msg("chat_count_failed")
	}
}

// Today is what she has done in a guild today, for /chat status.
func (s *Service) Today(guildID string) map[string]int {
	return s.store.MindDayCounts(guildID, s.day(time.Now()))
}

// Journal is a channel's recent decisions, oldest first, for /chat why.
func (s *Service) Journal(guildID, channelID string) []storage.MindJournal {
	return s.store.MindJournalIn(guildID, channelID)
}

// journalReaction records how her latest reply to someone landed, on the
// entry for that reply.
func (s *Service) journalReaction(guildID, channelID, userID string, r mind.Reception) {
	entries := s.store.MindJournalIn(guildID, channelID)
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		if e.ReplyID == "" || e.UserID != userID {
			continue
		}
		s.journalUpdate(guildID, e.ID, func(j *storage.MindJournal) { j.Reaction = string(r) })
		return
	}
}

// spoken is what speak learned about one reply, written to its journal entry
// once it resolves, whichever way that is.
type spoken struct {
	outcome, reason string
	told            []string
	thought         string
	perceived       string
	raw, posted     string
	backend         string
	took            time.Duration
	replyID         string
}

// closeSpoken writes the reply's outcome to its entry.
func (s *Service) closeSpoken(t task, sp *spoken) {
	if sp.outcome == "" {
		sp.outcome = outcomeDropped
	}
	s.journalUpdate(t.item.GuildID, t.item.Journal, func(j *storage.MindJournal) {
		j.Outcome, j.Reason = sp.outcome, sp.reason
		j.Told, j.Thought = sp.told, sp.thought
		j.Perceived = sp.perceived
		j.Raw, j.Posted = excerpt(sp.raw, journalReply), excerpt(sp.posted, journalReply)
		j.Backend, j.Took = sp.backend, sp.took
		j.ReplyID = sp.replyID
	})
}

func excerpt(s string, max int) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > max {
		return string(r[:max]) + "…"
	}
	return s
}
