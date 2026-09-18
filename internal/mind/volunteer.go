package mind

import (
	"fmt"
	"strings"
	"time"
)

// Things she says without being asked.
//
// Ranked by how much a real person's action justifies them, which is the whole
// of the design. A regular walking back in after weeks away is an event;
// someone raising a subject she remembers is a reason. Posting into a quiet
// channel because a timer fired is neither, and it is deliberately absent: the
// framework this replaced did exactly that — a three per cent chance every ten
// minutes — and it was the most bot-like thing it did.
const (
	// TriggerReturn is a regular speaking again after a long absence.
	TriggerReturn Trigger = "return"
	// TriggerRecall is the room talking about something she remembers.
	TriggerRecall Trigger = "recall"
)

// Volunteered reports whether a trigger is something she started rather than
// something she was asked. The difference decides what happens when no
// backend answers: an answer owed to a person is held and delivered late, and
// an unprompted remark is simply dropped, because one arriving twenty minutes
// after the moment it was about is stranger than silence.
func Volunteered(t Trigger) bool {
	return t == TriggerReturn || t == TriggerRecall || t == TriggerReach
}

// Proactivity tuning. How often she speaks up is decided by her initiative
// fatigue (see Fatigue); the daily maximum is only a floor under that.
const (
	// VolunteerDailyMax is a safety limit on unprompted remarks in one
	// channel a day, in the community's own timezone. Set where fatigue
	// should never let her reach it, so it is never what decides.
	VolunteerDailyMax = 6

	// busyRoomWindow and busyRoomVoices describe two other people in the
	// middle of an exchange, which is not a moment anyone should cut into
	// with something they remembered — the same barging the "about" trigger
	// was split out to prevent.
	busyRoomWindow = 2 * time.Minute
	busyRoomVoices = 2

	// tooTiredToVolunteer is the energy below which she only speaks when
	// spoken to.
	tooTiredToVolunteer = 0.25

	// returnChance and recallChance are the odds once every gate has passed.
	// A returning regular is worth more than a subject: noticing someone is
	// what people do, bringing up a memory is what people sometimes do.
	returnChance = 0.60
	recallChance = 0.35

	// recallOverlap is the share of a memory's keywords the room has to be
	// using before it counts as the same subject. A third: less than that is
	// a shared word, not a shared topic.
	recallOverlap = 0.34
	// minRecallAge keeps her from "remembering" the conversation she is in.
	// The memory writer runs minutes after a conversation settles, so without
	// this the freshest memory always matches the room it came from, and she
	// would bring up what was said an hour ago as though it were history.
	minRecallAge = 6 * time.Hour
)

// Volunteer is everything the decision to speak unprompted looks at.
type Volunteer struct {
	Trigger Trigger
	Now     time.Time

	// Enabled is whether this channel has been opted into proactivity at
	// all. Separate from being let into the channel: answering and speaking
	// unprompted are different postures, and a server can want one without
	// the other.
	Enabled bool
	// Today is how many unprompted remarks this channel has had today.
	Today int
	// LastSpokeAt is when she last said anything here, prompted or not.
	LastSpokeAt time.Time
	// EngagedWindow is how recently she must have spoken to count as already
	// in the conversation.
	EngagedWindow time.Duration

	// Turns is the live conversation and UserID the person whose message
	// prompted the thought, so the busy-room check can tell other people
	// talking to each other from that person talking.
	Turns  []Turn
	UserID string

	Drives Drives
	// Fatigue is how much she has put herself forward lately.
	Fatigue float64
	// WelcomeShift is how glad the person this is about has been of her,
	// from neutral; zero when she has learned nothing or it is about the
	// room. See WelcomeShift.
	WelcomeShift float64
}

