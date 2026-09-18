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
