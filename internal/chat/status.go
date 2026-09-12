package chat

import (
	"sort"
	"time"

	"github.com/keshon/server-domme/internal/ai"
	"github.com/keshon/server-domme/internal/mind"
)

// statReporter is the part of a provider that can describe itself. Pool
// implements it; a bare Client does not, and a test double need not.
type statReporter interface {
	Stats() []ai.BackendStat
}

// Status is an operator-facing snapshot of the persona.
type Status struct {
	// Backends is how each configured backend has been behaving, best first.
	// Empty when the provider does not report stats.
	Backends []ai.BackendStat
	// Waiting is how many approaches are held for a later attempt.
	Waiting int
	// Queued is how many approaches are waiting for a worker.
	Queued int
}

// Status reports what the service is doing. It is read-only and safe to call
// from a command handler.
func (s *Service) Status() Status {
	st := Status{
		Waiting: s.deferrals.Len(),
		Queued:  len(s.work),
	}
	if reporter, ok := s.provider.(statReporter); ok {
		st.Backends = reporter.Stats()
	}
	return st
}

// CharacterName reports who the persona is, for commands that describe her.
func (s *Service) CharacterName() string {
	if s.character == nil {
		return ""
	}
	return s.character.Name
}

// State is what the character is like right now in one channel, for the
// command that reports it.
//
// Every field is derived, not stored, which is the point: this shows the
// operator the same numbers the prompt was built from rather than a separate
// copy that can disagree with it.
type State struct {
	// Drives are how she is doing, on 0..1.
	Drives mind.Drives
	// Directives are the instructions those drives produced, verbatim as the
	// model receives them. Empty means the mood is unremarkable enough to say
	// nothing, which is the ordinary case.
	Directives []string
	// Style is the settled temperament from the character file, and its own
	// directives.
	Style          mind.SpeechStyle
	StyleDirective []string
	// LastSpokeAt is when she last said anything in this guild, which is what
	// the social drive is measured from.
	LastSpokeAt time.Time
	// Memories is how many things she still holds about this channel, and how
	// many of them are bright enough to reach a prompt right now.
	Memories int
	Recalled int
	// Nudge is how much the mood is moving the odds of answering an indirect
	// approach, positive or negative.
	Nudge float64
	// Irritated lists anyone in the conversation she has something against,
	// worst first. Held per person: being short with one member and ordinary
	// with the next is the thing being modelled.
	Irritated []Annoyance
}

// Annoyance is one person and how much they have got on her nerves.
type Annoyance struct {
	Username string
	Level    float64
}

// StateIn reports what she is like in one channel.
func (s *Service) StateIn(guildID, channelID string) State {
	now := time.Now()
	drives := s.drives(guildID, channelID, now)

	st := State{
		Drives:     drives,
		Directives: drives.Directives(),
		Nudge:      drives.Nudge(),
		Memories:   len(s.store.MindMemories(guildID, channelID)),
	}
	if s.character != nil {
		st.Style = s.character.Style
		st.StyleDirective = s.character.Style.Directives()
	}
	if guild := s.store.GetMindGuild(guildID); guild != nil {
		st.LastSpokeAt = guild.LastSpokeAt
	}

	present := s.present(guildID, channelID)
	_, recalled := s.remember(guildID, channelID, present, now)
	st.Recalled = len(recalled)

	for _, p := range present {
		if p.Irritation > 0 {
			st.Irritated = append(st.Irritated, Annoyance{Username: p.Username, Level: p.Irritation})
		}
	}
	sort.SliceStable(st.Irritated, func(i, j int) bool {
		return st.Irritated[i].Level > st.Irritated[j].Level
	})

	return st
}
