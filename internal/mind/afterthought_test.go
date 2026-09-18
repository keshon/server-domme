package mind

import (
	"strings"
	"testing"
	"time"
)

// clipped is an afterthought every gate allows, so each test can close one.
func clipped() Afterthought {
	return Afterthought{Trigger: TriggerMention, Reply: "yes", Now: time.Now(), UserID: "u1"}
}

func TestMayAddAfterthoughtAfterAClippedAnswer(t *testing.T) {
	if !adds(clipped(), 0) {
		t.Error("refused with every gate open")
	}
}

func TestMayAddAfterthoughtGates(t *testing.T) {
	now := time.Now()
	cases := map[string]func(*Afterthought){
		"reply already said its piece": func(a *Afterthought) {
			a.Reply = "it is fine, not the first time and it will not be the last"
		},
		"first reply was late":  func(a *Afterthought) { a.Late = true },
		"after an afterthought": func(a *Afterthought) { a.Trigger = TriggerAfterthought },
		"after a volunteer":     func(a *Afterthought) { a.Trigger = TriggerRecall },
		"too tired":             func(a *Afterthought) { a.Drives = Drives{Energy: 0.2, Interest: 0.5} },
		"irritated with them":   func(a *Afterthought) { a.Tension = 0.5 },
		"cold towards them":     func(a *Afterthought) { a.Regard = -0.5 },
		"others are talking": func(a *Afterthought) {
			a.Turns = []Turn{
				{UserID: "u2", Content: "a", At: now.Add(-20 * time.Second)},
				{UserID: "u3", Content: "b", At: now.Add(-10 * time.Second)},
			}
		},
	}
	for name, closeIt := range cases {
		t.Run(name, func(t *testing.T) {
			a := clipped()
			a.Now = now
			closeIt(&a)
			if adds(a, 0) {
				t.Error("allowed anyway")
			}
		})
	}
}

// Caps first, roll last: the roll only ever refuses something already allowed.
func TestMayAddAfterthoughtIsRare(t *testing.T) {
	if adds(clipped(), 0.9) {
		t.Error("a high roll still allowed one")
	}
}

func TestAfterthoughtDelayIsABeat(t *testing.T) {
	if d := AfterthoughtDelay(0); d < 3*time.Second {
		t.Errorf("shortest pause %s", d)
	}
	if d := AfterthoughtDelay(0.999); d > 8*time.Second {
		t.Errorf("longest pause %s", d)
	}
}

func TestIsSkipReadsADecoratedSkip(t *testing.T) {
	for _, reply := range []string{"SKIP", "skip.", "(SKIP)", "SKIP — nothing to add", ""} {
		if !IsSkip(reply) {
			t.Errorf("%q not read as declining", reply)
		}
	}
	for _, reply := range []string{"entertain me, then", "skipping class again?"} {
		if IsSkip(reply) {
			t.Errorf("%q read as declining", reply)
		}
	}
}

func TestLastWordNoticesThePersonAnswering(t *testing.T) {
	mine := Turn{FromBot: true, MessageID: "b1", Content: "yes"}
	if !LastWord([]Turn{{UserID: "u1", Content: "bored?"}, mine}, "b1") {
		t.Error("her message is the last word and was not seen as such")
	}
	if LastWord([]Turn{mine, {UserID: "u1", Content: "why"}}, "b1") {
		t.Error("sent an afterthought after the person had already answered")
	}
}

func TestAfterthoughtDirectiveQuotesHerAndAllowsDeclining(t *testing.T) {
	got := AfterthoughtDirective("yes")
	if !strings.Contains(got, `"yes"`) || !strings.Contains(got, AfterthoughtSkip) {
		t.Errorf("directive: %q", got)
	}
}

func adds(a Afterthought, roll float64) bool {
	ok, _ := MayAddAfterthought(a, roll)
	return ok
}
