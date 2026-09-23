package memory

import (
	"strings"
	"testing"
	"time"
)

func TestArcRoundTrips(t *testing.T) {
	s := open(t)
	start := time.Date(2026, 9, 23, 11, 10, 0, 0, s.Location())
	want := Arc{
		ChannelID: "555", Channel: "test-serverdomme", Started: start, Updated: start.Add(6 * time.Hour),
		People: []Ref{{ID: "365", Name: "Big M"}, {ID: "101", Name: "✨Duchess; the [first]"}},
		Text:   "Since late morning Big M and I have been circling whether he stays direct.",
		Source: Message(Interpreted, "999"),
	}
	if err := s.SetArc(guild, want); err != nil {
		t.Fatal(err)
	}
	got, ok, err := s.Arc(guild, "555")
	if err != nil || !ok {
		t.Fatalf("Arc = %v, %v", ok, err)
	}
	if got.Text != want.Text || got.Channel != want.Channel || !got.Started.Equal(want.Started) ||
		!got.Updated.Equal(want.Updated) || got.Source != want.Source || len(got.People) != 2 ||
		got.People[0] != want.People[0] || got.People[1].ID != "101" {
		t.Errorf("read back %+v", got)
	}
	if _, ok, _ := s.Arc(guild, "556"); ok {
		t.Error("an arc for a channel that has none")
	}
	if err := s.SetArc(guild, Arc{ChannelID: "../x", Text: "no"}); err == nil {
		t.Error("a channel id that names a path was accepted")
	}
}

// Once over, an arc is a moment of the day it was last touched, exactly
// once, and no longer an arc.
func TestClosedArcBecomesAMomentOnce(t *testing.T) {
	s := open(t)
	at := time.Date(2026, 9, 23, 17, 11, 0, 0, s.Location())
	if err := s.SetArc(guild, Arc{ChannelID: "555", Channel: "chat", Started: at.Add(-6 * time.Hour), Updated: at,
		People: []Ref{{ID: "365", Name: "Big M"}}, Text: "we circled one question all afternoon"}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		closed, err := s.CloseArc(guild, "555")
		if err != nil {
			t.Fatal(err)
		}
		if closed != (i == 0) {
			t.Errorf("close %d reported %v", i+1, closed)
		}
	}
	if _, ok, _ := s.Arc(guild, "555"); ok {
		t.Error("the arc is still there after closing")
	}
	day, err := s.Day(guild, at)
	if err != nil {
		t.Fatal(err)
	}
	if len(day.Moments) != 1 {
		t.Fatalf("day has %d moments, want 1", len(day.Moments))
	}
	m := day.Moments[0]
	if !strings.Contains(m.Text, "we circled one question") || m.Kind != Interpreted || m.Weight != ArcWeight ||
		m.Channel != "chat" || len(m.People) != 1 || m.People[0].ID != "365" || !m.At.Equal(at) {
		t.Errorf("moment = %+v", m)
	}
}

// Reflection closes the arcs of the days it looks back on, and leaves a
// conversation touched since alone.
func TestCloseArcsBeforeLeavesLaterOnesAlone(t *testing.T) {
	s := open(t)
	yesterday := time.Date(2026, 9, 22, 22, 0, 0, 0, s.Location())
	today := yesterday.Add(12 * time.Hour)
	for id, at := range map[string]time.Time{"1": yesterday, "2": today} {
		if err := s.SetArc(guild, Arc{ChannelID: id, Updated: at, Text: "talk in " + id}); err != nil {
			t.Fatal(err)
		}
	}
	n, err := s.CloseArcsBefore(guild, time.Date(2026, 9, 23, 0, 0, 0, 0, s.Location()))
	if err != nil || n != 1 {
		t.Fatalf("closed %d, %v; want 1", n, err)
	}
	if _, ok, _ := s.Arc(guild, "1"); ok {
		t.Error("yesterday's arc was not closed")
	}
	if _, ok, _ := s.Arc(guild, "2"); !ok {
		t.Error("today's arc was closed")
	}
}
