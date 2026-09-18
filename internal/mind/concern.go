package mind

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// Concerns: something on her mind about someone, because of something they
// said they were going to do.
//
// Not a behaviour. Nothing here tells her to ask how the interview went or to
// wish anyone luck. A concern has a salience — how much it is on her mind now
// — computed from when the thing happens, how much she cares about the
// person, her mood, and how often she has already let it pass. When she is
// answering that person it is on her mind with that probability, and then the
// prompt carries it as a fact with its time: "Big M's job interview was
// yesterday." Whether she mentions it, and how — luck beforehand, a question
// after, nothing — is left to her and the moment.
//
// The plan comes from the notes call, which already reads each settled
// conversation, as one more line shape with a "when" from a fixed list. Go
// turns the word into a date; the model never does date arithmetic. And Go
// checks the plan is in that person's own words before keeping it, because a
// stored concern is repeated back to them, and an invented one is a false
// belief she would act on for days.

// Concern is one thing on her mind about someone.
type Concern struct {
	// What is the plan, in a few words: "job interview".
	What string
	// Due is when it happens; Expires when asking about it would be strange.
	Due, Expires time.Time
	// Noted is when she learned of it.
	Noted time.Time
	// Passed is how many times it was on her mind while she answered them
	// and she let it go. Each one weakens it: she thought of it, and did not.
	Passed int
}

// Plan is a plan as the notes call reported it, before Go has dated it.
type Plan struct {
	What string
	When string
}

// Concern tuning.
const (
	// MaxConcerns is how many she keeps per person. Oldest go first.
	MaxConcerns = 4
	// concernLead is how far ahead a plan starts to be on her mind at all.
	concernLead = 48 * time.Hour
	// concernFade is how long after the event it takes to half fade.
	concernFade = 48 * time.Hour
	// concernShelf is how long after the event it stays at all.
	concernShelf = 5 * 24 * time.Hour
	// anticipation is how much a plan weighs just before it happens, against
	// 1 just after: people ask how something went far more than they wish
	// luck beforehand.
	anticipation = 0.3
	// letPass is what each time she let it pass leaves of it.
	letPass = 0.6
	// maxConcernWords keeps "what" to the few words a plan is.
	maxConcernWords = 6
)

// The words the notes call may use for when. Anything else is "later".
const (
	whenTonight  = "tonight"
	whenTomorrow = "tomorrow"
	whenWeekend  = "this weekend"
	whenNextWeek = "next week"
	whenLater    = "later"
)

// DueFrom dates a plan said at said, in the community's timezone. Evenings
// for the day-sized words, because that is when a plan made in chat has
// usually happened by; a weekend is Saturday afternoon.
func DueFrom(when string, said time.Time, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.UTC
	}
	local := said.In(loc)
	day := func(offset, hour int) time.Time {
		d := local.AddDate(0, 0, offset)
		return time.Date(d.Year(), d.Month(), d.Day(), hour, 0, 0, 0, loc)
	}
	switch strings.ToLower(strings.TrimSpace(when)) {
	case whenTonight:
		if due := day(0, 21); due.After(said) {
			return due
		}
		return said.Add(2 * time.Hour)
	case whenTomorrow:
		return day(1, 19)
	case whenWeekend:
		ahead := (int(time.Saturday) - int(local.Weekday()) + 7) % 7
		if local.Weekday() == time.Sunday {
			ahead = 0
		}
		if due := day(ahead, 15); due.After(said) {
			return due
		}
		return said.Add(4 * time.Hour)
	case whenNextWeek:
		return day(7, 12)
	default:
		return day(3, 12)
	}
}

// NewConcern dates a reported plan and keeps it only if it is in the person's
// own words: at least half of what the plan says, and at least one word, has
// to appear in something they said. A plan the model invented fails that.
// One they mentioned about somebody else passes it — "my brother is flying
// to Porto" is cass's own line — so a plan that names a relation, or comes
// from a line about one, is refused as well; see aboutSomeoneElse.
func NewConcern(p Plan, said time.Time, loc *time.Location, theirLines []string) (Concern, bool) {
	what := strings.Join(strings.Fields(p.What), " ")
	if what == "" || len(strings.Fields(what)) > maxConcernWords {
		return Concern{}, false
	}
	words := Keywords(what)
	if len(words) == 0 {
		return Concern{}, false
	}
	theirs := Keywords(strings.Join(theirLines, " "))
	if shared := sharedWords(words, theirs); shared == 0 || 2*shared < len(words) {
		return Concern{}, false
	}
	if aboutSomeoneElse(what, words, theirLines) {
		return Concern{}, false
	}
	due := DueFrom(p.When, said, loc)
	return Concern{What: what, Due: due, Expires: due.Add(concernShelf), Noted: said}, true
}

