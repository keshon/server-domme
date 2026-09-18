package mind

import (
	"math"
	"testing"
	"time"
)

func salienceOf(p PersonMind, now time.Time) float64 {
	s, _ := PersonSalience(p, now)
	return s
}

// The ranking has to match intuition before anything acts on it: someone she
// is close to and has not heard from in a day is on her mind; someone she is
// indifferent to and last spoke to last week is not.
func TestSalienceRanksWhoMatters(t *testing.T) {
	now := time.Now()
	missed := PersonMind{Name: "Big M", Closeness: 0.8, LastExchange: now.Add(-30 * time.Hour)}
	stranger := PersonMind{Name: "newbie", LastExchange: now.Add(-7 * 24 * time.Hour)}
	annoying := PersonMind{Name: "troll", Tension: 0.7, LastExchange: now.Add(-20 * time.Minute)}

	if salienceOf(missed, now) < 0.6 {
		t.Errorf("close and missed: %.2f", salienceOf(missed, now))
	}
	if salienceOf(stranger, now) >= onMindFloor {
		t.Errorf("indifferent and gone a week: %.2f", salienceOf(stranger, now))
	}
	if salienceOf(annoying, now) < 0.6 {
		t.Errorf("annoyed with them just now: %.2f", salienceOf(annoying, now))
	}
}

// A conversation lingers and then fades, and lingers longer as a feeling
// than as a mere exchange.
func TestAConversationLingersThenFades(t *testing.T) {
	now := time.Now()
	just := PersonMind{Name: "cass", LastExchange: now.Add(-5 * time.Minute)}
	later := just
	later.LastExchange = now.Add(-10 * time.Hour)
	if s := salienceOf(just, now); s < onMindFloor {
		t.Errorf("someone she was just talking to is not on her mind: %.2f", s)
	}
	if s := salienceOf(later, now); s >= onMindFloor {
		t.Errorf("an indifferent exchange ten hours ago is still on her mind: %.2f", s)
	}
}

// Nobody misses an acquaintance.
func TestAbsenceOnlyRegistersForSomeoneSheIsCloseTo(t *testing.T) {
	now := time.Now()
	_, why := PersonSalience(PersonMind{Name: "x", Closeness: 0.05, LastExchange: now.Add(-72 * time.Hour)}, now)
	for _, w := range why {
		if w == "misses them" {
			t.Error("missed someone she barely knows")
		}
	}
}

// Two reasons count for more than one, and nothing runs past 1.
func TestReasonsStackButSaturate(t *testing.T) {
	now := time.Now()
	one := PersonMind{Name: "a", Closeness: 0.7}
	two := one
	two.Tension = 0.6
	if salienceOf(two, now) <= salienceOf(one, now) {
		t.Error("a second reason added nothing")
	}
	all := PersonMind{Name: "b", Closeness: 1, Tension: 1, LastExchange: now, LastActive: now}
	if s := salienceOf(all, now); s > 1 || math.IsNaN(s) {
		t.Errorf("salience ran to %.2f", s)
	}
}

func TestOnHerMindIsShortAndRanked(t *testing.T) {
	now := time.Now()
	var people []PersonMind
	for i := range 10 {
		people = append(people, PersonMind{Name: string(rune('a' + i)), Closeness: 0.3 + 0.07*float64(i)})
	}
	memories := []Memory{
		{At: now.Add(-2 * time.Hour), Gist: "the purge argument", Weight: 1},
		{At: now.Add(-3 * time.Hour), Gist: "cass's cat", Weight: 0.5},
		{At: now.Add(-4 * time.Hour), Gist: "the event schedule", Weight: 0.5},
	}
	got := OnHerMind(people, memories, now)
	if len(got) != onMindMax {
		t.Fatalf("%d things on her mind, want %d", len(got), onMindMax)
	}
	subjects := 0
	for i, m := range got {
		if i > 0 && m.Salience > got[i-1].Salience {
			t.Errorf("not ranked: %v", got)
		}
		if m.Kind == OnMindSubject {
			subjects++
		}
	}
	if subjects > maxSubjects {
		t.Errorf("%d subjects crowd out people", subjects)
	}
	if got[0].Name != "j" {
		t.Errorf("the one she is closest to is not first: %q", got[0].Name)
	}
}
