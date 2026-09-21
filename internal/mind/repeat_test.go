package mind

import (
	"testing"
)

// The conversation this was written for, as it happened.
func chickenTurns() []Turn {
	return []Turn{
		{UserID: "u1", Content: "it's your turn now"},
		{FromBot: true, Content: "fine. why did the chicken join discord? to get pecked at by strangers."},
		{UserID: "u1", Content: "hahaha good one"},
		{FromBot: true, Content: "why did the chicken join discord? it wanted to be in the group chat."},
		{UserID: "u1", Content: "this one is lame meeeh"},
	}
}

func TestRepeatsHerselfCatchesTheLoop(t *testing.T) {
	for _, reply := range []string{
		// Word for word, as the relay actually sent it.
		"why did the chicken join discord? to get pecked at by strangers.",
		// The same joke with a new tail: the same opening five words.
		"why did the chicken join the voice chat? to hear its own clucks echo",
	} {
		if _, ok := RepeatsHerself(reply, chickenTurns()); !ok {
			t.Errorf("%q passed as new", reply)
		}
	}
}

func TestRepeatsHerselfLetsNewThingsAndShortLinesThrough(t *testing.T) {
	for _, reply := range []string{
		"fine. i am out of chickens.",
		"no.",
		"tough crowd",
	} {
		if earlier, ok := RepeatsHerself(reply, chickenTurns()); ok {
			t.Errorf("%q flagged as repeating %q", reply, earlier)
		}
	}
}

func TestRepeatsHerselfCatchesAnOpeningHabit(t *testing.T) {
	her := func(lines ...string) []Turn {
		var turns []Turn
		for _, l := range lines {
			turns = append(turns, Turn{UserID: "u", Username: "Big M", Content: "hi"}, Turn{FromBot: true, Content: l})
		}
		return turns
	}

	// The production log: three mornings running, and a relay that opened
	// every reply with his name.
	if _, ok := RepeatsHerself("morning. keep it professional, and we won't have any issues",
		her("morning. keep it professional, or we'll revisit this", "morning. let's see if you can keep it that way")); !ok {
		t.Error("a third morning was let through")
	}
	if _, ok := RepeatsHerself("Big M, I've said my piece.",
		her("Big M, I'm not here to discuss my feelings.", "Big M, typos happen.")); !ok {
		t.Error("a third reply opening with his name was let through")
	}

	// Twice is a coincidence, and short words alone are how people talk.
	if _, ok := RepeatsHerself("morning again", her("sure thing", "morning to you")); ok {
		t.Error("a second morning was flagged")
	}
	if _, ok := RepeatsHerself("no.", her("no.", "no.")); ok {
		t.Error("a short no was flagged")
	}
}
