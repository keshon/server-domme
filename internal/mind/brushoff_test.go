package mind

import (
	"testing"
	"time"
)

func asked(now time.Time) []Turn {
	return []Turn{
		{UserID: "u1", Username: "Big M", Content: "guess what", At: now.Add(-2 * time.Minute)},
		{FromBot: true, MessageID: "q1", To: "u1", Content: "what did you break this time?", At: now.Add(-time.Minute)},
	}
}

func TestBrushedOffWhenTheyTurnToSomeoneElse(t *testing.T) {
	now := time.Now()
	id, ok := BrushedOff(asked(now), Snub{Speaker: "u1", ElsewhereAimed: true, Now: now})
	if !ok || id != "q1" {
		t.Errorf("BrushedOff = %q, %v; want the question counted", id, ok)
	}
}

// Every one of these is either not a snub or cannot be told from an answer,
// and a snub read wrongly means someone treated coldly for nothing.
func TestBrushedOffOnlyWhenItIsUnambiguous(t *testing.T) {
	now := time.Now()
	cases := map[string]struct {
		turns []Turn
		snub  Snub
	}{
		"an untagged message may be the answer": {
			asked(now), Snub{Speaker: "u1", ElsewhereAimed: false, Now: now},
		},
		"someone else talking to someone else": {
			asked(now), Snub{Speaker: "u2", ElsewhereAimed: true, Now: now},
		},
		"they already said something since": {
			append(asked(now), Turn{UserID: "u1", Content: "the printer", At: now}),
			Snub{Speaker: "u1", ElsewhereAimed: true, Now: now},
		},
		"too long ago to be ignoring her": {
			asked(now), Snub{Speaker: "u1", ElsewhereAimed: true, Now: now.Add(BrushOffWindow + time.Minute)},
		},
		"she did not ask anything": {
			[]Turn{{FromBot: true, MessageID: "q1", To: "u1", Content: "fine.", At: now}},
			Snub{Speaker: "u1", ElsewhereAimed: true, Now: now},
		},
		"her question was to someone else": {
			[]Turn{{FromBot: true, MessageID: "q1", To: "u9", Content: "and you?", At: now}},
			Snub{Speaker: "u1", ElsewhereAimed: true, Now: now},
		},
	}
	for name, c := range cases {
		if _, ok := BrushedOff(c.turns, c.snub); ok {
			t.Errorf("%s: counted as a snub", name)
		}
	}
}
