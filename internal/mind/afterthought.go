package mind

import (
	"fmt"
	"strings"
	"time"
	"unicode"
)

// TriggerAfterthought is a second message she sends a few seconds after her
// own reply, when something more occurs to her.
//
// People double-text. "are you bored?" gets "yes", and then, a beat later,
// "you could fix that". A bot that sends exactly one message per approach,
// every time, has a rhythm no person has, and that is noticeable long before
// anything it says is.
const TriggerAfterthought Trigger = "afterthought"

// Owed reports whether a failure to deliver should be retried later.
//
// Only an answer is owed. Something she volunteered, or a second thought about
// what she just said, belongs to its moment: delivered late it is stranger
// than never said at all.
func Owed(t Trigger) bool {
	return !Volunteered(t) && t != TriggerAfterthought
}

// Afterthought limits.
const (
	// afterthoughtMaxWords is the longest first reply that can have one. A
	// reply that already said its piece has nothing to add a beat later; the
	// double-text follows a clipped first answer.
	afterthoughtMaxWords = 6

	// afterthoughtChance is the odds once every gate has passed, before mood
	// and regard move it.
	afterthoughtChance = 0.35

	// tooTiredForAfterthought is the energy below which "yes" stays "yes".
	tooTiredForAfterthought = 0.35

	// tooIrritatedForAfterthought is how annoyed she can be with someone and
	// still give them more than the first word. A curt reply to someone who
	// irritates her is curt on purpose.
	tooIrritatedForAfterthought = 0.3

	// coldRegard is the standing below which she gives someone nothing
	// beyond what they asked for.
	coldRegard = -0.25

	// afterthoughtMinDelay and afterthoughtSpread bound the pause before it.
	// The pause is most of what makes it read as a second thought rather than
	// one message split in two.
	afterthoughtMinDelay = 3 * time.Second
	afterthoughtSpread   = 5 * time.Second
)

// Afterthought is what decides whether she adds a second message.
type Afterthought struct {
	// Trigger is what her first reply answered.
	Trigger Trigger
	// Late marks a first reply that was held back; its moment has already
	// passed once.
	Late bool
	// Reply is what she just said.
	Reply string
	Now   time.Time
	// Turns are the conversation, her reply included.
	Turns []Turn
	// UserID is who she was answering.
	UserID string
	Drives Drives
	// Tension and Regard are how she feels about that person, already
	// decayed and combined.
	Tension float64
	Regard  float64
	// Fatigue is how much she has put herself forward lately. It is what
	// keeps double-texting rare: the third in ten minutes is a pattern, and
	// a pattern is exactly what this exists to break.
	Fatigue float64
}

// MayAddAfterthought decides, gates before the roll, whether a second
// message is allowed at all, and returns the chance she had. Whether one is
// actually sent is then up to the model, which is told it may decline; see
// AfterthoughtDirective.
func MayAddAfterthought(a Afterthought, roll float64) (bool, float64) {
	if a.Late || !Owed(a.Trigger) {
		return false, 0
	}
	if words := len(strings.Fields(a.Reply)); words == 0 || words > afterthoughtMaxWords {
		return false, 0
	}
	// Zero drives mean unset, not exhausted; see Drives.Nudge.
	if a.Drives != (Drives{}) && a.Drives.Energy < tooTiredForAfterthought {
		return false, 0
	}
	if a.Tension >= tooIrritatedForAfterthought || a.Regard <= coldRegard {
		return false, 0
	}
	// One person and her. With others talking, a second line from her lands
	// in the middle of their exchange rather than after her own.
	if busyRoom(a.Turns, a.UserID, a.Now) {
		return false, 0
	}

	pull := afterthoughtChance + a.Drives.Nudge() + RegardNudge(a.Regard)
	chance := clamp01(pull) * Rested(a.Fatigue)
	return roll < chance, chance
}

// AfterthoughtDelay is how long she pauses before the second message, from a
// roll in [0, 1).
func AfterthoughtDelay(roll float64) time.Duration {
	return afterthoughtMinDelay + time.Duration(roll*float64(afterthoughtSpread))
}

// AfterthoughtSkip is the word the model answers with when nothing more
// occurs to her.
const AfterthoughtSkip = "SKIP"

// AfterthoughtDirective asks for the second message, or for nothing.
//
// Declining has to be allowed and has to be cheap to say, or every permitted
// afterthought becomes a sent one and the gate above is the only thing
// deciding. The shapes listed are the ones that suit her; a curious "why?"
// is what a model reaches for unprompted, and from this character it reads as
// needy.
func AfterthoughtDirective(first string) string {
	return fmt.Sprintf(
		"You just sent: %q. A few seconds later one more short line occurs to "+
			"you — the thing you add once you have thought about it for a moment: "+
			"a demand, a jab, the real reason. Do not open by repeating what you "+
			"said, and do not explain it. One line. If nothing worth adding comes to "+
			"you, reply with only %s.", strings.TrimSpace(first), AfterthoughtSkip)
}

// IsSkip reports whether the model declined to add anything.
//
// On the first word rather than the whole reply: models decorate the token
// they were told to answer with — "SKIP.", "(skip)", "SKIP — nothing to add".
// A real line that happens to open with "skip" is lost, which costs nothing:
// the afterthought was optional to begin with.
func IsSkip(reply string) bool {
	fields := strings.Fields(reply)
	if len(fields) == 0 {
		return true
	}
	first := strings.TrimFunc(fields[0], func(r rune) bool {
		return !unicode.IsLetter(r)
	})
	return strings.EqualFold(first, AfterthoughtSkip)
}

// SameLine reports whether two lines say the same thing, give or take case
// and punctuation. An afterthought that repeats the first message is a stutter.
func SameLine(a, b string) bool {
	return echoKey(a) != "" && echoKey(a) == echoKey(b)
}

// LastWord reports whether her message is still the latest in the
// conversation.
//
// Checked before the afterthought is generated and again before it is sent.
// If the person has answered in the meantime the moment has passed, and a
// second line arriving after their reply is the tell this whole feature exists
// to avoid.
func LastWord(turns []Turn, messageID string) bool {
	if len(turns) == 0 || messageID == "" {
		return false
	}
	last := turns[len(turns)-1]
	return last.FromBot && last.MessageID == messageID
}
