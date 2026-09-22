package mind

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/keshon/server-domme/internal/memory"
)

// Step 6 of docs/persona-v3.md: feelings with a cause, fading on their own.

func TestAFeelingIsKeptWithWhatItIsAboutAndWho(t *testing.T) {
	m, _ := newMind(t)
	m.Feelings = true
	s := sceneWith(him("your taste in music is tragic", noon))
	s.MessageID = "77"
	if err := m.Absorb(s, Appraisal{FeelingWhat: "stung", FeelingAbout: "his jab about my taste", Weight: 0.6}); err != nil {
		t.Fatal(err)
	}
	me, _ := m.Memory.Self(guildID)
	if len(me.Feelings) != 1 {
		t.Fatalf("feelings %+v", me.Feelings)
	}
	f := me.Feelings[0]
	if f.What != "stung" || f.Person.ID != "123" || f.Source.Kind != memory.Interpreted || f.Weight != 0.6 {
		t.Errorf("kept %+v", f)
	}

	// The same thing about the same person replaces it; something else is
	// held beside it.
	s.Now = noon.Add(time.Minute)
	_ = m.Absorb(s, Appraisal{FeelingWhat: "annoyed", FeelingAbout: "his jab about my taste", Weight: 0.5})
	_ = m.Absorb(s, Appraisal{FeelingWhat: "curious", FeelingAbout: "his band", Weight: 0.4})
	me, _ = m.Memory.Self(guildID)
	if len(me.Feelings) != 2 || me.Feelings[0].What != "annoyed" {
		t.Errorf("feelings %+v", me.Feelings)
	}
}

// Heavier feelings fade slower: a light one is gone in hours, a heavy one
// is still there the next morning.
func TestFeelingsFadeByWeight(t *testing.T) {
	light := memory.Feeling{At: noon, What: "amused", Weight: 0.2}
	heavy := memory.Feeling{At: noon, What: "hurt", Weight: 0.9}
	later := noon.Add(6 * time.Hour)
	if light.Strength(later) >= memory.FeelingGone {
		t.Errorf("a light feeling is still there after six hours: %.2f", light.Strength(later))
	}
	if heavy.Strength(noon.Add(18*time.Hour)) < memory.FeelingGone {
		t.Error("a heavy feeling was gone by the next morning")
	}
}

// With feelings on, the appraisal asks for a feeling, not a mood, and a
// mood volunteered anyway is not kept.
func TestFeelingsReplaceTheMood(t *testing.T) {
	m, p := newMind(t, `{"act":"ignore","mood":"grumpy"}`)
	m.Feelings = true
	s := sceneWith(him("hi", noon))
	a, err := m.Consider(context.Background(), s, Known{})
	if err != nil {
		t.Fatal(err)
	}
	if a.Mood != "" {
		t.Errorf("mood kept: %q", a.Mood)
	}
	sys := p.sent[0][0].Content
	if strings.Contains(sys, `"mood"`) || !strings.Contains(sys, `"feeling"`) {
		t.Errorf("shape asked for:\n%s", sys)
	}
}

// All live feelings are shown, most recent first, each with its age.
func TestLiveFeelingsAreShownMostRecentFirst(t *testing.T) {
	feelings := []memory.Feeling{
		{At: noon.Add(-3 * time.Hour), What: "stung", About: "the jab", Weight: 0.9},
		{At: noon.Add(-time.Hour), What: "curious", About: "his band", Weight: 0.5},
		{At: noon.Add(-20 * time.Hour), What: "amused", Weight: 0.2},
	}
	got := renderFeelings(feelings, noon, "her")
	if strings.Contains(got, "amused") {
		t.Errorf("a faded feeling was shown:\n%s", got)
	}
	if i, j := strings.Index(got, "curious"), strings.Index(got, "stung"); i < 0 || j < 0 || i > j {
		t.Errorf("not most recent first:\n%s", got)
	}
}

// Reflection folds what the day settled and lets it go.
func TestReflectionSettlesFeelings(t *testing.T) {
	m, _ := newMind(t, `{"summary":"s","lately":"l","people":[],"done":[],"threads":[],"settled":[1]}`)
	m.Feelings = true
	if err := m.Memory.UpdateSelf(guildID, func(me *memory.Self) {
		me.Feelings = []memory.Feeling{{At: noon, What: "stung", Weight: 0.9}}
	}); err != nil {
		t.Fatal(err)
	}
	if err := m.Memory.AddMoment(guildID, memory.Moment{At: noon, Text: "he apologised"}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Reflect(context.Background(), guildID, "Test", noon, noon.Add(2*time.Hour), nil); err != nil {
		t.Fatal(err)
	}
	me, _ := m.Memory.Self(guildID)
	if len(me.Feelings) != 0 {
		t.Errorf("still with her: %+v", me.Feelings)
	}
}

// Drives are facts: how long nobody has spoken to her, how long since
// anything weighed on her. What they mean is hers.
func TestDrivesAreFacts(t *testing.T) {
	s := Scene{Now: noon, QuietFor: 6 * time.Hour}
	got := renderDrives(s, Known{LastHeavy: noon.Add(-72 * time.Hour)})
	if !strings.Contains(got, "Nobody has spoken to her here for") || !strings.Contains(got, "weighed on her") {
		t.Errorf("drives %q", got)
	}
	for _, banned := range []string{"bored", "lonely", "wants"} {
		if strings.Contains(got, banned) {
			t.Errorf("a conclusion in the facts: %q", got)
		}
	}
	if renderDrives(Scene{Now: noon, QuietFor: time.Hour}, Known{}) != "" {
		t.Error("an hour's quiet was stated")
	}
}
