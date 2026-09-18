package mind

import (
	"strings"
	"time"
)

// Being brushed off.
//
// Cognitum expected an answer within ninety seconds of anything it said and
// grew anxious when none came, and that anxiety fed on itself until it was
// the only thing the character had left. The idea is right — people notice
// being ignored — and the implementation is what not to copy: it fired on
// silence, which is usually someone making tea, and it accumulated without
// a floor.
//
// Here it fires only on an unambiguous snub: she asked someone a question and
// they turned and spoke to somebody else instead. It feeds the per-person
// irritation that already exists, so it decays on the same ninety-minute
// halflife and cannot build into a mood of its own.
const (
	// BrushOffWindow is how long after her question a snub still counts as
	// one. Later than this they have simply moved on, which is allowed.
	BrushOffWindow = 10 * time.Minute
	// BrushOffStep is what one snub adds to irritation with that person:
	// just past the band where she is a little cooler with them, which the
	// halflife brings back under in about a quarter of an hour.
	BrushOffStep = 0.34
)

// Snub describes the message that might be one.
type Snub struct {
	// Speaker is who sent it.
	Speaker string
	// ElsewhereAimed is whether it was plainly aimed at someone other than
	// her — a Discord reply to another person's message, or an @mention of
	// someone else. An untagged message is never enough: it may well be the
	// answer to her question, and she cannot tell.
	ElsewhereAimed bool
	Now            time.Time
}

// BrushedOff reports whether a message is someone ignoring her question, and
// if so the id of the question, so the same one is never counted twice.
//
// turns is the conversation as it stood before this message.
func BrushedOff(turns []Turn, s Snub) (string, bool) {
	if !s.ElsewhereAimed || s.Speaker == "" {
		return "", false
	}

	for i := len(turns) - 1; i >= 0; i-- {
		t := turns[i]
		if !t.FromBot {
			// They already said something after her question, which may have
			// been the answer. Not a snub.
			if t.UserID == s.Speaker {
				return "", false
			}
			continue
		}
		// Her latest message: the only one that can be ignored now.
		if t.To != s.Speaker || t.MessageID == "" || s.Now.Sub(t.At) > BrushOffWindow {
			return "", false
		}
		if !strings.HasSuffix(strings.TrimSpace(t.Content), "?") {
			return "", false
		}
		return t.MessageID, true
	}
	return "", false
}
