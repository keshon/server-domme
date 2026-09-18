package mind

import (
	"math"
	"sort"
	"strings"
	"time"
)

// What is on her mind: the people and subjects that matter to her right now,
// ranked by salience.
//
// Everything she starts on her own is an event meeting something that
// matters to her — a regular walking back in, a subject coming round, someone
// she misses staying away. The events were already modelled; how much the
// thing they touch matters to her was a fixed number per kind of event, except
// for reaching out. Salience is that second half, computed once, from state
// she already has, so that later the events can be weighed by it rather than
// by constants.
//
// /chat state lists it, and the things she starts are weighed by it: see
// pull.go.

// OnMindKind is what sort of thing is on her mind.
type OnMindKind string

const (
	// OnMindPerson is someone she knows.
	OnMindPerson OnMindKind = "person"
	// OnMindSubject is something she remembers happening.
	OnMindSubject OnMindKind = "subject"
	// OnMindConcern is something someone told her they were about to do.
	OnMindConcern OnMindKind = "concern"
)

// OnMind is one thing on her mind, with how much and why.
type OnMind struct {
	Kind     OnMindKind
	Name     string
	Salience float64
	// Why are the reasons, strongest first, as short phrases for a panel.
	Why []string
}

// PersonMind is what salience reads about one person, already decayed.
type PersonMind struct {
	Name      string
	Closeness float64
	Tension   float64
	// LastExchange is when they last spoke to her, LastActive when they were
	// last seen anywhere she could see.
	LastExchange time.Time
	LastActive   time.Time
	// Concerns are what they told her they were about to do.
	Concerns []Concern
}

// Salience tuning.
const (
	// onMindFloor is the salience below which something is not on her mind
	// at all.
	onMindFloor = 0.15
	// onMindMax is how many things the list holds. A mind with twelve things
	// on it has none.
	onMindMax = 5
	// maxSubjects is how many remembered subjects compete with people.
	maxSubjects = 2
	// afterglow is how long a conversation stays with her afterwards, as the
	// time for it to fade to about a third.
	afterglow = 3 * time.Hour
	// minFeelingForMissing is how close she has to be to someone before
	// their absence registers at all. Nobody misses an acquaintance.
	minFeelingForMissing = 0.1
	// subjectCeiling is the most a remembered subject can be on her mind:
	// people matter to her more than topics do.
	subjectCeiling = 0.6
)

// PersonSalience is how much someone is on her mind, 0..1, and why.
//
// Each reason is a separate pull, and they combine as independent chances —
// one minus the product of what each leaves — so two reasons count for more
// than one, but nothing runs past 1 and no single reason dominates by being
// added twice.
func PersonSalience(p PersonMind, now time.Time) (float64, []string) {
	type reason struct {
		pull float64
		why  string
	}
	var reasons []reason
	add := func(pull float64, why string) {
		if pull > 0.01 {
			reasons = append(reasons, reason{clamp01(pull), why})
		}
	}

	switch {
	case p.Closeness > fondAbove:
		add(0.8*p.Closeness, "fond of them")
	case p.Closeness > likesAbove:
		add(0.8*p.Closeness, "likes them")
	case p.Closeness >= minFeelingForMissing:
		add(0.8*p.Closeness, "a little attached")
	}
	add(0.9*p.Tension, "annoyed with them")

	// A conversation lingers, more so with someone who stirs something in
	// her either way.
	if !p.LastExchange.IsZero() {
		since := now.Sub(p.LastExchange)
		if since < 0 {
			since = 0
		}
		stirred := 0.4 + 0.6*math.Max(p.Closeness, p.Tension)
		add(0.6*stirred*math.Exp(-float64(since)/float64(afterglow)), "just talked")
	}

	if p.Closeness >= minFeelingForMissing {
		longing := FeelLonging(now, p.LastExchange, p.LastActive, p.Closeness)
		add(0.9*longing.Missing*p.Closeness, "misses them")
		if longing.Neglected {
			add(0.3+0.4*p.Closeness, "around, not talking to her")
		}
	}

	sort.SliceStable(reasons, func(i, j int) bool { return reasons[i].pull > reasons[j].pull })
	left := 1.0
	why := make([]string, 0, len(reasons))
	for _, r := range reasons {
		left *= 1 - r.pull
		why = append(why, r.why)
	}
	return clamp01(1 - left), why
}

// SubjectSalience is how much a memory is on her mind: how bright it still
// is, raised by how much it mattered, and never as much as a person can be.
// Memories fade over days, so everything from today is near full brightness;
// uncapped, the last conversation's subject outranked everyone she knows.
func SubjectSalience(m Memory, now time.Time) float64 {
	return clamp01(subjectCeiling * m.Brightness(now, nil, nil) * (0.4 + 0.6*clamp01(m.Weight)))
}

// OnHerMind ranks what is on her mind: people and remembered subjects
// together, strongest first, above the floor and at most onMindMax of them.
func OnHerMind(people []PersonMind, memories []Memory, mood float64, now time.Time) []OnMind {
	var out []OnMind
	for _, p := range people {
		if strings.TrimSpace(p.Name) == "" {
			continue
		}
		if s, why := PersonSalience(p, now); s >= onMindFloor {
			out = append(out, OnMind{Kind: OnMindPerson, Name: p.Name, Salience: s, Why: why})
		}
		for _, c := range p.Concerns {
			if s := c.Salience(now, p.Closeness, mood); s >= onMindFloor {
				out = append(out, OnMind{
					Kind: OnMindConcern, Name: p.Name + ": " + c.What, Salience: s,
					Why: []string{concernTiming(c, now)},
				})
			}
		}
	}

	var subjects []OnMind
	for _, m := range memories {
		gist := strings.TrimSpace(m.Gist)
		if gist == "" {
			continue
		}
		if s := SubjectSalience(m, now); s >= onMindFloor {
			subjects = append(subjects, OnMind{
				Kind: OnMindSubject, Name: gist, Salience: s,
				Why: []string{roughAbsence(now.Sub(m.At)) + " ago"},
			})
		}
	}
	sort.SliceStable(subjects, func(i, j int) bool { return subjects[i].Salience > subjects[j].Salience })
	if len(subjects) > maxSubjects {
		subjects = subjects[:maxSubjects]
	}
	out = append(out, subjects...)

	sort.SliceStable(out, func(i, j int) bool { return out[i].Salience > out[j].Salience })
	if len(out) > onMindMax {
		out = out[:onMindMax]
	}
	return out
}

// concernTiming says roughly when a concern is, for a panel.
func concernTiming(c Concern, now time.Time) string {
	if d := c.Due.Sub(now); d > 0 {
		return "in " + roughAbsence(d)
	}
	return roughAbsence(now.Sub(c.Due)) + " ago"
}
