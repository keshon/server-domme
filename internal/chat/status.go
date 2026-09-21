package chat

import (
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/keshon/server-domme/internal/ai"
	"github.com/keshon/server-domme/internal/memory"
	"github.com/keshon/server-domme/internal/storage"
)

// Excerpt lengths for the journal. Short on purpose: enough to recognise the
// moment, not a transcript of the channel.
const (
	journalExcerpt = 160
	journalReply   = 300
)

// journal records one decision, for /chat why. A lost entry is not worth
// failing anything over.
func (s *Service) journal(j storage.MindJournal) {
	if j.Outcome == "" {
		return
	}
	if _, err := s.store.AddMindJournal(j); err != nil {
		s.log.Debug().Err(err).Str("guild_id", j.GuildID).Msg("chat_journal_write_failed")
	}
	if err := s.store.CountMindEvent(j.GuildID, s.day(), j.Outcome); err != nil {
		s.log.Debug().Err(err).Str("guild_id", j.GuildID).Msg("chat_count_failed")
	}
}

// Journal is a channel's recent decisions, oldest first, for /chat why.
func (s *Service) Journal(guildID, channelID string) []storage.MindJournal {
	return s.store.MindJournalIn(guildID, channelID)
}

// Today is what she has done in a guild today, for /chat status.
func (s *Service) Today(guildID string) map[string]int {
	return s.store.MindDayCounts(guildID, s.day())
}

func (s *Service) day() string {
	return s.now().In(s.location).Format("2006-01-02")
}

func excerpt(s string, max int) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > max {
		return string(r[:max]) + "…"
	}
	return s
}

// statReporter is the part of a provider that can describe itself.
type statReporter interface {
	Stats() []ai.BackendStat
}

// Status is an operator-facing snapshot of the persona.
type Status struct {
	// Backends is how each backend has been behaving, best first. Empty when
	// the provider does not report stats.
	Backends []ai.BackendStat
	// Waiting is how many answers are held for a later attempt, and Queued
	// how many moments are waiting for a worker.
	Waiting int
	Queued  int
	// MemoryPath is where her memory lives on disk.
	MemoryPath string
}

// Status reports what the service is doing.
func (s *Service) Status() Status {
	st := Status{Waiting: s.deferrals.Len(), Queued: len(s.work), MemoryPath: s.memory.Root()}
	if reporter, ok := s.mind.Provider.(statReporter); ok {
		st.Backends = reporter.Stats()
	}
	return st
}

// CharacterName reports who the persona is.
func (s *Service) CharacterName() string {
	if s.character == nil {
		return ""
	}
	return s.character.Name
}

// State is how she is in one channel, as her memory has it: the same things
// the next prompt is built from, not a second copy that can disagree.
type State struct {
	Self memory.Self
	// People are the dossiers of whoever is in the conversation here.
	People []memory.Person
	// Threads are what she means to do in the guild.
	Threads []memory.Thread
	// Proactive is whether she may speak first here.
	Proactive bool
	// Last is the latest decision she made here, if any.
	Last *storage.MindJournal
}

// StateIn reports how she is in one channel.
func (s *Service) StateIn(guildID, channelID string) State {
	st := State{Proactive: s.store.IsChatProactive(guildID, channelID)}
	var err error
	if st.Self, err = s.memory.Self(guildID); err == nil && st.Self.Lately == "" && s.character != nil {
		st.Self.Lately = s.character.Lately
	}
	seen := make(map[string]bool)
	for _, t := range s.conv.Recent(channelID) {
		if t.FromBot || t.UserID == "" || seen[t.UserID] {
			continue
		}
		seen[t.UserID] = true
		if p, ok, err := s.memory.Person(guildID, t.UserID); err == nil && ok {
			st.People = append(st.People, p)
		}
	}
	if threads, err := s.memory.Threads(guildID); err == nil {
		st.Threads = memory.Unfinished(threads)
	}
	if entries := s.Journal(guildID, channelID); len(entries) > 0 {
		st.Last = &entries[len(entries)-1]
	}
	return st
}

// About is her dossier on someone, and the bookkeeping the datastore holds.
func (s *Service) About(guildID, userID string) (memory.Person, bool, *storage.MindPerson) {
	p, ok, err := s.memory.Person(guildID, userID)
	if err != nil {
		ok = false
	}
	return p, ok, s.store.GetMindPerson(guildID, userID)
}

// Now is the service's clock, in the community's timezone.
func (s *Service) Now() time.Time { return s.now().In(s.location) }

// Session is the current gateway session, for commands that need one.
func (s *Service) Session() *discordgo.Session { return s.session() }
