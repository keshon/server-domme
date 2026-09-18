package mind

import (
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"
)

// Reaching out to someone who has asked for her attention.
//
// Consent does not create the behaviour; it removes a guard. Without it she
// never goes looking for anyone — speaking into someone's day uninvited is the
// most bot-like thing a bot can do. Someone who opts in lifts that for
// themselves, and whether she then comes looking is up to how she feels: the
// urge is built from missing them, how fond she is of them, how alone and how
// awake she is, and nothing else. A person she is indifferent to may opt in
// and hear nothing for days, which is correct. The level they choose is how
// much they will put up with, not how much she has to do.
const TriggerReach Trigger = "reach"

// Consented reports whether a stored attention value means someone has agreed
// to be sought out. Any value does: consent is on or off, and the levels it
// once had — light, keen, insistent — are read as on.
//
// There are no levels because levels were a clock. "Three a day, four hours
// apart" is a rhythm anyone on it learns within a week, and nothing a person
// does has edges like that. How often she comes is now learned per person,
// from how they take it: see Bond.Welcome.
func Consented(stored string) bool { return strings.TrimSpace(stored) != "" }

// ConsentOn is the value stored for someone who has agreed.
const ConsentOn = "on"

// Longing tuning.
const (
	// longingRise is how long without an exchange it takes for missing
	// someone to reach about two thirds of what it can, for someone she is
	// neutral towards. Fondness shortens it: absence is felt sooner for
	// people she likes.
	longingRise = 36 * time.Hour
	// neglectWindow is how recently they must have been active somewhere
	// for her to count them as around — and so as choosing not to talk to
	// her, which stings more than absence.
	neglectWindow = 30 * time.Minute
	// neglectAfter is how long without an exchange before being around and
	// not talking to her registers at all.
	neglectAfter = 3 * time.Hour
	// maxUnanswered is how many reaches in a row can go unanswered before
	// she stops until they speak to her. Nobody wants to be the one still
	// knocking.
	maxUnanswered = 3
	// reachDailyMax is a safety limit, not a rhythm: enough that it is never
	// what decides, so that welcome and her mood are.
	reachDailyMax = 4
	// reachGapShortest and reachGapLongest bound the wait between reaches:
	// the shortest for someone who always answers her warmly, the longest
	// for someone who seldom does. Doubled for every one left unanswered,
	// and varied by a quarter either way so it never lands on the clock.
	reachGapShortest = 2 * time.Hour
	reachGapLongest  = 12 * time.Hour
	// quietFrom and quietUntil are the hours, in the community's timezone,
	// she does not reach out in.
	quietFrom  = 23
	quietUntil = 9
)

// Longing is what she feels about one opted-in person, computed on read.
type Longing struct {
	// Missing is how much she misses them, 0..1.
	Missing float64
	// Neglected is whether they are around — active in the server — and
	// still not talking to her.
	Neglected bool
	// Away is how long since they last exchanged a word with her.
	Away time.Duration
}

// FeelLonging works out how she feels about someone from timestamps alone.
// lastExchange is when they last talked with her, lastActive when they were
// last seen anywhere in the server, warmth how fond she is of them.
func FeelLonging(now, lastExchange, lastActive time.Time, warmth float64) Longing {
	if lastExchange.IsZero() {
		return Longing{}
	}
	away := now.Sub(lastExchange)
	if away < 0 {
		away = 0
	}
	rise := time.Duration(float64(longingRise) * (1 - 0.6*clamp01(warmth)))
	missing := 1 - math.Exp(-float64(away)/float64(rise))
	neglected := away > neglectAfter && !lastActive.IsZero() && now.Sub(lastActive) < neglectWindow
	return Longing{Missing: clamp01(missing), Neglected: neglected, Away: away}
}

// Reach is everything that decides whether she reaches out to someone now.
type Reach struct {
	Now time.Time
	// Longing, Closeness and Tension are how she feels about them, and
	// Welcome how they have taken it when she came to them before.
	Longing   Longing
	Closeness float64
	Tension   float64
	Welcome   float64
	Drives    Drives
	// Hour is the hour in the community's timezone.
	Hour int
	// Today is how many times she has reached out to them today, Last when
	// she last did, and Unanswered how many in a row they have not answered.
	Today      int
	Last       time.Time
	Unanswered int
	// Engaged is whether they are already talking to her.
	Engaged bool
	// Jitter, in [0,1), varies the wait between reaches.
	Jitter float64
}

