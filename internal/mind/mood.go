package mind

import (
	"strings"
	"time"
)

// Drives are the slow-moving needs behind how she is feeling, on 0..1.
//
// Borrowed from the cognitum experiment, with its central claim kept and its
// cost dropped: the symbolic core owns how the character feels, and the model
// is only told the result. Nothing here calls a backend, which is what makes it
// affordable — the version this is taken from spent around eighty model calls
// an hour maintaining state like this, and a third of the replies failed to
// parse.
//
// Three rather than cognitum's four. Social, Energy and Interest each have an
// input this bot can actually observe; Coherence had none, and a drive fed by
// nothing is a number that drifts convincingly and means nothing.
type Drives struct {
	// Social rises with time alone and falls on contact. High means she has
	// had nobody to talk to for a while.
	Social float64
	// Energy follows the clock, lowest in the small hours.
	Energy float64
	// Interest rises with how busy the channel is and how much of it is aimed
	// at her.
	Interest float64
}

// MoodInput is everything the drives are computed from. All of it is either a
// timestamp or a count the bot already has.
type MoodInput struct {
	Now time.Time
	// LastSpokeAt is when she last said something in this guild. Zero means
	// she never has, which reads as solitude rather than as a fresh start —
	// a bot that has never spoken here has, in fact, been quiet.
	LastSpokeAt time.Time
	// RecentTurns is how many messages are in the live conversation.
	RecentTurns int
	// AddressedTurns is how many of those were aimed at her.
	AddressedTurns int
	// Location is the timezone the community keeps, not the one the server is
	// racked in. Nil means UTC.
	Location *time.Location
}

// Solitude thresholds, in the sense of "how long is a while".
const (
	// solitudeFull is the gap at which the social drive is saturated. Hours
	// rather than minutes because this is meant to separate "quiet evening"
	// from "nobody has been here since yesterday", and the live conversation
	// buffer already covers everything shorter.
	solitudeFull = 6 * time.Hour
	// busyChannel is the number of live turns that counts as a busy room.
	busyChannel = 12
)

// DeriveDrives computes the drives from observable state.
//
// Pure, and deliberately not a tick. cognitum recomputed on a one-second timer,
// which needs a goroutine, loses everything on restart and burns cycles in an
// empty server. Computing from elapsed time on read gives the same curves, for
// nothing, and survives a redeploy because the timestamps do.
func DeriveDrives(in MoodInput) Drives {
	return Drives{
		Social:   solitude(in.Now, in.LastSpokeAt),
		Energy:   circadian(in.Now, in.Location),
		Interest: interest(in.RecentTurns, in.AddressedTurns),
	}
}

// solitude is how much of a need to talk has built up, saturating at
// solitudeFull.
func solitude(now, lastSpoke time.Time) float64 {
	if lastSpoke.IsZero() {
		return 1
	}
	gap := now.Sub(lastSpoke)
	if gap <= 0 {
		return 0
	}
	return clamp01(float64(gap) / float64(solitudeFull))
}

// dayCurve is energy at each anchor hour, interpolated between and wrapping at
// midnight.
//
// Anchors rather than a cosine, which was the first attempt and is wrong for a
// reason worth keeping: a 24-hour cosine is symmetric, so placing the trough at
// 04:00 necessarily makes 08:00 just as dark. People do not work that way —
// energy collapses slowly overnight and returns sharply after waking — and the
// test that caught it was checking 04:00 against 10:00.
var dayCurve = [...]float64{
	0.35, 0.25, 0.18, 0.15, 0.18, 0.25, // 00-05
	0.35, 0.50, 0.65, 0.75, 0.80, 0.82, // 06-11
	0.84, 0.86, 0.86, 0.87, 0.88, 0.90, // 12-17
	0.90, 0.89, 0.87, 0.80, 0.65, 0.48, // 18-23
}

// circadian is energy across the day in the community's timezone: lowest
// around 03:00, highest through the afternoon and evening.
func circadian(now time.Time, loc *time.Location) float64 {
	if loc == nil {
		loc = time.UTC
	}
	local := now.In(loc)

	hour := local.Hour()
	frac := float64(local.Minute()) / 60
	next := (hour + 1) % len(dayCurve)

	// Interpolated between anchors so the value moves continuously. Energy
	// that steps at the top of the hour is no more convincing than none.
	return clamp01(dayCurve[hour]*(1-frac) + dayCurve[next]*frac)
}

// interest is how much is going on and how much of it involves her.
func interest(recent, addressed int) float64 {
	if recent <= 0 {
		return 0
	}
	busy := clamp01(float64(recent) / busyChannel)
	aimed := clamp01(float64(addressed) / float64(recent))
	return clamp01(0.6*busy + 0.4*aimed)
}

// Nudge is how much the drives move the odds of answering, positive or
// negative, roughly within a fifth either way.
//
// Applied only to the indirect approaches — being named in passing, or
// continuing an exchange. A direct mention or a reply is answered on its own
// terms regardless of mood: someone tired still answers when spoken to, and
// making that conditional is how the earlier "she ignored my direct question"
// bug gets reintroduced with a better excuse.
func (d Drives) Nudge() float64 {
	// The zero value means "not computed", not "exhausted and friendless".
	// Without this, every caller that leaves Drives unset is silently taxed
	// a fifth of its odds by an Energy of nought — which is exactly what an
	// existing test caught when this was first wired up.
	if d == (Drives{}) {
		return 0
	}

	lonely := 0.15 * d.Social
	tired := 0.20 * (1 - d.Energy)
	engaged := 0.10 * d.Interest

	nudge := lonely + engaged - tired
	if nudge < -0.2 {
		return -0.2
	}
	if nudge > 0.25 {
		return 0.25
	}
	return nudge
}

// Phrase renders the drives as something a person would say about themselves,
// or the empty string when there is nothing worth remarking on.
//
// No numbers, and never more than two clauses. The character file's own rule is
// that she does not describe her internal state; the point of this line is to
// colour how she answers, not to give her something to announce. Anything
// longer starts being reported rather than felt — the cognitum identity file
// hit the same wall and answered it the same way, with "never expose internal
// metrics".
func (d Drives) Phrase() string {
	var parts []string

	switch {
	case d.Energy < 0.25:
		parts = append(parts, "it is the dead of night and you are running on fumes")
	case d.Energy < 0.45:
		parts = append(parts, "you are low on energy")
	case d.Energy > 0.8:
		parts = append(parts, "you are wide awake")
	}

	switch {
	case d.Social > 0.8:
		parts = append(parts, "nobody has spoken to you in a long time")
	case d.Social > 0.5:
		parts = append(parts, "it has been quiet for a while")
	}

	if len(parts) < 2 && d.Interest > 0.7 {
		parts = append(parts, "the room is busy and a fair bit of it is aimed at you")
	}

	if len(parts) == 0 {
		return ""
	}
	if len(parts) > 2 {
		parts = parts[:2]
	}
	return "Right now: " + strings.Join(parts, ", ") + "."
}
