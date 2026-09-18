package mind

import (
	"strings"
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

func TestReadReceptionOnWhatPeopleActuallyWrote(t *testing.T) {
	cases := map[string]Reception{
		"hahaha good one":             ReceptionLiked,
		"lol":                         ReceptionLiked,
		"😂":                           ReceptionLiked,
		"this one is lame meeeh":      ReceptionPanned,
		"boring":                      ReceptionPanned,
		"you are repeating yourself":  ReceptionRepeating,
		"same joke again, lame":       ReceptionRepeating,
		"Tell me another one please":  ReceptionNone,
		"ok":                          ReceptionNone,
		"what did you have for lunch": ReceptionNone,
	}
	for content, want := range cases {
		if got := ReadReception(content); got != want {
			t.Errorf("ReadReception(%q) = %q, want %q", content, got, want)
		}
	}
}

func TestReceptionReachesTheNextReplyAndOnlyThatPersonsReply(t *testing.T) {
	g := Grounding{Reception: ReceptionDirective("Big M", ReceptionPanned)}
	if got := buildSystem(nil, g, DefaultBudget()); !strings.Contains(got, "Big M was not impressed") {
		t.Errorf("reception missing from the prompt:\n%s", got)
	}
}
