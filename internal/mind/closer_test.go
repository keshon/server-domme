package mind

import (
	"strings"
	"testing"
	"time"
)

func TestIsCloserOnWhatPeopleType(t *testing.T) {
	for _, c := range []string{"same", "Same.", "ok", "okkk", "yeahhh", "lol", "same here", "👍", "😂😂", "not much", "k"} {
		if !IsCloser(c) {
			t.Errorf("%q not read as closing the topic", c)
		}
	}
	for _, c := range []string{"same?", "ok but why", "how are you?", "yeah i got a cat now", "?", "what", "no"} {
		if IsCloser(c) {
			t.Errorf("%q read as closing the topic", c)
		}
	}
}

// Letting "same" go is how a conversation ends, not a bot failing to answer,
// so the rails that force an answer do not apply.
func TestDecideUsuallyLetsACloserGo(t *testing.T) {
	a := DefaultAttention()
	s := Situation{Trigger: TriggerFollowUp, Now: time.Now(), Closer: true, IgnoredLast: true, FirstApproach: true}
	if Decide(a, s, 0.5) != OutcomeIgnore {
		t.Error("answered a closer at even odds, overrides and all")
	}
	if Decide(a, s, 0.01) != OutcomeSpeak {
		t.Error("never answers a closer at all")
	}

	// A mention is someone asking, whatever they wrote.
	s.Trigger = TriggerMention
	if Decide(a, s, 0.5) != OutcomeSpeak {
		t.Error("a direct mention saying ok was treated as a closer")
	}
}

func TestFlatDirectiveBringsSomethingConcrete(t *testing.T) {
	facts := []Fact{{Key: "pet", Value: "a cat called Bo"}}
	bring, concrete := SomethingToBring("Big M", facts, nil, 0)
	if !concrete || !strings.Contains(bring, "a cat called Bo") {
		t.Errorf("did not bring the fact: %q", bring)
	}
	got := FlatDirective("Big M", "same", bring)
	for _, want := range []string{`"same"`, "Do not greet them again", "a cat called Bo"} {
		if !strings.Contains(got, want) {
			t.Errorf("flat directive missing %q: %q", want, got)
		}
	}
	if fallback, concrete := SomethingToBring("Big M", nil, nil, 0.3); fallback == "" || concrete {
		t.Errorf("fallback for someone she knows nothing about: %q, concrete=%v", fallback, concrete)
	}
}

func TestDeclineIsOfferedOnlyWhereItIsAllowed(t *testing.T) {
	c := &Character{Name: "Domme", Persona: "someone"}
	has := func(g Grounding) bool {
		for _, m := range Build(c, g, nil, DefaultBudget()) {
			if strings.Contains(m.Content, DeclineNote) {
				return true
			}
		}
		return false
	}
	if !has(Grounding{MayDecline: true}) {
		t.Error("decline not offered")
	}
	if has(Grounding{}) {
		t.Error("decline offered for an answer that is owed")
	}
	if MayDecline(TriggerMention) || MayDecline(TriggerReply) || !MayDecline(TriggerFollowUp) {
		t.Error("MayDecline covers the wrong triggers")
	}
}
