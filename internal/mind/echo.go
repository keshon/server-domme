package mind

import (
	"strings"
	"unicode"
)

// minEchoRunes is the shortest reply treated as a possible echo. Below it a
// match is as likely to be a real answer — "no" to "no?" — as a copy.
const minEchoRunes = 8

// SameAsk reports whether two gists are the same ask, mostly the same words.
// Her wording changes while the demand does not: "finish your sentence",
// "you still owe me the real one", "what sentence are you still afraid to
// finish" — thirteen times in one evening.
func SameAsk(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return overlap(strings.Fields(echoKey(a)), strings.Fields(echoKey(b))) >= askOverlap
}

// askOverlap is the share of words that makes two gists the same ask.
const askOverlap = 0.5

// Echoes reports whether reply is just something a person in turns already
// said, give or take case and punctuation.
//
// Observed while probing an unprompted remark: with nothing aimed at her, one
// relay answered with the last line of the transcript word for word — "no
// idea, ask a mod" — which posted under her name reads as mockery at best.
// Only other people's lines count; repeating herself is a separate problem and
// not one this is for.
func Echoes(reply string, turns []Turn) bool {
	said := echoKey(withoutSpeaker(reply, turns))
	if len([]rune(said)) < minEchoRunes {
		return false
	}
	words := strings.Fields(said)
	for _, t := range turns {
		if t.FromBot {
			continue
		}
		theirs := echoKey(t.Content)
		if theirs == said {
			return true
		}
		// Near enough is a copy too: "You are not my Domme yet" came back
		// as "You're not my Domme yet", and the exact match missed it.
		if len([]rune(theirs)) >= minEchoRunes && nearCopy(words, strings.Fields(theirs)) {
			return true
		}
	}
	return false
}

// nearCopy reports whether two lines are the same line typed twice: nearly
// all of the words of one are in the other, and they are nearly as long as
// each other. Length matters because most of a short line is in any long one
// that happens to use its words — "this again" is not a copy of "are the pins
// getting purged again this week".
func nearCopy(a, b []string) bool {
	short, long := len(a), len(b)
	if short > long {
		short, long = long, short
	}
	if long == 0 || float64(short)/float64(long) < echoLengths {
		return false
	}
	return overlap(a, b) >= echoOverlap
}

// How alike two lines must be to be the one line: the share of words in
// common, and how close in length they are.
const (
	echoOverlap = 0.8
	echoLengths = 0.7
)

// withoutSpeaker drops a name someone in the conversation is called, where
// a reply opens with it: "Big M: you are very pushy" is his line with his
// name stuck on, not hers.
func withoutSpeaker(reply string, turns []Turn) string {
	name, rest, ok := strings.Cut(reply, ":")
	if !ok || len([]rune(name)) > maxSpeakerName {
		return reply
	}
	for _, t := range turns {
		if !t.FromBot && t.Username != "" && strings.EqualFold(strings.TrimSpace(name), t.Username) {
			return strings.TrimSpace(rest)
		}
	}
	return reply
}

// maxSpeakerName is how long the thing before a colon may be and still be
// read as somebody's name.
const maxSpeakerName = 32

// echoKey reduces a line to its letters and digits, lowercased, with single
// spaces, so trailing punctuation or capitals do not hide a copy.
func echoKey(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		case unicode.IsSpace(r):
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}