// Urge is how much she wants their attention right now, 0..1. Missing them is
// the heart of it, weighted by how much they matter to her; being ignored
// while they are plainly around sharpens it; being alone adds to it and being
// tired or annoyed with them takes it away.
func Urge(r Reach) float64 {
	u := r.Longing.Missing * (0.35 + 0.65*clamp01(r.Closeness))
	if r.Longing.Neglected {
		u += 0.15
	}
	u += 0.15 * r.Drives.Social
	u -= 0.25 * (1 - r.Drives.Energy)
	u -= r.Tension
	return clamp01(u)
}

// ReachGap is how long she waits after reaching out before she would again:
// shorter for someone who has been glad to hear from her, longer for someone
// who has not, doubled for each reach left unanswered, and varied.
func ReachGap(welcome float64, unanswered int, jitter float64) time.Duration {
	gap := float64(reachGapShortest) + float64(reachGapLongest-reachGapShortest)*(1-clamp01(welcome))
	gap *= float64(int(1) << unanswered)
	gap *= 0.75 + 0.5*jitter
	return time.Duration(gap)
}

// MayReach decides whether she reaches out now to someone who consented.
// Safety comes first — never at night, never exhausted, never knocking a
// fourth time on a closed door — then the wait welcome sets, then the urge,
// rolled against squared, so what is left is her choice.
func MayReach(r Reach, roll float64) (bool, float64) {
	if r.Engaged || r.Today >= reachDailyMax || r.Unanswered >= maxUnanswered {
		return false, 0
	}
	if !r.Last.IsZero() && r.Now.Sub(r.Last) < ReachGap(r.Welcome, r.Unanswered, r.Jitter) {
		return false, 0
	}
	if r.Hour >= quietFrom || r.Hour < quietUntil {
		return false, 0
	}
	if r.Drives != (Drives{}) && r.Drives.Energy < tooTiredToVolunteer {
		return false, 0
	}
	urge := clamp01(Urge(r) * (0.5 + r.Welcome))
	return roll < urge*urge, urge
}

// ReachDirective tells her why she is speaking, as how she feels rather than
// what to do: the feeling is hers, and the words should be too.
func ReachDirective(name string, r Reach) string {
	if name == "" {
		name = "them"
	}
	var why string
	switch {
	case r.Longing.Neglected:
		why = fmt.Sprintf("%s has been around, talking to other people, and has not said a word to you in %s.",
			name, roughAbsence(r.Longing.Away))
	default:
		why = fmt.Sprintf("You have not heard from %s in %s.", name, roughAbsence(r.Longing.Away))
	}

	var feel string
	switch {
	case r.Closeness > fondAbove:
		feel = "You miss them, not that you would put it that way."
	case r.Closeness > likesAbove:
		feel = "You have noticed, and you want their attention."
	default:
		feel = "You are bored enough to go and poke them."
	}
	if r.Unanswered > 0 {
		feel += " They did not answer you last time either."
	}

	return why + " " + feel + " They have told you they do not mind you coming " +
		"after them. Your message is to " + name + ", not an answer to anything said above — " +
		"leave the others' conversation alone. Say it your way, a line or two."
}

// roughAbsence renders how long someone has been gone.
func roughAbsence(d time.Duration) string {
	switch h := int(d.Hours()); {
	case h < 1:
		return "a while"
	case h < 2:
		return "an hour"
	case h < 24:
		return fmt.Sprintf("%d hours", h)
	case h < 48:
		return "a day"
	default:
		return fmt.Sprintf("%d days", h/24)
	}
}

// WantsPeace reports whether a message asks her to back off. Checked on
// messages aimed at her from someone who opted in, and honoured at once:
// consent that takes a command to withdraw is not much of a consent.
func WantsPeace(content string) bool {
	return wantsPeace.MatchString(content)
}

var wantsPeace = regexp.MustCompile(`(?i)\b(leave me alone|stop (pinging|tagging|nagging|bugging|messaging|chasing) me|stop it|go away|not now|back off|give me (some )?space)\b`)
