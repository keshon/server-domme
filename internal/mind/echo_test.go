package mind

import "testing"

func TestEchoesCatchesACopiedLine(t *testing.T) {
	turns := []Turn{
		{UserID: "2", Username: "mira", Content: "are the pins getting purged again this week"},
		{UserID: "1", Username: "cass", Content: "no idea, ask a mod"},
	}
	for _, reply := range []string{"no idea, ask a mod", "No idea. Ask a mod!"} {
		if !Echoes(reply, turns) {
			t.Errorf("%q passed as her own words", reply)
		}
	}
	for _, reply := range []string{"ask a mod, they love that", "no", "this again"} {
		if Echoes(reply, turns) {
			t.Errorf("%q flagged as an echo", reply)
		}
	}
}

// Her own earlier line is not someone else's words.
func TestEchoesIgnoresHerOwnTurns(t *testing.T) {
	turns := []Turn{{FromBot: true, Content: "read the pins first"}}
	if Echoes("read the pins first", turns) {
		t.Error("counted her own line as an echo")
	}
}

// The same line typed slightly differently is the same line: "You are not my
// Domme yet" came back as "You're not my Domme yet". His name in front of it
// does not make it hers either.
func TestEchoesCatchesANearCopyAndOneWithHisNameOnIt(t *testing.T) {
	turns := []Turn{{UserID: "1", Username: "Big M", Content: "You are not my Domme yet"}}
	for _, reply := range []string{"You're not my Domme yet", "Big M: you are not my Domme yet"} {
		if !Echoes(reply, turns) {
			t.Errorf("%q passed as her own words", reply)
		}
	}
	for _, reply := range []string{"not yet, no", "i am not anyone's Domme by appointment"} {
		if Echoes(reply, turns) {
			t.Errorf("%q flagged as an echo", reply)
		}
	}
}

// Two ways of putting the same demand are the same ask; a different demand
// is not.
func TestSameAskSeesPastTheWording(t *testing.T) {
	first := "get him to say whether he actually meant it"
	if !SameAsk(first, "ask him again whether he meant it") {
		t.Error("the same demand in new words read as a new one")
	}
	if SameAsk(first, "tell him the pins are sorted") {
		t.Error("a different thing to say read as the same ask")
	}
	if SameAsk("", first) {
		t.Error("nothing read as an ask")
	}
}
