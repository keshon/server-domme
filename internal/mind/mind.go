// Package mind is the persona's thinking: what she makes of a moment, what
// she wants to do about it, the words she says, and what she makes of a day
// when she looks back on it.
//
// v1 of this package held the premise that the language model is a speech
// cortex and nothing else, and kept every judgement in Go as a number or a
// word list. That is what retired it: numbers turned into instructions made
// her a caricature, word lists could not read an apology, and with no memory
// of what she had said she disowned her own words. v2 turns it round — the
// model interprets, appraises and remembers, in prose, and the code keeps
// time, memory and the few promises a model cannot be trusted with. See
// docs/persona.md; do not reintroduce emotional scalars here.
//
// Everything a moment needs is in a Scene; everything she knows comes from
// the memory store. Nothing here knows about Discord.
package mind

import (
	"context"
	"errors"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/keshon/server-domme/internal/ai"
	"github.com/keshon/server-domme/internal/memory"
	"github.com/rs/zerolog"
)

// Temperatures. Deciding wants a steady hand: the same moment should not be
// read as an insult on one call and a joke on the next, and the JSON has to
// come back parseable. The voice wants the backend's own.
const (
	thinkingTemperature = 0.4
	// reflectTemperature is a little warmer: a day summary written at 0.4
	// comes out as a list of what happened rather than what it meant.
	reflectTemperature = 0.6
)

// Recall limits: how much of her past goes in front of her at once. Small on
// purpose — the point is that the right thing surfaces, not that she arrives
// holding a dossier.
const (
	// recallDays is how far back recall looks at all. Ordinary moments
	// fade from it well before; see memory.FadeAfter.
	recallDays    = 90
	recallMoments = 8
	// recallPool is how many candidates recall scores, of which the best
	// recallMoments are shown; the rest are where a drifted memory comes
	// from.
	recallPool     = 40
	recentSummary  = 3
	transcriptTail = 6
)

// ErrUnreadable is returned when the model answered but not in the shape
// asked for. It is not a backend failure: retrying the same prompt tends to
// get the same shape back, so callers fall back rather than hold.
var ErrUnreadable = errors.New("mind: the model's answer could not be read")

// Mind is one persona: who she is, what she speaks through, and what she
// remembers.
type Mind struct {
	Character *Character
	// Provider is what she thinks through: appraisals, reflection,
	// initiative.
	Provider ai.Provider
	// Voice is what she speaks through; nil means Provider. Separate because
	// every model has a native voice, and hers should come from one chosen
	// for it. See docs/persona-v3.md, workstream A.
	Voice  ai.Provider
	Memory *memory.Store

	// SelfFacts is whether what she says about herself becomes true of her;
	// see ReflectSelf. Off, there is no me.md. docs/persona-v3.md, F2.
	SelfFacts bool
	// ExamplesSample is how many of the authored examples the voice is shown
	// on a call, drawn at random; zero shows them all. See voicePrompt.
	ExamplesSample int
	// ReplyTokens caps how long her spoken replies may run, in tokens; zero
	// leaves it to the backend. A backstop against a runaway reply, not
	// the thing that keeps her short: that is the character.
	ReplyTokens int
	// Drift is the odds that recall's weakest slot goes to a loosely related
	// memory instead; see drift. Zero recalls strictly by relevance.
	Drift float64
	// Feelings is whether she has feelings with a cause that fade on their
	// own, in place of v2's one-line mood. See docs/persona-v3.md, C.
	Feelings bool
	// ThinkBudget and VoiceBudget are the characters one prompt may run to
	// before what she knows starts giving way; zero uses the defaults. See
	// fit.
	ThinkBudget int
	VoiceBudget int
	// StyleCheck is whether a reply is held to her style after she writes
	// it: asked again once for a phrase that is not hers or a length far
	// past her examples, and cut to length. See Style.
	StyleCheck bool
	// Roll supplies randomness; nil uses the global source. One source for
	// the code's randomness, so a run can be repeated.
	Roll func() float64
	// Log is where refused proposals and trimmed prompts are reported. The
	// zero logger reports nothing.
	Log zerolog.Logger
}

// roll is a random number in [0,1).
func (m *Mind) roll() float64 {
	if m.Roll != nil {
		return m.Roll()
	}
	return rand.Float64()
}

