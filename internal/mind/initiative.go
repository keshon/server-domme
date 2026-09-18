package mind

import (
	"math"
	"time"
)

// Taking the initiative: everything she starts rather than answers.
//
// Speaking up when a regular comes back or an old subject returns, adding a
// second line to her own reply, and going after someone who opted in used to
// have a cap and a cooldown each — three a day and forty-five minutes apart,
// twenty minutes between double-texts, and so on. Each was reasonable alone,
// and together they were a timetable: anyone watching long enough learns
// when she cannot speak, and nobody real has edges like that.
//
// Now each of those is an opportunity with a pull of its own, and one thing
// decides them all: how much she has put herself forward lately. Fatigue
// rises every time she takes the initiative and wears off with time, so
// having just spoken up she is less likely to again, and a quiet afternoon
// leaves her ready. The spacing comes out uneven, the way a person's does.
// Fatigue is hers, per server, not per channel or per kind: a mind that has
// just chased one person is less inclined to chime in somewhere else.
const (
	// fatigueHalflife is how long putting herself forward takes to half wear
	// off.
	fatigueHalflife = 4 * time.Hour
	// fatigueFloor is where what is left counts as none.
	fatigueFloor = 0.02
)

// initiativeCost is how much each kind of initiative tires her. Speaking up
// unprompted costs the most, being the most exposed thing she does; a second
// line after her own reply costs the least, since she is already talking.
var initiativeCost = map[Trigger]float64{
	TriggerReturn:       0.6,
	TriggerRecall:       0.6,
	TriggerReach:        0.4,
	TriggerAfterthought: 0.3,
}

// Fatigue is how much she has put herself forward lately, stored with when
// it was last changed and decayed on read.
type Fatigue struct {
	Level float64
	At    time.Time
}

// Now is the fatigue left at now.
func (f Fatigue) Now(now time.Time) float64 {
	level := decayed(f.Level, f.At, now, fatigueHalflife)
	if level < fatigueFloor {
		return 0
	}
	return level
}

// Spend adds the cost of taking the initiative of kind t at now.
func (f Fatigue) Spend(t Trigger, now time.Time) Fatigue {
	return Fatigue{Level: clamp01(f.Now(now) + initiativeCost[t]), At: now}
}

// Rested scales an opportunity's odds by how much she has put herself
// forward lately. Cubed, so a little fatigue barely matters and a lot all
// but stops her: half-tired she is at an eighth of her usual odds.
func Rested(fatigue float64) float64 {
	r := 1 - clamp01(fatigue)
	return r * r * r
}

// decayed halves a stored value every halflife since at.
func decayed(stored float64, at, now time.Time, halflife time.Duration) float64 {
	if stored <= 0 || at.IsZero() {
		return 0
	}
	elapsed := now.Sub(at)
	if elapsed <= 0 {
		return stored
	}
	return stored * math.Pow(0.5, float64(elapsed)/float64(halflife))
}
