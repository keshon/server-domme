package chat

import (
	"math"
	"sort"
	"time"

	"github.com/bwmarrin/discordgo"
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
// operator the same numbers and the same instructions the prompt is built
// from rather than a separate copy that can disagree with it.
//
// What is left out matters as much. The character file's temperament used to
// be listed here, and it only changes when the file does — on a panel about
// how she is right now, it was the one thing that never moved.
type State struct {
	// Drives are how she is doing, on 0..1, and Mood and Wants are the same
	// thing in words.
	Drives mind.Drives
	Mood   string
	Wants  []string
	// Nudge is how much the mood is moving the odds of answering an indirect
	// approach, positive or negative.
	Nudge float64
	// People are the people in the conversation and how she stands with
	// each, the ones she feels most about first.
	People []Stance
	// Reaction is how her last message landed, while it still colours her
	// next reply, and ReactionFrom who it came from.
	Reaction     mind.Reception
	ReactionFrom string
	// Told is every instruction about her state that the next reply here
	// would carry, verbatim as the model receives it.
	Told []string
	// LastSpokeAt is when she last said anything in this guild.
	LastSpokeAt time.Time
	// Memories is how many things she still holds about this channel, and how
	// many of them are bright enough to reach a prompt right now.
	Memories int
	Recalled int
	// Proactive is whether she may speak first in this channel, and
	// VolunteeredToday how much of the day's allowance she has used.
	Proactive        bool
	VolunteeredToday int
	// Fatigue is how much she has put herself forward lately in the guild;
	// see mind.Fatigue.
	Fatigue float64
	// OnMind is what is on her mind in the guild, strongest first; see
	// mind.OnHerMind.
	OnMind []mind.OnMind
	// InnerVoice is whether she thinks before speaking, and Thought the
	// latest thing she thought here, empty until she has.
	InnerVoice bool
	Thought    Thought
}

// Stance is how she stands with one person.
type Stance struct {
	Username  string
	Attitude  string
	Closeness float64
	Tension   float64
	Regard    float64
}

// StateIn reports what she is like in one channel.
func (s *Service) StateIn(sess *discordgo.Session, guildID, channelID string) State {
	now := time.Now()
	drives := s.drives(guildID, channelID, now)

	st := State{
		Drives:   drives,
		Mood:     mind.MoodWords(drives),
		Wants:    mind.Wants(drives),
		Nudge:    drives.Nudge(),
		Memories: len(s.store.MindMemories(guildID, channelID)),
	}
	if guild := s.store.GetMindGuild(guildID); guild != nil {
		st.LastSpokeAt = guild.LastSpokeAt
	}
	st.Fatigue = s.fatigue(guildID, now)
	st.OnMind = s.onHerMind(guildID, now)

	st.Proactive = s.store.IsChatProactive(guildID, channelID)
	st.InnerVoice = s.innerVoice
	if t, ok := s.lastThought(channelID); ok {
		st.Thought = t
	}
	if ch := s.store.MindChannelState(guildID, channelID); ch.Day == s.day(now) {
		st.VolunteeredToday = ch.Today
	}

	present := s.present(guildID, channelID)
	_, recalled := s.remember(guildID, channelID, present, now)
	st.Recalled = len(recalled)

	for _, p := range present {
		regard := s.regardFor(sess, guildID, p.UserID)
		st.People = append(st.People, Stance{
			Username:  p.Username,
			Attitude:  mind.Attitude(p.Closeness, p.Tension, regard),
			Closeness: p.Closeness,
			Tension:   p.Tension,
			Regard:    regard,
		})
	}
	sort.SliceStable(st.People, func(i, j int) bool {
		return feeling(st.People[i]) > feeling(st.People[j])
	})

	g := mind.Grounding{Present: present, Drives: drives, Now: now}
	s.receptionMu.Lock()
	r, ok := s.receptions[channelID]
	s.receptionMu.Unlock()
	if ok && r.Current(r.UserID, now) {
		st.Reaction, st.ReactionFrom = r.Kind, r.Username
		g.Reception = mind.ReceptionDirective(r.Username, r.Kind)
	}
	st.Told = g.Told()

	return st
}

// feeling is how strongly she feels about someone either way, for ordering.
func feeling(p Stance) float64 {
	return p.Closeness + p.Tension + math.Abs(p.Regard)
}
