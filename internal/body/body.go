// Package body is the persona's body: how long she has been awake, the time
// of day her body is set to, her energy for people, and whether she is
// online, away or asleep.
//
// It is numbers, and the numbers never reach a model. They decide when she
// sees anything and how much of it, never what she is told about herself.
// Sleep follows from state rather than from a schedule: a late night shows
// the next morning. See docs/persona-v3.md, workstream B.
//
// Nothing here knows about Discord or about models; the chat service drives
// it, and cmd/bodysim runs it alone to tune it.
package body

import (
	"math"
	"sync"
	"time"
)

// Presence is where she is.
type Presence string

// Presences. Stored, so frozen once shipped.
const (
	// Online behaves as she always has.
	Online Presence = "online"
	// Away is about today, not looking. Messages are recorded; nothing is
	// handled.
	Away Presence = "away"
	// Asleep is the night.
	Asleep Presence = "asleep"
)

// Constants of the model. Starting values, fitted in bodysim: a quiet week
// has to put her to sleep around 00:30 and wake her around 09:30, with three
// to six online stretches a day. See TestAQuietWeek.
const (
	// sRise is how long awake takes sleep pressure from empty to full.
	sRise = 16 * time.Hour
	// sDecay is the time constant sleep pressure drains with, asleep.
	sDecay = 3 * time.Hour
	// circadianPeak is the hour her body is most awake.
	circadianPeak = 16.0
	// circadianWeight is how much the time of day counts against pressure.
	circadianWeight = 0.5
	// thetaSleep and thetaWake are the sleepiness she falls asleep above and
	// wakes below. engagedMargin raises the first while she is in a
	// conversation: a good evening keeps her up.
	thetaSleep    = 0.89
	thetaWake     = -0.17
	engagedMargin = 0.1

	// Battery recovery half-lives, by presence.
	recoverOnline = 40 * time.Minute
	recoverAway   = 15 * time.Minute
	recoverAsleep = 90 * time.Minute
	// tiredAt is the battery she steps away below, and backAt what she has
	// to recover to before coming back on her own.
	tiredAt = 0.15
	backAt  = 0.5

	// interruptEvery is the mean time online between life getting in the
	// way, and interruptMedian how long it usually takes. Tuned from the
	// spec's 90 minutes to 180 against TestAQuietWeek, to land three to six
	// stretches online on a quiet day.
	interruptEvery  = 180 * time.Minute
	interruptMedian = 25 * time.Minute
	interruptSpread = 0.5

	// step is the resolution the model advances in.
	step = time.Minute

	// wokenHold is how long someone woken early stays up before sleep can
	// take her again. Pressure is not reset by being woken: if her body was
	// not done, she goes back to bed once nobody is keeping her, and wakes
	// a little later than she would have.
	wokenHold = time.Hour
)

// State is the body at a moment. Everything the service needs to persist.
type State struct {
	// S is sleep pressure, B the social battery, 0 to 1.
	S, B     float64
	Presence Presence
	// Since is when the presence last changed; WokeAt when she last woke.
	Since  time.Time
	WokeAt time.Time
	// AwayUntil is when an interruption ends. Zero while away because she
	// ran out of energy: she comes back when she has recovered.
	AwayUntil time.Time
	// Session counts the times she has come online, so a caller can tell
	// one stretch online from the next.
	Session int
	// Pending is an interruption that came while she was in a conversation,
	// waiting for a lull.
	Pending bool
	// Woken is that she was woken early rather than waking on her own, and
	// HeldUntil how long being woken keeps her up. See Wake.
	Woken     bool
	HeldUntil time.Time
	// At is when the state was last advanced to.
	At time.Time
}

// Event is a change of presence, and why.
type Event struct {
	From, To Presence
	Why      string
	At       time.Time
}

// Reasons for a change of presence.
const (
	WhyWoke        = "woke"
	WhySlept       = "slept"
	WhyTired       = "tired"
	WhyInterrupted = "interrupted"
	WhyBack        = "back"
	WhyNoticed     = "noticed"
	WhyWoken       = "woken"
)

// Body is one body, safe for concurrent use.
type Body struct {
	mu   sync.Mutex
	st   State
	loc  *time.Location
	roll func() float64
	// quiet turns off life's interruptions: used to bring a new body to the
	// present without inventing a history of them.
	quiet bool
}

// New returns a body living in loc, starting as it would be at now on a
// quiet day. roll supplies randomness; the body's only randomness is when
// life interrupts and for how long.
func New(loc *time.Location, roll func() float64, now time.Time) *Body {
	if loc == nil {
		loc = time.UTC
	}
	b := &Body{loc: loc, roll: roll}
	b.st = b.settled(now)
	return b
}

// Restore returns a body resumed from a stored state and brought forward to
// now, as if she had lived the time in between.
func Restore(loc *time.Location, roll func() float64, st State, now time.Time) *Body {
	if loc == nil {
		loc = time.UTC
	}
	b := &Body{loc: loc, roll: roll, st: st}
	if st.At.IsZero() || st.Presence == "" {
		b.st = b.settled(now)
		return b
	}
	b.Advance(now, false)
	return b
}

// settled is the state a quiet day puts her in at now: woken at the fitted
// hour and lived forward without interruptions.
func (b *Body) settled(now time.Time) State {
	local := now.In(b.loc)
	wake := time.Date(local.Year(), local.Month(), local.Day(), 9, 30, 0, 0, b.loc)
	if wake.After(local) {
		wake = wake.AddDate(0, 0, -1)
	}
	sim := &Body{loc: b.loc, roll: b.roll, quiet: true, st: State{
		S: 0.05, B: 1, Presence: Online, Since: wake, WokeAt: wake, Session: 1, At: wake,
	}}
	sim.Advance(now, false)
	return sim.st
}

