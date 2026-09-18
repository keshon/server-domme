package mind

import (
	"math"
	"strings"
	"testing"
	"time"
)

func TestDueFromTurnsWordsIntoDates(t *testing.T) {
	wed := time.Date(2026, 9, 16, 14, 0, 0, 0, time.UTC) // a Wednesday afternoon
	cases := map[string]time.Time{
		"tomorrow":     time.Date(2026, 9, 17, 19, 0, 0, 0, time.UTC),
		"tonight":      time.Date(2026, 9, 16, 21, 0, 0, 0, time.UTC),
		"this weekend": time.Date(2026, 9, 19, 15, 0, 0, 0, time.UTC),
		"next week":    time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC),
		"whenever":     time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC),
	}
	for when, want := range cases {
		if got := DueFrom(when, wed, nil); !got.Equal(want) {
			t.Errorf("%q said on Wednesday 14:00: %s, want %s", when, got.Format("Mon 15:04"), want.Format("Mon 15:04"))
		}
	}
	late := time.Date(2026, 9, 16, 23, 30, 0, 0, time.UTC)
	if got := DueFrom("tonight", late, nil); !got.After(late) {
		t.Errorf("tonight, said at 23:30, is due %s — in the past", got)
	}
}

// A stored concern is repeated back to them, so an invented one is a false
// belief she acts on for days. It has to be in their own words.
func TestNewConcernKeepsOnlyWhatTheySaid(t *testing.T) {
	now := time.Now()
	theirs := []string{"not for long, got a job interview tomorrow at the clinic"}
	if _, ok := NewConcern(Plan{What: "clinic interview", When: "tomorrow"}, now, nil, theirs); !ok {
		t.Error("a plan in their own words was rejected")
	}
	if _, ok := NewConcern(Plan{What: "wedding in Porto", When: "tomorrow"}, now, nil, theirs); ok {
		t.Error("a plan they never mentioned was kept")
	}
	// Measured on the local model: "my brother is flying to Porto" came back
	// as cass's plan, in cass's own words.
	brother := []string{"my brother is flying to Porto tomorrow for his wedding"}
	for _, what := range []string{"brother flying to Porto", "flying to Porto"} {
		if _, ok := NewConcern(Plan{What: what, When: "tomorrow"}, now, nil, brother); ok {
			t.Errorf("%q, someone else's plan, was kept", what)
		}
	}
	if _, ok := NewConcern(Plan{What: "a very long plan that goes on and on", When: "later"}, now, nil, theirs); ok {
		t.Error("a paragraph was kept as a plan")
	}
}

// The numbers promised when this was designed: fond of him against
// indifferent, the night before, the day after, four days on.
func TestSalienceFollowsTimingAndCare(t *testing.T) {
	due := time.Date(2026, 9, 17, 19, 0, 0, 0, time.UTC)
	c := Concern{What: "job interview", Due: due, Expires: due.Add(concernShelf)}
	cases := []struct {
		name      string
		at        time.Time
		closeness float64
		want      float64
	}{
		{"fond, night before", due.Add(-20 * time.Hour), 0.8, 0.15},
		{"fond, day after", due.Add(12 * time.Hour), 0.8, 0.71},
		{"fond, four days on", due.Add(96 * time.Hour), 0.8, 0.21},
		{"indifferent, night before", due.Add(-20 * time.Hour), 0, 0.04},
		{"indifferent, day after", due.Add(12 * time.Hour), 0, 0.21},
	}
	for _, tc := range cases {
		if got := c.Salience(tc.at, tc.closeness, 0); math.Abs(got-tc.want) > 0.02 {
			t.Errorf("%s: %.2f, want about %.2f", tc.name, got, tc.want)
		}
	}
	if c.Salience(due.Add(-72*time.Hour), 1, 0) != 0 {
		t.Error("on her mind three days before it happens")
	}
	if c.Salience(due.Add(concernShelf+time.Hour), 1, 0) != 0 {
		t.Error("still on her mind after it expired")
	}
	passed := c
	passed.Passed = 2
	if passed.Salience(due.Add(12*time.Hour), 0.8, 0) >= c.Salience(due.Add(12*time.Hour), 0.8, 0)/2 {
		t.Error("letting it pass twice barely weakened it")
	}
}

func TestPhraseSaysWhenAsAPersonWould(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	c := Concern{What: "job interview", Due: time.Date(2026, 9, 17, 19, 0, 0, 0, time.UTC)}
	if got := c.Phrase("Big M", now, nil); got != "On your mind: Big M's job interview was yesterday." {
		t.Errorf("got %q", got)
	}
	c.Due = time.Date(2026, 9, 19, 19, 0, 0, 0, time.UTC)
	if got := c.Phrase("Big M", now, nil); !strings.Contains(got, "is tomorrow") {
		t.Errorf("got %q", got)
	}
}

func TestMergeConcernsDedupesExpiresAndCaps(t *testing.T) {
	now := time.Now()
	held := []Concern{
		{What: "job interview", Due: now, Expires: now.Add(time.Hour)},
		{What: "old trip", Due: now.Add(-9 * 24 * time.Hour), Expires: now.Add(-time.Hour)},
	}
	noted := []Concern{
		{What: "the job interview", Due: now, Expires: now.Add(time.Hour)},
		{What: "exam", Due: now, Expires: now.Add(time.Hour)},
	}
	got := MergeConcerns(held, noted, now)
	if len(got) != 2 {
		t.Fatalf("got %v, want the interview once and the exam", got)
	}
	var many []Concern
	for _, w := range []string{"alpha trip", "bravo exam", "charlie match", "delta move", "echo party"} {
		many = append(many, Concern{What: w, Expires: now.Add(time.Hour)})
	}
	if got := MergeConcerns(nil, many, now); len(got) != MaxConcerns || got[0].What != "bravo exam" {
		t.Errorf("cap kept %v", got)
	}
}

func TestParseNotesReadsPlans(t *testing.T) {
	got := ParseNotes("FACT Big M: pet = a cat called Bo\nPLAN Big M: clinic interview | tomorrow\nPLAN Big M: no when", time.Now())
	if len(got) != 1 || len(got[0].Plans) != 1 {
		t.Fatalf("got %+v", got)
	}
	if p := got[0].Plans[0]; p.What != "clinic interview" || p.When != "tomorrow" {
		t.Errorf("plan %+v", p)
	}
}

func TestStateCarriesWhatIsOnHerMindLast(t *testing.T) {
	g := Grounding{Drives: Drives{Energy: 0.1}, OnMind: "On your mind: Big M's job interview was yesterday."}
	got := g.stateSentences()
	if len(got) != 2 || got[1] != g.OnMind {
		t.Errorf("state %q", got)
	}
}