// Scene is everything about a moment that is not in her memory: where she is,
// what is being said, and who she is answering or going to.
type Scene struct {
	GuildID      string
	GuildName    string
	ChannelID    string
	ChannelName  string
	ChannelTopic string
	// Brief is what an administrator told her the server is.
	Brief string
	// SelfName is what people here call her.
	SelfName string
	Now      time.Time

	// Turns are the live conversation in the channel, oldest first.
	Turns []Turn

	// Trigger is how the moment reached her, and UserID and Username who
	// it is about: who she is answering, or who she is going to.
	Trigger  Trigger
	UserID   string
	Username string
	// MessageID is the message she is answering, when there is one.
	MessageID string
	// WokenEarly is that she did not wake on her own: someone woke her.
	WokenEarly bool
	// Dropped is when the person she is answering asked her to drop
	// something, while that is still recent. A fact, like the time of day.
	Dropped time.Time
	// Reads are the rooms she passes through and never speaks in, by name.
	// She may ask to look into one before she answers; see Appraisal.Look.
	Reads []string
	// Crowd marks a follow-up in a room where others were talking too: the
	// code knows they spoke right after her, not that it was to her.
	Crowd bool
	// Late is how long she has taken to get to it, for an answer held back
	// by a backend that would not answer.
	Late time.Duration
	// Thread is what she meant to follow up on with the person, when that
	// is why the moment reached her; see TriggerSight.
	Thread *memory.Thread

	// Facts about her body, stated to her like the time of day and never
	// as how she feels: when she woke, and how long she has been talking in
	// this room and with how many. Zero leaves each out. See
	// docs/persona-v3.md, B5.
	Woke        time.Time
	TalkingFor  time.Duration
	TalkingWith int
	// RecallCap and ShortExamples are how much she takes in, set by the
	// body: at most RecallCap recalled moments (negative for none, zero for
	// the usual), and the shorter voice examples. Mechanics, never said to
	// her. See docs/persona-v3.md, B3.
	RecallCap     int
	ShortExamples bool
	// Reactions are what people put on her messages here since she last
	// spoke, as facts rather than as moments: no call is spent per emoji.
	Reactions []Reaction
	// QuietFor is how long since anyone spoke to her in this guild, when
	// known: a fact a want can be formed from, never a want itself.
	QuietFor time.Duration
	// ReactOnly is a room where she does not speak unless spoken to, and
	// may react: a fact about the room, stated to her.
	ReactOnly bool

	// Roles are what an administrator says about people here, by user id:
	// the note set for a role they hold. Standing a server decided, which
	// she takes as given rather than something she worked out.
	Roles map[string]string
}

// Reaction is one emoji on her messages, how many times, and from whom.
type Reaction struct {
	Emoji string
	Count int
	Names []string
}

// Known is what her memory holds that matters for a scene.
type Known struct {
	Self memory.Self
	// People are the dossiers of everyone in the scene, the person it is
	// about first. Someone she has no dossier on is present by name alone.
	People []memory.Person
	// Days are the summaries of the last few days she reflected on.
	Days []memory.Day
	// Recalled are moments from the past that bear on this one.
	Recalled []memory.Moment
	// Threads are the things she means to do, numbered from 1 in the order
	// given, which is how the model refers back to them.
	Threads []memory.Thread
	// Specifics are the author's concrete facts about her that go in front
	// of her this time: all of them while they fit their budget, otherwise
	// those that bear on the conversation first. See pickSpecifics.
	Specifics []string
	// SelfFacts are the things she has said about herself that bear on the
	// conversation, best match first. See pickSelfFacts.
	SelfFacts []memory.SelfFact
	// LastHeavy is when something last weighed on her, from the days
	// recall reads: a moment of weight at least heavyWeight.
	LastHeavy time.Time
	// Arc is how the conversation in this channel has gone so far, while
	// one is under way; nil otherwise. See memory.Arc.
	Arc *memory.Arc
}

// heavyWeight is the weight of a moment that counts as something having
// happened to her, for the drive facts.
const heavyWeight = 0.5