// MayVolunteer decides whether she speaks unprompted, given a roll in [0,1),
// and returns the chance she had.
//
// The gates are about the moment — already talking, too tired, other people
// mid-exchange — and the odds are about her: the trigger's pull, her mood,
// and how much she has put herself forward lately.
func MayVolunteer(v Volunteer, roll float64) (bool, float64) {
	if !v.Enabled || !Volunteered(v.Trigger) {
		return false, 0
	}
	if v.Today >= VolunteerDailyMax {
		return false, 0
	}
	// Already in the conversation: whatever she says next is a reply, and
	// the follow-up and mention triggers own that.
	if !v.LastSpokeAt.IsZero() && v.Now.Sub(v.LastSpokeAt) < v.EngagedWindow {
		return false, 0
	}
	// Zero drives mean unset, not exhausted; see Drives.Nudge.
	if v.Drives != (Drives{}) && v.Drives.Energy < tooTiredToVolunteer {
		return false, 0
	}
	if v.Trigger == TriggerRecall && busyRoom(v.Turns, v.UserID, v.Now) {
		return false, 0
	}

	pull := recallChance
	if v.Trigger == TriggerReturn {
		pull = returnChance
	}
	chance := clamp01(welcomed(clamp01(pull+v.Drives.Nudge()), v.WelcomeShift)) * Rested(v.Fatigue)
	return roll < chance, chance
}

// busyRoom reports whether other people are in the middle of an exchange.
func busyRoom(turns []Turn, promptedBy string, now time.Time) bool {
	voices := make(map[string]bool)
	for _, t := range turns {
		if t.FromBot || t.UserID == "" || now.Sub(t.At) > busyRoomWindow {
			continue
		}
		voices[t.UserID] = true
	}
	// The person whose message prompted this is part of the room, but two
	// other people as well is a conversation she was not part of.
	delete(voices, promptedBy)
	return len(voices) >= busyRoomVoices
}

// Relevant finds the memory the room is currently closest to, if any is close
// enough to count as the same subject.
//
// Word overlap, like the rest of recall, so it matches words rather than
// meanings: "the purge rules" will not surface for "channel cleanup". That
// errs towards silence, which is the right direction for something she was not
// asked to say.
func Relevant(memories []Memory, now time.Time, topic []string) (Memory, bool) {
	if len(topic) == 0 {
		return Memory{}, false
	}

	var best Memory
	var bestOverlap float64
	for _, m := range memories {
		if strings.TrimSpace(m.Gist) == "" || now.Sub(m.At) < minRecallAge {
			continue
		}
		if m.Brightness(now, topic, nil) < brightnessFloor {
			continue
		}
		// Against the gist alone. Overlap is a share of the memory's words, so
		// counting the detail too made a vividly remembered episode harder to
		// bring up than a vague one — backwards. The gist is what it was about,
		// which is the thing the room has to be on.
		overlap := wordOverlap(topic, Keywords(m.Gist))
		if overlap >= recallOverlap && overlap > bestOverlap {
			best, bestOverlap = m, overlap
		}
	}
	return best, bestOverlap > 0
}

// ReturnDirective is why she is speaking when a regular comes back.
func ReturnDirective(name string, away time.Duration) string {
	if name == "" {
		name = "someone"
	}
	return fmt.Sprintf(
		"Nobody asked you anything. %s has just come back after %s away, and you "+
			"noticed. Say something to them about being back — that they were gone "+
			"is the point, so do not skip it. One line, in your own way, without "+
			"making a ceremony of it. You do not know what happened while they were "+
			"away, so do not tell them.", name, roughDuration(away))
}

// RecallDirective is why she is speaking when the room reminds her of
// something.
//
// Told not to recount it, because the pull of a memory placed in front of a
// model is to narrate it back, and "that reminds me of the time…" followed by
// a paragraph is exactly how an unprompted remark outstays its welcome.
func RecallDirective(gist string) string {
	return fmt.Sprintf(
		"Nobody asked you anything. You are speaking because this has come up "+
			"before, and you were there: %s. Your whole message is a dry remark that "+
			"it is back — the kind of line someone who remembers last time drops in "+
			"passing. Leave their question to them. One line.",
		strings.TrimSpace(gist))
}
