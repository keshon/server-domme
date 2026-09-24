package mind

import (
	"strings"
	"unicode"
)

// Garbled catches a generation that came apart: the same character or the
// same word over and over, for as long as the model was willing to keep
// going. In production she posted a wall of some nine hundred "3"s.
//
// Nothing reads it as language, so nothing downstream would catch it — it is
// not a control word, not an echo, not a repeat of anything she said. It is
// the sampler falling into a hole, and the only thing to do with one is to
// not send it.
//
// Short lines are left alone: "hahaha", "nooo" and "..." are writing.
func Garbled(reply string) bool {
	text := strings.TrimSpace(reply)
	runes := []rune(text)
	if len(runes) < minGarbledRunes {
		return false
	}
	counts := make(map[rune]int, len(runes))
	total := 0
	for _, r := range runes {
		if unicode.IsSpace(r) {
			continue
		}
		counts[r]++
		total++
	}
	if total == 0 {
		return false
	}
	if len(counts) < minGarbledKinds {
		return true
	}
	for _, n := range counts {
		if float64(n)/float64(total) > maxOneRuneShare {
			return true
		}
	}
	run, last := 0, ""
	for _, w := range strings.Fields(strings.ToLower(text)) {
		if w == last {
			run++
			if run >= maxWordRun {
				return true
			}
			continue
		}
		run, last = 1, w
	}
	return false
}

// What counts as coming apart. The shares are deliberately far from
// ordinary writing: in English no letter reaches a fifth of a message, and
// a line of forty characters drawn from two of them is not a message.
const (
	minGarbledRunes = 40
	minGarbledKinds = 3
	maxOneRuneShare = 0.6
	maxWordRun      = 8
)
