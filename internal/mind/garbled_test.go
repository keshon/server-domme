package mind

import (
	"context"
	"strings"
	"testing"
)

// The wall of threes she posted on the real server, and the things that look
// like it but are somebody typing.
func TestGarbledCatchesAGenerationThatCameApart(t *testing.T) {
	for _, reply := range []string{
		strings.Repeat("3", 900),
		strings.Repeat("3333333333 ", 40),
		strings.Repeat("ha", 30),
		strings.Repeat("interesting ", 10),
	} {
		if !Garbled(reply) {
			t.Errorf("%.30q passed as writing", reply)
		}
	}
	for _, reply := range []string{
		"hahahaha",
		"...",
		"nooooo",
		"no no no no no",
		"no. no. no. absolutely not, and you know why",
		"that's the third time you've asked me that, and the answer has not moved. 3 strikes",
		strings.Repeat("this goes on and on. ", 10),
	} {
		if Garbled(reply) {
			t.Errorf("%.40q flagged as garbled", reply)
		}
	}
}

// One that came apart is not sent, and is not held against her as words.
func TestSpeakRefusesAGenerationThatCameApart(t *testing.T) {
	m, _ := newMind(t, strings.Repeat("3", 900))
	s := sceneWith(Turn{UserID: "1", Username: "Big M", Content: "how many times", At: noon})
	got, _, err := m.Speak(context.Background(), s, Known{}, Appraisal{Act: ActReply}, "")
	if err != ErrGarbled || got != "" {
		t.Errorf("sent %.20q, %v", got, err)
	}
}
