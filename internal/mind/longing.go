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

// AttentionLevel is how much reaching out someone has agreed to.
type AttentionLevel string

const (
	AttentionOff       AttentionLevel = ""
	AttentionLight     AttentionLevel = "light"
	AttentionKeen      AttentionLevel = "keen"
	AttentionInsistent AttentionLevel = "insistent"
)

// ParseAttention reads a level, reporting whether it is one.
func ParseAttention(s string) (AttentionLevel, bool) {
	switch l := AttentionLevel(strings.ToLower(strings.TrimSpace(s))); l {
	case AttentionLight, AttentionKeen, AttentionInsistent:
		return l, true
	case "off", "":
		return AttentionOff, true
	default:
		return AttentionOff, false
	}
}

// ceiling is what a level tolerates: how many a day and how far apart.
type ceiling struct {
	perDay   int
	cooldown time.Duration
}

var ceilings = map[AttentionLevel]ceiling{
	AttentionLight:     {perDay: 1, cooldown: 12 * time.Hour},
	AttentionKeen:      {perDay: 3, cooldown: 4 * time.Hour},
	AttentionInsistent: {perDay: 6, cooldown: 90 * time.Minute},
}

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
	Now   time.Time
	Level AttentionLevel
	// Longing, Warmth and Irritation are how she feels about them.
	Longing    Longing
	Warmth     float64
	Irritation float64
	Drives     Drives
	// Hour is the hour in the community's timezone.
	Hour int
	// Today is how many times she has reached out to them today, Last when
	// she last did, and Unanswered how many in a row they have not answered.
	Today      int
	Last       time.Time
	Unanswered int
	// Engaged is whether they are already talking to her.
	Engaged bool
}

// Urge is how much she wants their attention right now, 0..1. Missing them is
// the heart of it, weighted by how much they matter to her; being ignored
// while they are plainly around sharpens it; being alone adds to it and being
// tired or annoyed with them takes it away.
func Urge(r Reach) float64 {
	u := r.Longing.Missing * (0.35 + 0.65*clamp01(r.Warmth))
	if r.Longing.Neglected {
		u += 0.15
	}
	u += 0.15 * r.Drives.Social
	u -= 0.25 * (1 - r.Drives.Energy)
	u -= r.Irritation
	return clamp01(u)
}

// MayReach decides whether she reaches out now. The ceiling the person set
// and the guards come first; then the urge, rolled against, so what is left is
// her choice.
func MayReach(r Reach, roll float64) (bool, float64) {
	c, ok := ceilings[r.Level]
	if !ok || r.Engaged {
		return false, 0
	}
	if r.Today >= c.perDay {
		return false, 0
	}
	// Doubling with every unanswered one, and stopping after a few: the
	// ceiling is what they allowed, silence is what they said.
	if r.Unanswered >= maxUnanswered {
		return false, 0
	}
	wait := c.cooldown * time.Duration(1<<r.Unanswered)
	if !r.Last.IsZero() && r.Now.Sub(r.Last) < wait {
		return false, 0
	}
	if r.Hour >= quietFrom || r.Hour < quietUntil {
		return false, 0
	}
	if r.Drives != (Drives{}) && r.Drives.Energy < tooTiredToVolunteer {
		return false, 0
	}
	urge := Urge(r)
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
	case r.Warmth > fondAbove:
		feel = "You miss them, not that you would put it that way."
	case r.Warmth > likesAbove:
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
