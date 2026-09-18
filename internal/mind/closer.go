package mind

import (
	"fmt"
	"strings"
	"unicode"
)

// Closers.
//
// "same", "ok", "lol", "yeah" after something she said close a topic rather
// than open one. The decision model weighed them like any other follow-up —
// eighty per cent odds of an answer — and an answer was then forced to exist:
// told to say something brief rather than pad, with nothing to say, she
// greeted again, echoed the word back and added filler ("hey, same here. just
// another day in the server"). People mostly let a closer be the end.
var closers = map[string]bool{
	"same": true, "same here": true, "me too": true, "you too": true, "u too": true,
	"ok": true, "okay": true, "k": true, "kk": true, "okie": true, "oki": true,
	"yeah": true, "yea": true, "ye": true, "yep": true, "yup": true, "ya": true, "yes": true,
	"sure": true, "true": true, "fair": true, "fair enough": true, "right": true,
	"nice": true, "cool": true, "neat": true, "good": true, "fine": true, "alright": true,
	"lol": true, "lmao": true, "haha": true, "hah": true, "ha": true, "heh": true,
	"mhm": true, "mm": true, "hm": true, "hmm": true,
	"not much": true, "nothing much": true, "nm": true, "nmu": true,
	"thanks": true, "thx": true, "ty": true, "np": true,
}

// IsCloser reports whether a message closes the topic rather than moving it:
// one of a short list of acknowledgements, or nothing but emoji and
// punctuation.
//
// Deliberately narrow. A false positive costs an answer someone wanted, which
// reads as her ignoring them; a false negative costs only the reply she would
// have given anyway.
func IsCloser(content string) bool {
	trimmed := strings.TrimSpace(content)
	// A question mark makes anything a question — "same?" asks for an answer.
	if trimmed == "" || strings.ContainsAny(trimmed, "?？") {
		return false
	}
	key := echoKey(trimmed)
	if key == "" {
		// Emoji and punctuation only: a reaction typed out.
		return true
	}
	// Repeated letters are how people type these: "okkk", "yeahhh", "lolll".
	return closers[key] || closers[squeeze(key)]
}

// squeeze collapses runs of the same letter, so "yeahhh" reads as "yeah" and
// "lolll" as "lol". A second pass for doubled words like "kk".
func squeeze(s string) string {
	var b strings.Builder
	var last rune
	for _, r := range s {
		if r == last && unicode.IsLetter(r) {
			continue
		}
		b.WriteRune(r)
		last = r
	}
	return b.String()
}

// FlatDirective is what she is told when she answers a closer anyway: that
// the conversation has gone flat, not to echo it, and one concrete thing to
// bring. Without the concrete thing the instruction was "say something", and
// "say something" with nothing to say is how the filler got written.
//
// Phrased as a plain instruction, not "if you say anything": by the time this
// is written the decision to speak has been made, and a conditional read as a
// second invitation to stay quiet — offered SKIP as well, she took it three
// times in four even with a fact about the person to hand.
func FlatDirective(name, said, bring string) string {
	if name == "" {
		name = "They"
	}
	line := fmt.Sprintf(
		"%s just said %q, which closes the topic. Do not greet them again, do not echo "+
			"what they said and do not fill the silence with small talk.",
		name, strings.TrimSpace(said))
	if bring != "" {
		line += " Bring something: " + bring
	}
	return line
}

// SomethingToBring picks what to bring when a conversation goes flat: a thing
// they told her or a thing she remembers, chosen by roll among what exists so
// the same fact is not raised every time. It reports whether it found anything
// concrete; without that it falls back to a question or letting it drop.
func SomethingToBring(name string, facts []Fact, remembered []Memory, roll float64) (string, bool) {
	var options []string
	for i, f := range facts {
		if i == renderedFacts {
			break
		}
		options = append(options, fmt.Sprintf(
			"you know %s's %s is %s — ask about it or needle them about it.",
			name, strings.ReplaceAll(f.Key, "_", " "), f.Value))
	}
	for i, m := range remembered {
		if i == 2 {
			break
		}
		options = append(options, fmt.Sprintf("you remember %s — bring it up if it fits.", m.Gist))
	}
	if len(options) == 0 {
		return "ask them something you actually want to know about them, or let it drop.", false
	}
	i := int(roll * float64(len(options)))
	if i >= len(options) {
		i = len(options) - 1
	}
	return options[i], true
}

// DeclineNote lets her not answer after all, for the approaches where a reply
// was likely but not owed. The same word as an afterthought's, so one parser
// serves both; see IsSkip.
const DeclineNote = "If this does not actually need an answer from you — it closes the topic, " +
	"or it was not really for you — reply with only " + AfterthoughtSkip + "."

// MayDecline reports whether an approach is one she is allowed to decline at
// the last moment. Only the ones she was not directly asked: a mention or a
// reply to her is owed an answer once she has decided to give one, and
// declining it after typing has shown would look like a fault.
func MayDecline(t Trigger) bool {
	return t == TriggerFollowUp || t == TriggerAbout
}