// Know gathers what she remembers that bears on a scene, with the dossiers of
// anyone in also — the people she might go to, for an initiative.
func (m *Mind) Know(s Scene, also ...string) (Known, error) {
	var k Known
	var err error

	if k.Self, err = m.Memory.Self(s.GuildID); err != nil {
		return k, err
	}
	if k.Self.Lately == "" && m.Character != nil {
		k.Self.Lately = m.Character.Lately
	}

	ids := presentIDs(s)
	for _, id := range also {
		if id != "" && !contains(ids, id) {
			ids = append(ids, id)
		}
	}
	for _, id := range ids {
		p, ok, err := m.Memory.Person(s.GuildID, id)
		if err != nil {
			return k, err
		}
		if !ok {
			p = memory.Person{ID: id, Name: nameOf(s, id)}
		}
		if name := nameOf(s, id); name != "" {
			p.Name = name
		}
		k.People = append(k.People, p)
	}

	days, err := m.Memory.Days(s.GuildID, s.Now, recentSummary+1)
	if err != nil {
		return k, err
	}
	for _, d := range days {
		if d.Summary != "" && dayDiff(d.Date, s.Now) > 0 {
			k.Days = append(k.Days, d)
		}
	}

	// Moments inside the live transcript are already in front of her;
	// recalled ones start where it does. Only in this room, though: what
	// happened elsewhere while this conversation ran is not in front of her
	// at all.
	before := s.Now
	if len(s.Turns) > 0 {
		before = s.Turns[0].At
	}
	for _, d := range days {
		for _, mo := range d.Moments {
			if mo.Weight >= heavyWeight && mo.At.After(k.LastHeavy) && !mo.At.After(s.Now) {
				k.LastHeavy = mo.At
			}
		}
	}

	pool, err := m.Memory.Recall(s.GuildID, s.Now, before, s.ChannelName, topicWords(s.Turns), ids, recallDays, recallPool)
	if err != nil {
		return k, err
	}
	k.Recalled = capRecall(m.drift(s, pool, ids), s.RecallCap)

	threads, err := m.Memory.Threads(s.GuildID)
	if err != nil {
		return k, err
	}
	k.Threads = memory.Unfinished(threads)

	if k.Arc, err = m.freshArc(s); err != nil {
		return k, err
	}

	words := topicWords(s.Turns)
	if m.Character != nil {
		k.Specifics = pickSpecifics(m.Character.Specifics, words)
	}
	if m.SelfFacts {
		me, err := m.Memory.Me(s.GuildID)
		if err != nil {
			return k, err
		}
		k.SelfFacts = pickSelfFacts(me.Facts, words)
	}
	return k, nil
}

// presentIDs is everyone in the scene, the person it is about first.
func presentIDs(s Scene) []string {
	seen := make(map[string]bool)
	var ids []string
	add := func(id string) {
		if id != "" && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	add(s.UserID)
	for i := len(s.Turns) - 1; i >= 0; i-- {
		if !s.Turns[i].FromBot {
			add(s.Turns[i].UserID)
		}
	}
	return ids
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// nameOf is what someone is called in the scene.
func nameOf(s Scene, id string) string {
	if id == s.UserID && s.Username != "" {
		return s.Username
	}
	for i := len(s.Turns) - 1; i >= 0; i-- {
		if s.Turns[i].UserID == id && s.Turns[i].Username != "" {
			return s.Turns[i].Username
		}
	}
	return ""
}

// topicWords are the words of the end of the conversation, which decide what
// comes back from further away.
func topicWords(turns []Turn) []string {
	if len(turns) > transcriptTail {
		turns = turns[len(turns)-transcriptTail:]
	}
	var b strings.Builder
	for _, t := range turns {
		b.WriteString(t.Content + " ")
	}
	return memory.Keywords(b.String())
}

// generate asks the provider, reporting which backend answered when the
// provider can say.
func (m *Mind) generate(ctx context.Context, msgs []ai.Message) (string, string, error) {
	return generateWith(ctx, m.Provider, msgs)
}

// speak is generate through her voice.
func (m *Mind) speak(ctx context.Context, msgs []ai.Message) (string, string, error) {
	if m.ReplyTokens > 0 {
		ctx = ai.WithMaxTokens(ctx, m.ReplyTokens)
	}
	if m.Voice != nil {
		return generateWith(ctx, m.Voice, msgs)
	}
	return generateWith(ctx, m.Provider, msgs)
}

func generateWith(ctx context.Context, p ai.Provider, msgs []ai.Message) (string, string, error) {
	type named interface {
		GenerateNamed(ctx context.Context, messages []ai.Message) (string, string, error)
	}
	if n, ok := p.(named); ok {
		return n.GenerateNamed(ctx, msgs)
	}
	reply, err := p.Generate(ctx, msgs)
	return reply, "", err
}

// name is what she is called in a scene.
func (m *Mind) name(s Scene) string {
	if s.SelfName != "" {
		return s.SelfName
	}
	if m.Character != nil {
		return m.Character.Name
	}
	return "her"
}
