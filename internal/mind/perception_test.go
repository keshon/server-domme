package mind

import (
	"strings"
	"testing"
	"time"

	"github.com/keshon/server-domme/internal/ai"
)

func TestSplitPerceptionTakesTheLabelOut(t *testing.T) {
	cases := map[string]struct {
		reply string
		label Perception
		msg   string
	}{
		"clean":            {"<tone>needling</tone>\nnice try", PerceivedNeedling, "nice try"},
		"same line":        {"<tone>warm</tone> glad you're back", PerceivedWarm, "glad you're back"},
		"decorated word":   {"<tone> **Playful.** </tone>\nha", PerceivedPlayful, "ha"},
		"wrong closing":    {"<tone>hostile<tone>\nno.", PerceivedHostile, "no."},
		"off the list":     {"<tone>curious</tone>\nwhat", "", "what"},
		"left out":         {"just the message", "", "just the message"},
		"word as the tag":  {"<neutral>unfortunately</neutral>", PerceivedNeutral, "unfortunately"},
		"word tag, line":   {"<Flirty>\nbehave", PerceivedFlirty, "behave"},
		"with the thought": {"<tone>flirty</tone>\n<inner>oh please</inner>\nbehave", PerceivedFlirty, "<inner>oh please</inner>\nbehave"},
	}
	for name, c := range cases {
		label, msg, ok := SplitPerception(c.reply)
		if !ok || label != c.label || msg != c.msg {
			t.Errorf("%s: got (%q, %q, %v), want (%q, %q)", name, label, msg, ok, c.label, c.msg)
		}
	}
}

// A tag left open must never reach the channel: the line goes with it.
func TestSplitPerceptionNeverLeaksAnOpenTag(t *testing.T) {
	_, msg, ok := SplitPerception("<tone>needling\nfine, whatever")
	if !ok || strings.Contains(strings.ToLower(msg), "tone") {
		t.Errorf("an open tag leaked: %q (ok %v)", msg, ok)
	}
	for _, only := range []string{"<tone>warm</tone>", "<warm>"} {
		if _, _, ok := SplitPerception(only); ok {
			t.Errorf("%q, a label with no message, was posted as one", only)
		}
	}
}

func TestBuildAsksForALabelOnlyOnAnAnswer(t *testing.T) {
	c := &Character{Name: "X", Persona: "someone"}
	asks := func(g Grounding) bool {
		for _, m := range Build(c, g, nil, DefaultBudget()) {
			if m.Role == ai.RoleSystem && m.Content == PerceiveNote {
				return true
			}
		}
		return false
	}
	base := Grounding{Now: time.Now(), Perceive: true}
	if !asks(base) {
		t.Error("an answer was not asked for a label")
	}
	for name, g := range map[string]Grounding{
		"speaking up":    {Now: time.Now(), Perceive: true, Volunteering: "a regular is back"},
		"reaching out":   {Now: time.Now(), Perceive: true, Reaching: "you miss them"},
		"second thought": {Now: time.Now(), Perceive: true, Afterthought: "one more line"},
		"switched off":   {Now: time.Now()},
	} {
		if asks(g) {
			t.Errorf("%s was asked for a label", name)
		}
	}
}

// The bug this caught: a reach-out clears Volunteering and sets Reaching, so
// the thought was asked for there and never taken back out.
func TestBuildNeverAsksForAThoughtWhenReachingOut(t *testing.T) {
	c := &Character{Name: "X", Persona: "someone"}
	g := Grounding{Now: time.Now(), InnerVoice: true, Reaching: "you miss them"}
	for _, m := range Build(c, g, nil, DefaultBudget()) {
		if m.Content == InnerVoiceNote {
			t.Fatal("asked for a private thought on a reach-out, which is posted unsplit")
		}
	}
}
