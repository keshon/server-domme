package mind

import (
	"strings"
	"testing"
)

func TestSplitThoughtSeparatesThoughtFromMessage(t *testing.T) {
	thought, message, ok := SplitThought("<inner>he wants attention. fine.</inner>\nentertain me, then")
	if !ok || thought != "he wants attention. fine." || message != "entertain me, then" {
		t.Errorf("got %q / %q / %v", thought, message, ok)
	}
}

// A private thought reaching the channel cannot be taken back; a discarded
// reply is retried. Every case where the thought's end is unknown discards.
func TestSplitThoughtNeverLeaksAnUnendedThought(t *testing.T) {
	for _, reply := range []string{
		"<inner>he wants attention and i am going to",
		"sure. <INNER>this is going nowhere",
		"stray closer</inner> and a message",
		"<inner>only a thought</inner>",
		"",
	} {
		if _, message, ok := SplitThought(reply); ok {
			t.Errorf("SplitThought(%q) posted %q", reply, message)
		}
	}
}

// The wrong closing tags relays actually wrote. The thought's end is still
// clear, so the reply is kept.
func TestSplitThoughtAcceptsTheClosersRelaysWrite(t *testing.T) {
	for _, reply := range []string{
		"<inner>polite, curious<inner>\nread the pins",
		"<inner>polite, curious</ inner>\nread the pins",
		"<inner>polite, curious<\\inner>\nread the pins",
	} {
		thought, message, ok := SplitThought(reply)
		if !ok || thought != "polite, curious" || message != "read the pins" {
			t.Errorf("SplitThought(%q) = %q / %q / %v", reply, thought, message, ok)
		}
	}
}

// Ignoring the instruction costs the thought, not the message.
func TestSplitThoughtPostsAReplyWithNoTagAsItIs(t *testing.T) {
	thought, message, ok := SplitThought("yes")
	if !ok || thought != "" || message != "yes" {
		t.Errorf("got %q / %q / %v", thought, message, ok)
	}
}

func TestInnerVoiceIsAskedForLastAndNeverForAnAfterthought(t *testing.T) {
	c := &Character{Name: "Domme", Persona: "someone"}
	msgs := Build(c, Grounding{InnerVoice: true}, nil, DefaultBudget())
	if last := msgs[len(msgs)-1].Content; !strings.Contains(last, "<inner>") {
		t.Errorf("inner voice not the last instruction: %q", last)
	}

	for name, g := range map[string]Grounding{
		"afterthought": {InnerVoice: true, Afterthought: "one more line or SKIP"},
		"volunteer":    {InnerVoice: true, Volunteering: "cass is back after a month"},
	} {
		for _, m := range Build(c, g, nil, DefaultBudget()) {
			if strings.Contains(m.Content, "<inner>") {
				t.Errorf("%s: asked for a private thought", name)
			}
		}
	}
}
