package mind

import (
	"strings"
	"unicode"
)

// minEchoRunes is the shortest reply treated as a possible echo. Below it a
// match is as likely to be a real answer — "no" to "no?" — as a copy.
const minEchoRunes = 8

// Echoes reports whether reply is just something a person in turns already
// said, give or take case and punctuation.
//
// Observed while probing an unprompted remark: with nothing aimed at her, one
// relay answered with the last line of the transcript word for word — "no
// idea, ask a mod" — which posted under her name reads as mockery at best.
// Only other people's lines count; repeating herself is a separate problem and
// not one this is for.
func Echoes(reply string, turns []Turn) bool {
	said := echoKey(reply)
	if len([]rune(said)) < minEchoRunes {
		return false
	}
	for _, t := range turns {
		if !t.FromBot && echoKey(t.Content) == said {
			return true
		}
	}
	return false
}

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
