package mind

import (
	"strings"
	"testing"
)

func stateOf(d Drives, present ...Acquaintance) string {
	return Grounding{Drives: d, Present: present}.State()
}

// The contradictions the separate sentences used to hand the model, each
// settled in one direction.
func TestStateSettlesContradictions(t *testing.T) {
	awake := Drives{Energy: 0.8, Social: 0.3, Arousal: 0.4}

	bad := awake
	bad.Mood = -0.7
	got := stateOf(bad, Acquaintance{Username: "cass", Closeness: 0.8})
	if !strings.Contains(got, "not for cass") {
		t.Errorf("a bad day and someone she is fond of, unreconciled: %s", got)
	}

	good := awake
	good.Mood = 0.7
	got = stateOf(good, Acquaintance{Username: "Big M", Tension: 0.9})
	if !strings.Contains(got, "does not extend to Big M") {
		t.Errorf("a good day and someone pushing her, unreconciled: %s", got)
	}

	absorbedButSpent := Drives{Energy: 0.1, Social: 0.2, Arousal: 0.95}
	if got := stateOf(absorbedButSpent); strings.Contains(got, "content in it") {
		t.Errorf("told to be brief and to say something with content: %s", got)
	}

	lonelyAndTired := Drives{Energy: 0.35, Social: 0.95}
	got = stateOf(lonelyAndTired)
	if !strings.Contains(got, "shorter") || !strings.Contains(got, "stay in the conversation") {
		t.Errorf("lonely and tired should be brief but stay: %s", got)
	}
}

// Energy and mood are one sentence, not two that each assume the other is
// neutral.
func TestStateCombinesEnergyAndMood(t *testing.T) {
	cases := map[string]Drives{
		"exhausted, foul":  {Energy: 0.1, Mood: -0.8},
		"exhausted, happy": {Energy: 0.1, Mood: 0.8},
		"tired, foul":      {Energy: 0.35, Mood: -0.8},
		"tired, happy":     {Energy: 0.35, Mood: 0.8},
	}
	seen := make(map[string]string)
	for name, d := range cases {
		got := Grounding{Drives: d}.stateSentences()
		if len(got) != 1 {
			t.Errorf("%s: %d sentences, want one: %q", name, len(got), got)
			continue
		}
		if other, dup := seen[got[0]]; dup {
			t.Errorf("%s reads the same as %s", name, other)
		}
		seen[got[0]] = name
	}
}

// The same person is never both the one she is short with and the one she is
// fond of.
func TestStateNeverWarmAndShortWithOnePerson(t *testing.T) {
	got := stateOf(Drives{Energy: 0.8}, Acquaintance{Username: "cass", Tension: 0.9, Closeness: 0.9})
	if strings.Contains(got, "fond of cass") {
		t.Errorf("short with cass and fond of cass at once: %s", got)
	}
}

// The panel shows exactly what the model gets, heading aside.
func TestBuildCarriesTheStateVerbatim(t *testing.T) {
	g := Grounding{Drives: Drives{Energy: 0.1, Mood: -0.8}}
	system := buildSystem(nil, g, DefaultBudget())
	if !strings.Contains(system, stateHeading+"\n"+g.State()) {
		t.Errorf("the prompt does not carry the state as the panel shows it:\n%s", system)
	}
}