// State reports the body now.
func (b *Body) State() State {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.st
}

// Sleepiness is how sleepy she is: pressure against the time of day.
func (b *Body) Sleepiness(t time.Time) float64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.sleepiness(b.st.S, t)
}

func (b *Body) sleepiness(s float64, t time.Time) float64 {
	return s - circadianWeight*circadian(t.In(b.loc))
}

// circadian is how awake the time of day makes her, 0 to 1: highest at
// circadianPeak, lowest twelve hours from it.
func circadian(t time.Time) float64 {
	h := float64(t.Hour()) + float64(t.Minute())/60
	return 0.5 + 0.5*math.Cos(2*math.Pi*(h-circadianPeak)/24)
}

// Drain takes energy for people: talking costs, more people cost more.
func (b *Body) Drain(amount float64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.st.B = clamp(b.st.B - amount)
}

// Notice brings her back from away because something got through — a
// mention, like a phone notification. Asleep, nothing gets through.
func (b *Body) Notice(now time.Time) []Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.st.Presence != Away {
		return nil
	}
	return []Event{b.move(Online, WhyNoticed, now)}
}

// Wake brings her online because someone woke her: from sleep, woken early
// and kept up for wokenHold whatever her pressure says; from away, back as
// if she had noticed. Online, nothing happens.
func (b *Body) Wake(now time.Time) []Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch b.st.Presence {
	case Asleep:
		e := b.move(Online, WhyWoken, now)
		b.st.WokeAt, b.st.Woken, b.st.HeldUntil = now, true, now.Add(wokenHold)
		return []Event{e}
	case Away:
		return []Event{b.move(Online, WhyWoken, now)}
	}
	return nil
}

// Advance lives the body forward to now. engaged is whether she is in a
// conversation: it holds sleep off a little, and makes an interruption wait
// for a lull. It returns every change of presence on the way.
func (b *Body) Advance(now time.Time, engaged bool) []Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	var events []Event
	for b.st.At.Before(now) {
		dt := min(step, now.Sub(b.st.At))
		t := b.st.At.Add(dt)
		b.live(dt)
		if e, ok := b.decide(t, dt, engaged && now.Sub(t) < step); ok {
			events = append(events, e)
		}
		b.st.At = t
	}
	return events
}

// live moves pressure and battery on by dt.
func (b *Body) live(dt time.Duration) {
	if b.st.Presence == Asleep {
		b.st.S *= math.Exp(-float64(dt) / float64(sDecay))
	} else {
		b.st.S += float64(dt) / float64(sRise)
	}
	half := recoverOnline
	switch b.st.Presence {
	case Away:
		half = recoverAway
	case Asleep:
		half = recoverAsleep
	}
	b.st.B = 1 - (1-b.st.B)*math.Exp(-math.Ln2*float64(dt)/float64(half))
}

// decide is what the body does at t, having lived dt to get there.
func (b *Body) decide(t time.Time, dt time.Duration, engaged bool) (Event, bool) {
	sleepy := b.sleepiness(b.st.S, t)
	switch b.st.Presence {
	case Asleep:
		if sleepy < thetaWake {
			e := b.move(Online, WhyWoke, t)
			b.st.WokeAt, b.st.Woken = t, false
			return e, true
		}
	case Online:
		limit := thetaSleep
		if engaged {
			limit += engagedMargin
		}
		held := t.Before(b.st.HeldUntil)
		if sleepy > limit && !held {
			return b.move(Asleep, WhySlept, t), true
		}
		// Woken before her body was done, she goes back to bed once the
		// hour is up and nobody is keeping her: between the two
		// thresholds a body stays as it is, and without this someone
		// woken at three would stay up until evening.
		if b.st.Woken && !held && !engaged && sleepy > thetaWake {
			return b.move(Asleep, WhySlept, t), true
		}
		if b.st.B < tiredAt {
			return b.move(Away, WhyTired, t), true
		}
		if !b.quiet && !b.st.Pending && b.roll != nil && b.roll() < 1-math.Exp(-float64(dt)/float64(interruptEvery)) {
			b.st.Pending = true
		}
		if b.st.Pending && !engaged {
			e := b.move(Away, WhyInterrupted, t)
			b.st.AwayUntil = t.Add(b.interruption())
			return e, true
		}
	case Away:
		if sleepy > thetaSleep {
			return b.move(Asleep, WhySlept, t), true
		}
		if !t.Before(b.st.AwayUntil) && b.st.B > backAt {
			return b.move(Online, WhyBack, t), true
		}
	}
	return Event{}, false
}

// move changes presence.
func (b *Body) move(to Presence, why string, t time.Time) Event {
	e := Event{From: b.st.Presence, To: to, Why: why, At: t}
	b.st.Presence, b.st.Since, b.st.Pending = to, t, false
	if to == Online {
		b.st.Session++
		b.st.AwayUntil = time.Time{}
	}
	return e
}

// interruption is how long life gets in the way this time: lognormal, so
// usually around the median and now and then much longer.
func (b *Body) interruption() time.Duration {
	u1, u2 := b.roll(), b.roll()
	if u1 <= 0 {
		u1 = 1e-9
	}
	z := math.Sqrt(-2*math.Log(u1)) * math.Cos(2*math.Pi*u2)
	return time.Duration(float64(interruptMedian) * math.Exp(interruptSpread*z))
}

func clamp(v float64) float64 { return max(0, min(1, v)) }
