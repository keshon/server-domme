package mind

import "time"

// How strongly an opportunity draws her: the event's own strength times how
// much the thing it touches matters to her.
//
// Each kind of thing she starts used to carry a constant — a returning
// regular 0.6, an old subject coming round 0.35, a second thought 0.35 — so a
// regular she had never warmed to was greeted as readily as one she had
// missed for a month, and a faded memory was brought up as readily as a
// charged one. Salience (see OnHerMind) is how much a person or a memory is
// on her mind; these multiply it by how strong the event is, and what comes
// out is still weighed by mood, welcome and fatigue like everything else she
// starts.

// Pull tuning.
const (
	// absenceStrength is how long past the point a return is noticed at all
	// it takes to count in full: two weeks away is an event, two months is a
	// bigger one.
	absenceStrength = 60 * 24 * time.Hour
	// recallStrengthFloor is how strong the least matching subject she would
	// bring up is, against 1 for the room using every word of it.
	recallStrengthFloor = 0.5
	// recallReach is the most a subject can draw her, at full strength and
	// on her mind as much as a subject can be: less than a returning person
	// can. A day-old memory of middling weight comes to about 0.2 to 0.3,
	// around the constant it replaces; a charged, fresh one to about 0.7.
	recallReach = 0.8
	// afterthoughtFloor and afterthoughtReach bound a second thought's pull
	// between someone who is barely on her mind and someone who is wholly
	// on it.
	afterthoughtFloor = 0.2
	afterthoughtReach = 0.35
)

// familiarAttention is how much a returning face draws her before any
// feeling about them: a regular walking back in is noticeable whoever they
// are to her, and someone she has only half met less so.
var familiarAttention = map[Familiarity]float64{
	FamiliarityRegular: 0.55,
	FamiliarityKnown:   0.35,
}

// ReturnPull is how strongly someone coming back after a long absence draws
// her: the longer they were gone the stronger the event, and the more they
// are on her mind — missed, liked, resented — the stronger her response.
//
// Never for a newcomer: someone who said two things a month ago and came back
// is a stranger, and greeting a stranger's return reads as surveillance
// rather than as recognition.
func ReturnPull(away time.Duration, f Familiarity, salience float64) float64 {
	if away < absenceGap || f == FamiliarityNewcomer {
		return 0
	}
	event := 0.6 + 0.4*clamp01(float64(away-absenceGap)/float64(absenceStrength))
	base := familiarAttention[f]
	return clamp01(event * (base + (1-base)*clamp01(salience)))
}

// RecallPull is how strongly an old subject coming round draws her: how much
// of it the room is using, times how much that memory still matters to her.
func RecallPull(overlap, salience float64) float64 {
	if overlap < recallOverlap {
		return 0
	}
	event := recallStrengthFloor + (1-recallStrengthFloor)*clamp01((overlap-recallOverlap)/(1-recallOverlap))
	return clamp01(event * recallReach * clamp01(salience/subjectCeiling))
}

// AfterthoughtPull is how strongly a second thought draws her after her own
// short reply: more with someone on her mind, since a clipped answer to
// someone she cares about is the one she is likeliest to add to.
func AfterthoughtPull(salience float64) float64 {
	return afterthoughtFloor + afterthoughtReach*clamp01(salience)
}
