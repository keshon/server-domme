package mind

import "time"

// Reward: what happens after she puts herself forward, and what it does to
// her.
//
// Fatigue is the cost of taking the initiative and is paid whatever happens.
// This is the other half, the one dopamine carries in a person: not the
// outcome itself but how it compares with what she expected. A warm answer
// from someone she expected nothing from is a real reward; the same answer
// from someone who always answers warmly is barely one; silence from someone
// who always answers stings more than silence from a stranger.
//
// The expectation is her welcome with that person — learned first from
// reaching out, now from everything she starts with them — so one number is
// both what she learns and what she predicts from. The surprise moves her
// mood. The need that drove her is satisfied or not by the outcome itself,
// not by her speaking: see MoodInput.LastContact.

// Payoff is how something she started was received.
type Payoff string

const (
	// PayoffLaughed is a laugh at what she said.
	PayoffLaughed Payoff = "laughed"
	// PayoffEngaged is the person, or the room, taking it up.
	PayoffEngaged Payoff = "engaged"
	// PayoffPanned is an answer that told her it fell flat.
	PayoffPanned Payoff = "panned"
	// PayoffIgnored is nothing, within the window.
	PayoffIgnored Payoff = "ignored"
)

// Reward tuning.
const (
	// PayoffWindow is how long she waits to see how something landed. Ten
	// minutes: in a live channel an answer comes in that time or not at all.
	PayoffWindow = 10 * time.Minute
	// surpriseMood is how far the difference between what she got and what
	// she expected moves her mood. Of the same order as a laugh's own share,
	// so a laugh she did not expect counts roughly double.
	surpriseMood = 0.15
)

// Value is how rewarding an outcome is, 0..1, on the same scale as welcome.
func (p Payoff) Value() float64 {
	switch p {
	case PayoffLaughed:
		return 1
	case PayoffEngaged:
		return 0.7
	case PayoffPanned:
		return 0.1
	default:
		return 0
	}
}

// PayoffOf reads how an answer to something she started received it.
func PayoffOf(content string) Payoff {
	switch ReadReception(content) {
	case ReceptionLiked:
		return PayoffLaughed
	case ReceptionPanned:
		return PayoffPanned
	default:
		return PayoffEngaged
	}
}

// Surprise is the outcome against what she expected, -1..+1.
func Surprise(p Payoff, expected float64) float64 {
	return p.Value() - clamp01(expected)
}

// SurpriseMood is how far a surprise moves her mood.
func SurpriseMood(surprise float64) float64 {
	return surpriseMood * clampSigned(surprise)
}

// PayoffEvent is what an outcome does to her bond with the person it was
// aimed at: welcome learned, a little, from anything she starts. Reaching
// out has its own, stronger events, since coming into someone's day uninvited
// says more about how welcome she is than a remark in a room they are in.
func PayoffEvent(p Payoff) Event {
	switch p {
	case PayoffLaughed, PayoffEngaged:
		return EventInitiativeTaken
	default:
		return EventInitiativeDropped
	}
}

// WelcomeShift is how far welcome sits from neutral, -0.5..+0.5: the zero
// value is "has learned nothing", which is what callers that do not know the
// person should pass.
func WelcomeShift(welcome float64) float64 {
	return clamp01(welcome) - welcomeNeutral
}

// welcomed scales a pull by how welcome she has learned she is: unchanged
// at neutral, up to half again for someone always glad of her, down to half
// for someone who never is. The same factor reaching out uses.
func welcomed(pull, shift float64) float64 {
	return pull * (1 + clampSigned(2*shift)/2)
}
