package mind

import (
	"regexp"
	"strings"
)

// How their message came across to her, as the model read it.
//
// The Go side of her mind reacts only to what it can count — pestering, a
// laugh or a pan, the tone of a conversation after it settles — and cannot
// tell "you're actually funny" from "you're a bot, aren't you". The model can,
// and in the call that writes her answer it has the message in context
// anyway. So it is asked for one word from a fixed list alongside the reply:
// no extra request, and a label rather than free text, because free text fed
// back is how cognitum's reflection fixated, and letting the model set her
// feelings directly is how its state came to mean nothing.
//
// The model perceives; Go decides what that does to her. Until the labels
// have been seen to match what people actually meant, they only go in the
// journal (CHAT_PERCEPTION=shadow): the relays were measured answering "was
// this rude?" confidently and arbitrarily, and a label that moved her bond on
// that basis would make her moody at random.

// Perception is one of the words the model may use for how a message landed.
type Perception string

// The fixed list. Short on purpose: every word is one the relays have to
// tell apart reliably, and one a later appraisal has to have a rule for.
const (
	PerceivedWarm     Perception = "warm"
	PerceivedPlayful  Perception = "playful"
	PerceivedFlirty   Perception = "flirty"
	PerceivedNeedling Perception = "needling"
	PerceivedHostile  Perception = "hostile"
	PerceivedNeutral  Perception = "neutral"
)

var perceptions = []Perception{
	PerceivedWarm, PerceivedPlayful, PerceivedFlirty,
	PerceivedNeedling, PerceivedHostile, PerceivedNeutral,
}

// PerceiveNote asks for the label, before the message and before any private
// thought. Its own tag, so it can be taken out whether or not the inner voice
// is on, and a tag rather than a "TONE:" label for the reason the inner voice
// gives: ai.Clean cuts at label-shaped lines.
const PerceiveNote = "Before anything else, write how their last message came across to you, " +
	"as one word between <tone> and </tone>, chosen from: warm, playful, flirty, needling, hostile, neutral. " +
	"Nobody will see it. Then write your message as you would anyway."

var (
	toneBlock = regexp.MustCompile(`(?is)<tone>\s*(.*?)\s*(?:</\s*tone>|<\\tone>|<tone>)`)
	toneStray = regexp.MustCompile(`(?i)</?\s*\\?tone>`)
	// labelTag is the other shape relays wrote: the word itself used as the
	// tag — "<warm>", or "<neutral>unfortunately</neutral>" around the
	// message. Seen in two replies of eighteen when first measured, and both
	// would have been posted with the tag in them.
	labelTag = regexp.MustCompile(`(?i)</?\s*(warm|playful|flirty|needling|hostile|neutral)\s*/?>`)
)

// SplitPerception takes the label out of a reply.
//
// The label is "" when the model left it out or used a word not on the list;
// the message still stands, since a missing label costs the shadow journal a
// row and nothing else. A tag left open is cut out with the rest of its line,
// because a label reaching the channel reads as the bot's plumbing showing.
// ok is false only when nothing is left to post.
func SplitPerception(reply string) (Perception, string, bool) {
	var label Perception
	message := toneBlock.ReplaceAllStringFunc(reply, func(block string) string {
		if m := toneBlock.FindStringSubmatch(block); len(m) == 2 && label == "" {
			label = ParsePerception(m[1])
		}
		return ""
	})
	message = labelTag.ReplaceAllStringFunc(message, func(tag string) string {
		if m := labelTag.FindStringSubmatch(tag); len(m) == 2 && label == "" {
			label = ParsePerception(m[1])
		}
		return ""
	})
	if toneStray.MatchString(message) {
		var kept []string
		for _, line := range strings.Split(message, "\n") {
			if !toneStray.MatchString(line) {
				kept = append(kept, line)
			}
		}
		message = strings.Join(kept, "\n")
	}
	message = strings.TrimSpace(message)
	return label, message, message != ""
}

// ParsePerception reads a label, forgiving case and punctuation, or returns
// "" for a word that is not on the list.
func ParsePerception(word string) Perception {
	w := strings.ToLower(strings.Trim(strings.TrimSpace(word), ".,!*\"'`"))
	for _, p := range perceptions {
		if w == string(p) {
			return p
		}
	}
	return ""
}
