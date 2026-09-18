package mind

import (
	"hash/fnv"
	"math"
	"time"
)

// Her own state beyond the drives: a mood, the kind of day she is having, and
// how much the room keeps saying the same thing.
//
// Mood is how her day is going, -1 to +1, one per server. It is what lets one
// bad exchange colour the next conversation without becoming a grudge against
// someone who had nothing to do with it: every event that moves a bond also
// moves her mood a little (Shift.Mood), less than it moves the bond and for
// less long. Someone who gets on her nerves leaves her short with them for the
// afternoon and a bit short with everyone for an hour or two.
//
// On top of that each server gets a tone for the day — flat most days, and
// now and then clearly good or clearly bad — which lifts or lowers both her
// energy and the mood she drifts back to. Seeded from the date rather than
// rolled, so it holds all day and survives a restart.
const (
	// moodHalflife is how long a swing in mood takes to half fade back to
	// her baseline. Shorter than tension (90 minutes to half, but felt
	// towards one person for longer because it starts higher), longer than a
	// conversation: long enough to carry into the next one.
	moodHalflife = 3 * time.Hour
	// moodFloor is where a swing that small counts as none.
	moodFloor = 0.02

	// dayEnergy and dayMood are how far the best and worst days move energy
	// and her baseline mood.
	dayEnergy = 0.12
	dayMood   = 0.35

	// habituation is how much of her arousal a conversation that only
	// circles the same words takes away.
	habituation = 0.6
	// repetitionMinWords is how many keywords the room needs before its
	// repetition is judged at all: three short lines repeating "lol" are not
	// a subject gone stale.
	repetitionMinWords = 12

	// warmthMood is how far the character's warmth dial moves her baseline
	// mood from neutral, at its extremes.
	warmthMood = 0.4
)

// MoodSwing is how far events have moved her mood from its baseline, stored
// with when it was last moved and decayed on read.
type MoodSwing struct {
	Level float64
	At    time.Time
}

// Now is the swing left at now.
func (m MoodSwing) Now(now time.Time) float64 {
	if m.Level == 0 || m.At.IsZero() {
		return 0
	}
	level := m.Level
	if elapsed := now.Sub(m.At); elapsed > 0 {
		level *= math.Pow(0.5, float64(elapsed)/float64(moodHalflife))
	}
	if math.Abs(level) < moodFloor {
		return 0
	}
	return clampSigned(level)
}

// Add moves her mood by delta at now.
func (m MoodSwing) Add(delta float64, now time.Time) MoodSwing {
	return MoodSwing{Level: clampSigned(m.Now(now) + delta), At: now}
}

// DayTone is the kind of day she is having in a server, -1 to +1: the same all
// day, different tomorrow, and different between servers.
//
// Cubed, so most days sit near zero and a clearly good or bad one — beyond
// ±0.6 — comes about one day in six. Noticeable but rare: a flat day most of
// the time, and now and then one regulars would call her being in a mood.
func DayTone(seed string, now time.Time, loc *time.Location) float64 {
	if loc == nil {
		loc = time.UTC
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(seed + "|" + now.In(loc).Format(time.DateOnly)))
	x := h.Sum64()
	u := float64(x>>11) / (1 << 53)
	if x&1 == 1 {
		return -u * u * u
	}
	return u * u * u
}

// Repetition is how much the live conversation keeps coming back to words it
// has already used, 0..1: the share of each line's keywords that an earlier
// line had. Her own lines are left out; it is the room going stale, not her.
//
// This is what habituation is made of. The fifth round of the same subject
// holds her less than the first, which a count of messages cannot see.
func Repetition(turns []Turn) float64 {
	seen := make(map[string]bool)
	var repeated, total int
	for _, t := range turns {
		if t.FromBot {
			continue
		}
		words := Keywords(t.Content)
		for _, w := range words {
			if seen[w] {
				repeated++
			}
			total++
		}
		for _, w := range words {
			seen[w] = true
		}
	}
	if total < repetitionMinWords {
		return 0
	}
	return float64(repeated) / float64(total)
}

// MoodBaseline is the mood she drifts back to, from her temperament: a warm
// character settles a little above neutral, a reserved one a little below.
func (s SpeechStyle) MoodBaseline() float64 {
	if s == (SpeechStyle{}) {
		return 0
	}
	return warmthMood * (s.Warmth - 0.5) * 2
}
