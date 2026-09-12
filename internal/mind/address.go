package mind

import "strings"

// Address is what a message carrying her name is doing with it.
type Address int

const (
	// AboutHer is someone talking about her to the room. Overheard, not
	// asked — the default, because it is much the commoner case and the
	// quieter reading.
	AboutHer Address = iota
	// AddressedToHer is someone speaking to her without an @mention.
	AddressedToHer
)

// Second and third person markers, in the two languages this bot is actually
// spoken to in.
//
// Deliberately a word list rather than a model call. This runs on every message
// in a watched channel, and a classifier there would cost a backend request per
// message rather than per reply — the difference between affordable and not on
// free relays. The cost of the cheap version is that it is sometimes wrong, and
// being wrong means she answers something she was not asked or stays out of
// something she could have joined. Both are things people do.
var (
	secondPerson = []string{
		"you", "your", "yours", "u", "ur",
		"ты", "тебя", "тебе", "тобой", "твой", "твоя", "твоё", "твое", "вы", "вам",
	}
	thirdPerson = []string{
		"she", "her", "hers", "he", "him", "his", "they", "them", "их",
		"она", "её", "ее", "ей", "неё", "нее", "ней",
	}
	// aboutVerbs follow a name when someone is talking about its owner rather
	// than to them: "domme would hate this", "domme thinks otherwise".
	aboutVerbs = []string{
		"would", "will", "could", "should", "might", "is", "was", "has", "had",
		"does", "did", "thinks", "said", "says", "seems", "never", "always",
		"бы", "был", "была", "думает", "сказала", "считает",
	}
)

// ClassifyAddress decides whether a message naming her is speaking to her or
// about her.
//
// The distinction is worth drawing because the two deserve very different odds
// of an answer: being named while someone talks to you is close to being asked
// a question, and being mentioned in passing is not an invitation at all.
// Without it both share one chance, and she either barges into conversations
// about her or ignores people addressing her by name.
//
// Ambiguity resolves to AboutHer. Overhearing is the commoner case, and the
// wrong answer there is silence rather than an interruption.
func ClassifyAddress(text string, names []string) Address {
	lower := strings.ToLower(strings.TrimSpace(text))
	if lower == "" {
		return AboutHer
	}

	// "domme, what do you reckon" and "domme: look at this" are the clearest
	// shape there is: a name used as a vocative, with the address punctuated.
	for _, name := range names {
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" {
			continue
		}
		for _, sep := range []string{",", ":", " -", " —"} {
			if strings.HasPrefix(lower, name+sep) {
				return AddressedToHer
			}
		}
	}

	// A third-person verb right after the name is someone describing her.
	// Checked before the pronouns, because "domme would know, wouldn't you
	// say" is about her despite the "you".
	for _, name := range names {
		if rest, ok := afterWord(lower, strings.ToLower(name)); ok {
			if startsWithAny(rest, aboutVerbs) {
				return AboutHer
			}
		}
	}

	hasSecond := anyWord(lower, secondPerson)
	hasThird := anyWord(lower, thirdPerson)

	switch {
	case hasSecond && !hasThird:
		return AddressedToHer
	case hasThird && !hasSecond:
		return AboutHer
	}

	// No pronouns either way: a question naming her is being asked of her,
	// anything else is a remark.
	if strings.Contains(lower, "?") {
		return AddressedToHer
	}
	return AboutHer
}

// afterWord returns what follows the first whole-word occurrence of needle.
func afterWord(haystack, needle string) (string, bool) {
	if needle == "" {
		return "", false
	}
	for from := 0; from+len(needle) <= len(haystack); {
		idx := strings.Index(haystack[from:], needle)
		if idx < 0 {
			return "", false
		}
		start := from + idx
		end := start + len(needle)
		if !alphanumericBefore(haystack, start) && !alphanumericAt(haystack, end) {
			return strings.TrimLeft(haystack[end:], " ,:;-—"), true
		}
		from = start + 1
	}
	return "", false
}

// startsWithAny reports whether text begins with any of words, as a word.
func startsWithAny(text string, words []string) bool {
	for _, w := range words {
		if strings.HasPrefix(text, w) && !alphanumericAt(text, len(w)) {
			return true
		}
	}
	return false
}

// anyWord reports whether any of words appears in text as a whole word.
func anyWord(text string, words []string) bool {
	for _, w := range words {
		if containsWord(text, w) {
			return true
		}
	}
	return false
}
