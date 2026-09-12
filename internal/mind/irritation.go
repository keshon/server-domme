package mind

import (
	"fmt"
	"math"
	"time"
)

// Irritation tuning.
const (
	// IrritationHalflife is how long it takes for half of an annoyance to
	// wear off. Long enough that coming back an hour later still finds her
	// cool, short enough that it is gone by the next day — which is roughly
	// how long a person stays annoyed about something small.
	IrritationHalflife = 90 * time.Minute
	// pesterStep is how much one push after being ignored adds.
	pesterStep = 0.34
	// PesterWindow is how soon after being passed over a second approach
	// counts as pushing rather than as a new conversation.
	PesterWindow = 4 * time.Minute
	// irritationFloor is the point below which it is not worth carrying.
	irritationFloor = 0.05

	// coolAbove and sharpAbove are the two bands that produce a directive.
	coolAbove  = 0.30
	sharpAbove = 0.65
)

// IrritationNow decays a stored irritation to what it is at now.
//
// Computed on read from a timestamp, like everything else here: there is no
// sweeper, nothing to keep in step, and a restart loses nothing because the
// timestamp is what was stored.
func IrritationNow(stored float64, at, now time.Time) float64 {
	if stored <= 0 || at.IsZero() {
		return 0
	}
	elapsed := now.Sub(at)
	if elapsed <= 0 {
		return clamp01(stored)
	}

	level := stored * math.Pow(0.5, float64(elapsed)/float64(IrritationHalflife))
	if level < irritationFloor {
		return 0
	}
	return clamp01(level)
}

// Pester raises irritation by one push.
//
// Deliberately not a judgement about tone. Deciding whether a message was rude
// needs a model call per message, which is not affordable and which the
// experiment this design came from showed answering confidently and
// arbitrarily. What is countable is behaviour: being passed over and
// immediately pressing again is pushing, in any language and with no
// interpretation at all.
func Pester(current float64) float64 {
	return clamp01(current + pesterStep)
}

// IrritationDirective turns irritation with one person into an instruction, or
// "" when there is nothing worth saying.
//
// Named, because the whole point of holding this per person is that it applies
// to them and not to the room. Someone else speaking to her while she is short
// with one member should find her ordinary.
func IrritationDirective(name string, level float64) string {
	if name == "" {
		return ""
	}
	switch {
	case level > sharpAbove:
		return fmt.Sprintf(
			"%s has been pushing at you. Be short with them — answer if you answer at all, "+
				"and do not pretend to be pleased about it.", name)
	case level > coolAbove:
		return fmt.Sprintf("You are a little tired of %s just now. Be cooler with them than usual.", name)
	default:
		return ""
	}
}

// IrritationNudge is how much irritation lowers the odds of answering this
// person, as a negative number.
//
// Applies to them alone. It is deliberately smaller than the mood nudge: being
// annoyed makes someone terser more than it makes them silent, and a character
// who simply stops responding reads as a broken bot rather than an irritated
// person — which is the failure this whole layer keeps having to avoid.
func IrritationNudge(level float64) float64 {
	if level <= coolAbove {
		return 0
	}
	return -0.15 * clamp01(level)
}
