package mind

import (
	"fmt"
	"strings"
)

// Regard is how well-disposed she is towards someone because of what they are
// here, on -1 to +1, with 0 meaning nothing in particular.
//
// Set per Discord role rather than per person: a server that has roles at all
// has already decided who is what, and asking an operator to rate three
// hundred members one at a time is asking them not to use the feature. What it
// models is standing, not worth — a negative regard is the reserve you show
// someone whose role has not earned any yet, and the bands below deliberately
// stop short of hostility, which is what irritation is for and has to be
// earned by behaviour.
type Regard float64

// Regard bands. Narrow around zero, so a role set to a small value says
// nothing rather than producing a sentence nobody meant.
const (
	regardWarmAbove  = 0.25
	regardCoolBelow  = -0.25
	regardStrongOver = 0.65
)

// Combine sums the regard of every role someone holds, clamped.
//
// Summed rather than averaged so two roles that both count for something add
// up, which is how anyone reads a person wearing several. Clamped because
// three favourable roles should not make her more forthcoming than any single
// role could.
func Combine(values []float64) float64 {
	var total float64
	for _, v := range values {
		total += v
	}
	switch {
	case total > 1:
		return 1
	case total < -1:
		return -1
	default:
		return total
	}
}

// RegardNudge is how much standing moves the odds of answering someone.
//
// Smaller than the mood and about the size of irritation. A role should colour
// how she treats people, not decide whether they can talk to her at all: a
// member who cannot get an answer because of a role they were given has no way
// to tell that from a broken bot.
func RegardNudge(regard float64) float64 {
	return 0.12 * clampSigned(regard)
}

// RegardDirective turns standing into an instruction about one person.
//
// The note is used verbatim when there is one, because a sentence an operator
// wrote about their own server — "a submissive here, speak to them as one" —
// says something no scalar can, and the whole of this file is evidence that
// authored text outperforms text derived from a number. The bands are the
// fallback for a role set without one.
func RegardDirective(name, note string, regard float64) string {
	if name == "" {
		return ""
	}

	if note = strings.TrimSpace(note); note != "" {
		return fmt.Sprintf("%s — %s", name, note)
	}

	switch {
	case regard > regardStrongOver:
		return fmt.Sprintf("%s has standing with you. Give them a real answer.", name)
	case regard > regardWarmAbove:
		return fmt.Sprintf("You think well enough of %s. Be forthcoming with them.", name)
	case regard < -regardStrongOver:
		return fmt.Sprintf("%s has no standing with you. Say as little as will do.", name)
	case regard < regardCoolBelow:
		return fmt.Sprintf("You have little time for %s. Keep it brief.", name)
	default:
		return ""
	}
}

func clampSigned(v float64) float64 {
	switch {
	case v > 1:
		return 1
	case v < -1:
		return -1
	default:
		return v
	}
}
