package mind

import (
	"fmt"
	"strings"
)

// Repetition.
//
// Her own earlier messages are replayed to the model as its own turns, which
// is what lets her avoid saying the same thing twice — and which a small model
// reads the other way, as examples of what to say. Observed: asked for another
// joke, she told the same one again word for word, and when told she was
// repeating herself, told a third with the same opening. The prompt already
// sees what she said; nothing checked what she was about to say.
const (
	// repeatOpening is how many opening words make two lines the same shape.
	// "why did the chicken join discord" three times in a row is one joke
	// being retold, whatever comes after it.
	repeatOpening = 5
	// repeatOverlap is the share of words two lines have to share to count
	// as one line reworded.
	repeatOverlap = 0.7
	// repeatMinWords keeps short lines out of it. "no." twice in a
	// conversation is how people talk.
	repeatMinWords = 4
)

// RepeatsHerself reports whether reply is something she has already said in
// turns, and returns the earlier line.
func RepeatsHerself(reply string, turns []Turn) (string, bool) {
	words := strings.Fields(echoKey(reply))
	if earlier, tic := openingHabit(words, turns); tic {
		return earlier, true
	}
	if len(words) < repeatMinWords {
		return "", false
	}
	for i := len(turns) - 1; i >= 0; i-- {
		t := turns[i]
		if !t.FromBot {
			continue
		}
		before := strings.Fields(echoKey(t.Content))
		if len(before) < repeatMinWords {
			continue
		}
		if sameOpening(words, before) || overlap(words, before) >= repeatOverlap {
			return t.Content, true
		}
	}
	return "", false
}

// habitRuns is how many of her lines in a row opening the same way make a
// habit. Two is a coincidence; the third is a tic the reader has noticed.
const habitRuns = 2

// openingHabit reports whether reply opens the way each of her last
// habitRuns lines did.
//
// The word checks above miss it because the tails differ. In production she
// opened "morning." three times running, and a relay opened a dozen replies
// in a row with "Big M, …"; both read as a machine long before anything she
// said did. The opening is the first word when it is a real word, else the
// first two, so "no" and "i" alone never count.
func openingHabit(words []string, turns []Turn) (string, bool) {
	open := opening(words)
	if open == "" {
		return "", false
	}
	var seen int
	var first string
	for i := len(turns) - 1; i >= 0 && seen < habitRuns; i-- {
		if !turns[i].FromBot {
			continue
		}
		if opening(strings.Fields(echoKey(turns[i].Content))) != open {
			return "", false
		}
		seen++
		first = turns[i].Content
	}
	return first, seen == habitRuns
}

func opening(words []string) string {
	switch {
	case len(words) == 0:
		return ""
	case len([]rune(words[0])) >= 5:
		return words[0]
	case len(words) >= 2:
		return words[0] + " " + words[1]
	default:
		return ""
	}
}

// RepeatNote asks for the reply again without the repetition.
func RepeatNote(earlier string) string {
	return fmt.Sprintf(
		"You already said %q in this conversation. Do not say it again, anything shaped "+
			"like it, or anything that opens the same way. Say something new — or, if you are out of material, say so in your own way.",
		strings.TrimSpace(earlier))
}

func sameOpening(a, b []string) bool {
	if len(a) < repeatOpening || len(b) < repeatOpening {
		return false
	}
	for i := 0; i < repeatOpening; i++ {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// overlap is the share of the shorter line's distinct words the other also
// has.
func overlap(a, b []string) float64 {
	setA := make(map[string]bool, len(a))
	for _, w := range a {
		setA[w] = true
	}
	setB := make(map[string]bool, len(b))
	for _, w := range b {
		setB[w] = true
	}
	small, large := setA, setB
	if len(small) > len(large) {
		small, large = large, small
	}
	if len(small) == 0 {
		return 0
	}
	var shared int
	for w := range small {
		if large[w] {
			shared++
		}
	}
	return float64(shared) / float64(len(small))
}
