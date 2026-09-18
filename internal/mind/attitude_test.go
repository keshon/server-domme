package mind

import (
	"strings"
	"testing"
)

func TestAttitudePutsFeelingsBeforeStanding(t *testing.T) {
	cases := []struct {
		warmth, irritation, regard float64
		want                       string
	}{
		{0.9, 0.9, 1, "short with"},
		{0, 0.4, 0, "cool towards"},
		{0.7, 0, -1, "fond of"},
		{0.4, 0, 0, "likes"},
		{0, 0, 0.5, "well disposed to"},
		{0, 0, -0.5, "has little time for"},
		{0, 0, 0, "neutral"},
	}
	for _, c := range cases {
		if got := Attitude(c.warmth, c.irritation, c.regard); got != c.want {
			t.Errorf("Attitude(%.1f, %.1f, %.1f) = %q, want %q", c.warmth, c.irritation, c.regard, got, c.want)
		}
	}
}

func TestWantsFollowTheDrives(t *testing.T) {
	lonely := Wants(Drives{Energy: 0.8, Social: 0.9, Interest: 0.3})
	if len(lonely) == 0 || !strings.Contains(lonely[0], "company") {
		t.Errorf("lonely wants %v", lonely)
	}
	if got := Wants(Drives{Energy: 0.2, Social: 0.1, Interest: 0.3}); len(got) != 1 || !strings.Contains(got[0], "quiet") {
		t.Errorf("exhausted wants %v", got)
	}
	if Wants(Drives{}) != nil {
		t.Error("unset drives want something")
	}
}

// What the panel lists as "being told" has to be what the prompt carries.
func TestToldIsWhatThePromptCarries(t *testing.T) {
	g := Grounding{
		Drives:    Drives{Energy: 0.2, Social: 0.9, Interest: 0.5},
		Present:   []Acquaintance{{UserID: "1", Username: "Big M", Tension: 0.4}, {UserID: "2", Username: "cass", Closeness: 0.7}},
		Reception: ReceptionDirective("Big M", ReceptionPanned),
	}
	told := g.Told()
	if len(told) < 4 {
		t.Fatalf("Told = %v, want mood, both people and the reaction", told)
	}
	system := buildSystem(nil, g, DefaultBudget())
	for _, line := range told {
		if !strings.Contains(system, line) {
			t.Errorf("panel says she is told %q, the prompt does not carry it", line)
		}
	}
}
