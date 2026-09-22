package body

import (
	"math/rand/v2"
	"testing"
	"time"
)

var loc = time.UTC

func seeded(seed uint64) func() float64 {
	r := rand.New(rand.NewPCG(seed, seed))
	return r.Float64
}

// The spec's targets for a quiet week, pinned: asleep around 00:30, awake
// around 09:30, three to six stretches online a day. The constants in body.go
// are fitted against this.
func TestAQuietWeek(t *testing.T) {
	start := time.Date(2026, 9, 21, 12, 0, 0, 0, loc)
	for seed := uint64(1); seed <= 5; seed++ {
		b := New(loc, seeded(seed), start)
		stretches := map[string]int{}
		var sleeps, wakes []time.Time
		for now := start; now.Before(start.Add(7 * 24 * time.Hour)); now = now.Add(5 * time.Minute) {
			for _, e := range b.Advance(now, false) {
				switch {
				case e.To == Asleep && e.From != Asleep:
					sleeps = append(sleeps, e.At)
				case e.Why == WhyWoke:
					wakes = append(wakes, e.At)
				}
				if e.To == Online {
					stretches[e.At.Format("2006-01-02")]++
				}
			}
		}
		for _, s := range sleeps {
			if off := clockOff(s, 0, 30); off > time.Hour {
				t.Errorf("seed %d: fell asleep at %s", seed, s.Format("15:04"))
			}
		}
		for _, w := range wakes {
			if off := clockOff(w, 9, 30); off > time.Hour {
				t.Errorf("seed %d: woke at %s", seed, w.Format("15:04"))
			}
		}
		if len(sleeps) < 6 || len(wakes) < 6 {
			t.Errorf("seed %d: %d nights, %d mornings in a week", seed, len(sleeps), len(wakes))
		}
		total := 0
		for _, n := range stretches {
			total += n
		}
		if avg := float64(total) / float64(len(stretches)); avg < 3 || avg > 6 {
			t.Errorf("seed %d: %.1f stretches online a day, want 3 to 6", seed, avg)
		}
	}
}

// clockOff is how far t's time of day is from h:m, around midnight.
func clockOff(t time.Time, h, m int) time.Duration {
	day := 24 * time.Hour
	got := time.Duration(t.Hour())*time.Hour + time.Duration(t.Minute())*time.Minute
	want := time.Duration(h)*time.Hour + time.Duration(m)*time.Minute
	d := (got - want + day) % day
	return min(d, day-d)
}

// A good evening keeps her up past her usual hour.
func TestAConversationKeepsHerUpLater(t *testing.T) {
	start := time.Date(2026, 9, 21, 20, 0, 0, 0, loc)
	quiet := New(loc, nil, start)
	busy := New(loc, nil, start)
	var quietAt, busyAt time.Time
	for now := start; now.Before(start.Add(8 * time.Hour)); now = now.Add(time.Minute) {
		for _, e := range quiet.Advance(now, false) {
			if e.To == Asleep && quietAt.IsZero() {
				quietAt = e.At
			}
		}
		for _, e := range busy.Advance(now, true) {
			if e.To == Asleep && busyAt.IsZero() {
				busyAt = e.At
			}
		}
	}
	if !busyAt.After(quietAt.Add(30 * time.Minute)) {
		t.Errorf("quiet slept at %s, engaged at %s", quietAt.Format("15:04"), busyAt.Format("15:04"))
	}
}

// Running out of energy sends her away until she has recovered.
func TestRunningOutSendsHerAway(t *testing.T) {
	now := time.Date(2026, 9, 21, 15, 0, 0, 0, loc)
	b := New(loc, nil, now)
	b.Drain(0.9)
	events := b.Advance(now.Add(time.Minute), false)
	if len(events) != 1 || events[0].Why != WhyTired {
		t.Fatalf("events %+v", events)
	}
	events = b.Advance(now.Add(3*time.Hour), false)
	if len(events) != 1 || events[0].Why != WhyBack {
		t.Errorf("events %+v", events)
	}
}

// An interruption during a conversation waits for a lull.
func TestAnInterruptionWaitsForALull(t *testing.T) {
	now := time.Date(2026, 9, 21, 15, 0, 0, 0, loc)
	b := New(loc, func() float64 { return 0 }, now)
	if events := b.Advance(now.Add(time.Minute), true); len(events) != 0 {
		t.Fatalf("left mid-conversation: %+v", events)
	}
	if !b.State().Pending {
		t.Fatal("no interruption pending")
	}
	events := b.Advance(now.Add(2*time.Minute), false)
	if len(events) != 1 || events[0].Why != WhyInterrupted {
		t.Errorf("events %+v", events)
	}
}

// Restored after six hours down, she has lived them.
func TestRestoreLivesTheGap(t *testing.T) {
	now := time.Date(2026, 9, 21, 22, 0, 0, 0, loc)
	b := New(loc, nil, now)
	st := b.State()
	later := now.Add(6 * time.Hour)
	r := Restore(loc, nil, st, later)
	if got := r.State(); got.Presence != Asleep || !got.At.Equal(later) {
		t.Errorf("restored at 04:00 as %+v", got)
	}
}

// Asleep, nothing gets through; away, a notice brings her back.
func TestNoticeOnlyReachesHerAway(t *testing.T) {
	night := time.Date(2026, 9, 22, 3, 0, 0, 0, loc)
	if events := New(loc, nil, night).Notice(night); len(events) != 0 {
		t.Errorf("woke her: %+v", events)
	}
	day := time.Date(2026, 9, 22, 15, 0, 0, 0, loc)
	b := New(loc, nil, day)
	b.Drain(0.9)
	b.Advance(day.Add(time.Minute), false)
	if events := b.Notice(day.Add(2 * time.Minute)); len(events) != 1 || events[0].To != Online {
		t.Errorf("events %+v", events)
	}
}

// Woken at three in the morning she is up, marked as woken, and kept up for
// an hour whatever her pressure says; her pressure is where the night left
// it, so she goes back down soon after. The next time she wakes on her own
// is ordinary again.
func TestWokenEarlyIsUpAWhileAndStillTired(t *testing.T) {
	night := time.Date(2026, 9, 23, 3, 0, 0, 0, loc)
	b := New(loc, nil, night)
	if b.State().Presence != Asleep {
		t.Fatalf("at three she is %s", b.State().Presence)
	}
	pressure := b.State().S
	events := b.Wake(night)
	st := b.State()
	if len(events) != 1 || events[0].Why != WhyWoken || st.Presence != Online || !st.Woken || !st.WokeAt.Equal(night) {
		t.Fatalf("woken: %+v, state %+v", events, st)
	}
	if st.S != pressure {
		t.Errorf("being woken reset her pressure: %.2f → %.2f", pressure, st.S)
	}
	b.Advance(night.Add(50*time.Minute), false)
	if b.State().Presence != Online {
		t.Errorf("back asleep within the hour: %s", b.State().Presence)
	}
	var slept bool
	for _, e := range b.Advance(night.Add(3*time.Hour), false) {
		slept = slept || e.To == Asleep
	}
	if !slept {
		t.Error("a woken body at four in the morning never went back to sleep")
	}
	var woke bool
	for _, e := range b.Advance(night.Add(14*time.Hour), false) {
		if e.Why == WhyWoke {
			woke = true
		}
	}
	if !woke || b.State().Woken {
		t.Errorf("waking on her own: woke %v, still marked woken %v", woke, b.State().Woken)
	}
	// Awake already, waking does nothing.
	if events := b.Wake(night.Add(14 * time.Hour)); len(events) != 0 {
		t.Errorf("woke someone awake: %+v", events)
	}
}