// Salience is how much this is on her mind at now, 0..1: when it happens,
// how much she cares about them, and her mood, weakened by each time she let
// it pass.
func (c Concern) Salience(now time.Time, closeness, mood float64) float64 {
	if !c.Expires.IsZero() && now.After(c.Expires) {
		return 0
	}
	var timing float64
	if until := c.Due.Sub(now); until > 0 {
		if until > concernLead {
			return 0
		}
		timing = anticipation * (1 - float64(until)/float64(concernLead))
	} else {
		timing = math.Pow(0.5, float64(-until)/float64(concernFade))
	}
	care := 0.25 + 0.75*clamp01(closeness)
	feeling := 1 + 0.3*clampSigned(mood)
	return clamp01(timing * care * feeling * math.Pow(letPass, float64(c.Passed)))
}

// Phrase is the concern as a fact with its time, for the prompt: "Big M's job
// interview was yesterday." Stated, not asked for — what she does with it is
// hers.
func (c Concern) Phrase(name string, now time.Time, loc *time.Location) string {
	if name == "" {
		name = "They"
	}
	return fmt.Sprintf("On your mind: %s's %s %s.", name, c.What, whenPhrase(c.Due, now, loc))
}

// whenPhrase says when something is or was, the way a person would.
func whenPhrase(due, now time.Time, loc *time.Location) string {
	if loc == nil {
		loc = time.UTC
	}
	d := calendarDays(now.In(loc), due.In(loc))
	switch {
	case d == 0 && due.After(now):
		return "is later today"
	case d == 0:
		return "was earlier today"
	case d == 1:
		return "is tomorrow"
	case d == -1:
		return "was yesterday"
	case d > 1:
		return fmt.Sprintf("is in %d days", d)
	default:
		return fmt.Sprintf("was %d days ago", -d)
	}
}

// calendarDays is how many calendar days b is after a.
func calendarDays(a, b time.Time) int {
	da := time.Date(a.Year(), a.Month(), a.Day(), 0, 0, 0, 0, time.UTC)
	db := time.Date(b.Year(), b.Month(), b.Day(), 0, 0, 0, 0, time.UTC)
	return int(math.Round(db.Sub(da).Hours() / 24))
}

// Mentions reports whether a message is about the concern: most of its words
// are there. Used both ways — their message closing it, and her reply having
// raised it.
func (c Concern) Mentions(text string) bool {
	words := Keywords(c.What)
	if len(words) == 0 {
		return false
	}
	shared := sharedWords(words, Keywords(text))
	return shared > 0 && 2*shared >= len(words)
}

// MergeConcerns folds newly noted concerns into the ones she has: the expired
// go, one already held is not held twice, and past MaxConcerns the oldest
// go.
func MergeConcerns(held, noted []Concern, now time.Time) []Concern {
	var out []Concern
	for _, c := range held {
		if c.Expires.IsZero() || now.Before(c.Expires) {
			out = append(out, c)
		}
	}
	for _, n := range noted {
		dup := false
		for _, c := range out {
			if c.Mentions(n.What) || n.Mentions(c.What) {
				dup = true
				break
			}
		}
		if !dup {
			out = append(out, n)
		}
	}
	if len(out) > MaxConcerns {
		out = out[len(out)-MaxConcerns:]
	}
	return out
}

// MostPressing is the concern most on her mind, and its salience.
func MostPressing(cs []Concern, now time.Time, closeness, mood float64) (int, float64) {
	best, top := -1, 0.0
	for i, c := range cs {
		if s := c.Salience(now, closeness, mood); s > top {
			best, top = i, s
		}
	}
	return best, top
}

func sharedWords(a, b []string) int {
	in := make(map[string]bool, len(b))
	for _, w := range b {
		in[w] = true
	}
	n := 0
	for _, w := range a {
		if in[w] {
			n++
		}
	}
	return n
}

// relations are the words that make a plan someone else's. Measured: asked
// for plans only of the speaker's own, a model still wrote "PLAN cass:
// brother flying to Porto" from "my brother is flying to Porto", and the
// own-words check passed it, because those were cass's words.
var relations = map[string]bool{
	"brother": true, "sister": true, "mom": true, "mum": true, "mother": true,
	"dad": true, "father": true, "parents": true, "wife": true, "husband": true,
	"girlfriend": true, "boyfriend": true, "partner": true, "son": true,
	"daughter": true, "kid": true, "kids": true, "friend": true, "friends": true,
	"cousin": true, "uncle": true, "aunt": true, "grandma": true, "grandpa": true,
	"boss": true, "colleague": true, "roommate": true, "neighbour": true,
	"neighbor": true, "family": true,
}

// aboutSomeoneElse reports whether a plan belongs to somebody the speaker
// mentioned rather than to the speaker: it names a relation, or the line it
// came from is about "my/his/her/their <relation>".
func aboutSomeoneElse(what string, words []string, lines []string) bool {
	for _, w := range strings.Fields(strings.ToLower(what)) {
		if relations[strings.Trim(w, ".,!?'\"")] {
			return true
		}
	}
	for _, line := range lines {
		lw := Keywords(line)
		if sharedWords(words, lw) == 0 {
			continue
		}
		fields := strings.Fields(strings.ToLower(line))
		for i := 0; i+1 < len(fields); i++ {
			switch fields[i] {
			case "my", "his", "her", "their", "our":
				if relations[strings.Trim(fields[i+1], ".,!?'\"")] {
					return true
				}
			}
		}
	}
	return false
}
