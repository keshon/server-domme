package mind

import (
	"math"
	"math/rand/v2"
	"testing"
	"time"
)

func TestFatigueWearsOff(t *testing.T) {
	now := time.Now()
	f := Fatigue{Level: 0.8, At: now.Add(-fatigueHalflife)}
	if got := f.Now(now); math.Abs(got-0.4) > 0.01 {
		t.Errorf("one half-life on: %.2f, want 0.40", got)
	}
	if got := f.Now(now.Add(24 * time.Hour)); got != 0 {
		t.Errorf("a day on: %.2f, want nothing left", got)
	}
}

func TestSpendingAddsUpAndSaturates(t *testing.T) {
	now := time.Now()
	f := Fatigue{}.Spend(TriggerAfterthought, now)
	once := f.Now(now)
	f = f.Spend(TriggerAfterthought, now)
	if f.Now(now) <= once {
		t.Errorf("twice (%.2f) not more tiring than once (%.2f)", f.Now(now), once)
	}
	for range 10 {
		f = f.Spend(TriggerReturn, now)
	}
	if f.Now(now) > 1 {
		t.Errorf("fatigue ran past 1: %.2f", f.Now(now))
	}
}

func TestEveryInitiativeCostsSomething(t *testing.T) {
	for _, tr := range []Trigger{TriggerReturn, TriggerRecall, TriggerReach, TriggerAfterthought} {
		if initiativeCost[tr] <= 0 {
			t.Errorf("%q costs nothing, so fatigue cannot space it out", tr)
		}
	}
}

func TestRestedIsGentleThenSteep(t *testing.T) {
	if Rested(0) != 1 || Rested(1) != 0 {
		t.Errorf("rested at the ends: %.2f, %.2f", Rested(0), Rested(1))
	}
	if Rested(0.1) < 0.7 || Rested(0.5) > 0.15 {
		t.Errorf("a little fatigue %.2f, half %.2f", Rested(0.1), Rested(0.5))
	}
}

func TestFatigueLowersEveryKindOfInitiative(t *testing.T) {
	v := willing(TriggerRecall)
	_, fresh := MayVolunteer(v, 1)
	v.Fatigue = 0.6
	if _, tired := MayVolunteer(v, 1); tired >= fresh {
		t.Errorf("speaking up: fresh %.2f, tired %.2f", fresh, tired)
	}

	a := clipped()
	_, fresh = MayAddAfterthought(a, 1)
	a.Fatigue = 0.6
	if _, tired := MayAddAfterthought(a, 1); tired >= fresh {
		t.Errorf("second thought: fresh %.2f, tired %.2f", fresh, tired)
	}

	r := eager()
	_, fresh = MayReach(r, 1)
	r.Fatigue = 0.6
	if _, tired := MayReach(r, 1); tired >= fresh {
		t.Errorf("reaching out: fresh %.2f, tired %.2f", fresh, tired)
	}
}

// A room that hands her a reason to speak every ten minutes all day. The
// caps this replaced allowed three a day at least forty-five minutes apart;
// fatigue should land near that on its own, and unlike the caps, the gaps
// between should vary rather than stack up at the cooldown.
func TestFatigueSpacesInitiativeUnevenly(t *testing.T) {
	const (
		days = 200
		step = 10 * time.Minute
		span = 14 * time.Hour
	)
	rng := rand.New(rand.NewPCG(1, 2))
	start := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)

	var total int
	var gaps []float64
	for d := range days {
		var f Fatigue
		var last time.Time
		day := start.Add(time.Duration(d) * 24 * time.Hour)
		for at := day; at.Before(day.Add(span)); at = at.Add(step) {
			v := willing(TriggerRecall)
			v.Now, v.Fatigue = at, f.Now(at)
			if ok, _ := MayVolunteer(v, rng.Float64()); !ok {
				continue
			}
			total++
			if !last.IsZero() {
				gaps = append(gaps, at.Sub(last).Minutes())
			}
			f, last = f.Spend(TriggerRecall, at), at
		}
	}

	perDay := float64(total) / days
	if perDay < 2 || perDay > 5 {
		t.Errorf("%.1f a day under constant opportunity, want about the old three", perDay)
	}
	mean, sd := meanSD(gaps)
	t.Logf("%.1f a day; gaps %.0f ± %.0f min, shortest %.0f", perDay, mean, sd, minOf(gaps))
	if sd/mean < 0.5 {
		t.Errorf("gaps average %.0f min, spread only %.0f: that is a clock", mean, sd)
	}
}

func meanSD(xs []float64) (float64, float64) {
	var sum float64
	for _, x := range xs {
		sum += x
	}
	mean := sum / float64(len(xs))
	var sq float64
	for _, x := range xs {
		sq += (x - mean) * (x - mean)
	}
	return mean, math.Sqrt(sq / float64(len(xs)))
}

func minOf(xs []float64) float64 {
	m := math.Inf(1)
	for _, x := range xs {
		m = math.Min(m, x)
	}
	return m
}
