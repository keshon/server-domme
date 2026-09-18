package mind

import (
	"math"
	"time"
)

// Bond is how she stands with one person, beyond knowing them.
//
// Three values, each stored as it stood at a moment and decayed on read, the
// way everything here is. Closeness is slow — liking someone builds over
// conversations and fades over weeks. Tension is fast — being annoyed with
// someone passes in an afternoon. Welcome is learned from how they take it
// when she comes to them: answered warmly, it rises; left unanswered, it
// falls. Missing them is not stored at all; it is read from time and
// closeness, see FeelLonging.
//
// It replaces irritation and warmth as separate mechanisms with their own
// ways in. Everything that moves her about a person is now an Event in one
// table (appraisals), so a new reason to feel something is a row, not a
// subsystem, and two reasons cannot quietly stack without either knowing.
type Bond struct {
	Closeness   float64
	ClosenessAt time.Time
	Tension     float64
	TensionAt   time.Time
	Welcome     float64
	WelcomeAt   time.Time
}

// Welcome tuning. It starts neutral rather than at zero: someone who has just
// opted in has not yet shown whether they want her, and treating them as
// unwelcoming from the start would mean she never learns otherwise.
const (
	welcomeNeutral  = 0.5
	welcomeHalflife = 14 * 24 * time.Hour
)

// Now is the bond decayed to the present: closeness and tension towards zero
// on their own half-lives, welcome back towards neutral.
func (b Bond) Now(now time.Time) (closeness, tension, welcome float64) {
	return ClosenessNow(b.Closeness, b.ClosenessAt, now),
		TensionNow(b.Tension, b.TensionAt, now),
		welcomeNow(b.Welcome, b.WelcomeAt, now)
}

func welcomeNow(stored float64, at, now time.Time) float64 {
	if at.IsZero() {
		return welcomeNeutral
	}
	elapsed := now.Sub(at)
	if elapsed <= 0 {
		return clamp01(stored)
	}
	f := math.Pow(0.5, float64(elapsed)/float64(welcomeHalflife))
	return clamp01(welcomeNeutral + (stored-welcomeNeutral)*f)
}

// Event is something that happened between her and one person.
type Event string

const (
	// EventPestered is being approached again straight after she ignored
	// their direct approach.
	EventPestered Event = "pestered"
	// EventBrushedOff is her question ignored for someone else.
	EventBrushedOff Event = "brushed off"
	// EventLaughed and EventPanned are how her last line landed.
	EventLaughed Event = "laughed at her line"
	EventPanned  Event = "panned her line"
	// Remembered conversations, by tone and by whether it was just the two
	// of them. Attribution is the hard half: a group conversation that went
	// badly does not say who made it go badly, so it moves nobody's tension.
	EventWarmAlone     Event = "a warm conversation, one to one"
	EventWarmGroup     Event = "a warm conversation, in company"
	EventOrdinaryAlone Event = "an ordinary conversation, one to one"
	EventTenseAlone    Event = "a tense conversation, one to one"
	EventHostileAlone  Event = "a hostile conversation, one to one"
	// Reaching out: how they took it.
	EventReachAnswered        Event = "answered her"
	EventReachAnsweredQuickly Event = "answered her quickly"
	EventReachIgnored         Event = "ignored her reaching out"
	EventAskedForPeace        Event = "asked her to back off"
	// EventInitiativeTaken and EventInitiativeDropped are how something she
	// started with them, other than reaching out, was received; see Payoff.
	EventInitiativeTaken   Event = "took up something she started"
	EventInitiativeDropped Event = "let something she started drop"
)

// Shift is what one event does to a bond, and Mood what it does to her
// generally: the spillover that lets someone getting on her nerves leave her
// a little short with everyone for a while. Smaller than the bond's share,
// and it fades sooner; see MoodSwing.
type Shift struct {
	Closeness, Tension, Welcome float64
	Mood                        float64
}

// appraisals is the whole of how events move a bond, in one place.
//
// The values are the ones each mechanism had when it was separate, carried
// over so this reorganisation changes structure and not behaviour; re-tuning
// them is a separate, measured step.
//
// Conversations carry no mood here, because they are appraised once for
// every person in them and the mood is one: see ConversationMood.
var appraisals = map[Event]Shift{
	EventPestered:             {Tension: pesterStep, Mood: -0.05},
	EventBrushedOff:           {Tension: BrushOffStep, Mood: -0.08},
	EventLaughed:              {Closeness: LikedWarmth, Mood: 0.08},
	EventPanned:               {Tension: PannedIrritation, Mood: -0.12},
	EventWarmAlone:            {Closeness: 0.15},
	EventWarmGroup:            {Closeness: 0.05},
	EventOrdinaryAlone:        {Closeness: 0.03},
	EventTenseAlone:           {Tension: 0.2},
	EventHostileAlone:         {Closeness: -0.15, Tension: 0.4},
	EventReachAnswered:        {Closeness: 0.02, Welcome: 0.10, Mood: 0.06},
	EventReachAnsweredQuickly: {Closeness: 0.03, Welcome: 0.15, Mood: 0.10},
	EventReachIgnored:         {Welcome: -0.12, Mood: -0.06},
	EventAskedForPeace:        {Welcome: -0.30, Mood: -0.12},
	// Smaller than reaching out's: a remark in a room they are already in
	// says less about whether they want her than coming to find them does.
	// No mood here; the surprise carries that. See SurpriseMood.
	EventInitiativeTaken:   {Welcome: 0.05},
	EventInitiativeDropped: {Welcome: -0.04},
}

// ConversationMood is what a remembered conversation does to her mood, once
// for the conversation however many were in it. Unlike the bond, a group
// counts: a room that turned hostile sours her even when nobody in it can
// fairly be blamed.
func ConversationMood(tone Tone) float64 {
	switch tone {
	case ToneWarm:
		return 0.12
	case ToneTense:
		return -0.08
	case ToneHostile:
		return -0.2
	default:
		return 0
	}
}

// Appraise is what an event does to a bond, or the zero shift for one that
// does nothing.
func Appraise(e Event) Shift { return appraisals[e] }

// Apply decays the bond to now, adds the event's shift and stamps it. Only
// the values the event moves are restamped, so an event about welcome does
// not reset how long tension has been fading.
func (b Bond) Apply(e Event, now time.Time) Bond {
	s := Appraise(e)
	closeness, tension, welcome := b.Now(now)
	if s.Closeness != 0 {
		b.Closeness, b.ClosenessAt = clamp01(closeness+s.Closeness), now
	}
	if s.Tension != 0 {
		b.Tension, b.TensionAt = clamp01(tension+s.Tension), now
	}
	if s.Welcome != 0 {
		b.Welcome, b.WelcomeAt = clamp01(welcome+s.Welcome), now
	}
	return b
}

// ConversationEvent is the event a remembered conversation amounts to for one
// of the people in it, or "" for none.
func ConversationEvent(tone Tone, alone bool) Event {
	switch {
	case tone == ToneWarm && alone:
		return EventWarmAlone
	case tone == ToneWarm:
		return EventWarmGroup
	case tone == ToneOrdinary && alone:
		return EventOrdinaryAlone
	case tone == ToneTense && alone:
		return EventTenseAlone
	case tone == ToneHostile && alone:
		return EventHostileAlone
	default:
		return ""
	}
}

// ReceptionEvent is the event a reaction to her line amounts to, or "".
func ReceptionEvent(r Reception) Event {
	switch r {
	case ReceptionLiked:
		return EventLaughed
	case ReceptionPanned:
		return EventPanned
	default:
		return ""
	}
}
