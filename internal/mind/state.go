package mind

import (
	"fmt"
	"strings"
)

// One description of how she is, for the prompt.
//
// Her state used to reach the model as separate sentences, each written by
// the feature that owned it: tiredness, loneliness, interest, being short
// with one person, fond of another, and then a mood. Each was measured and
// each worked alone, but together they could contradict each other — "you
// are in a bad mood, less patience" beside "you are fond of cass, more
// patience" — and a model handed a contradiction resolves it at random.
//
// Now one function reads all of it and writes at most four sentences with the
// contradictions already settled in Go, where the rule is visible and
// testable: a bad mood is not for the person she is fond of; a good one does
// not reach the person spoiling it; exhausted beats absorbed; lonely and tired
// means few words but staying in the conversation. Still instructions rather
// than a description of feelings, because that is the one thing about this
// prompt that has been measured three times over: a stated mood changes
// nothing, an instruction does.

// stateHeading introduces the description in the system prompt.
const stateHeading = "Right now:"

// Energy, company and interest bands. Only a pronounced state speaks.
const (
	exhaustedBelow = 0.25
	tiredBelow     = 0.45
	lonelyAbove    = 0.8
	absorbedAbove  = 0.75
)

// State is how she is right now as one short paragraph of instructions, or
// "" when nothing about her is pronounced enough to mention.
func (g Grounding) State() string {
	return strings.Join(g.stateSentences(), " ")
}

// stateSentences builds State: how the day is going, then company and
// interest, then the people present, each already reconciled with the rest.
func (g Grounding) stateSentences() []string {
	d := g.Drives
	unset := d == (Drives{})

	exhausted := !unset && d.Energy < exhaustedBelow
	tired := !unset && !exhausted && d.Energy < tiredBelow
	bad := !unset && d.Mood < -pronouncedMood
	good := !unset && d.Mood > pronouncedMood

	var out []string
	if day := daySentence(exhausted, tired, bad, good); day != "" {
		out = append(out, day)
	}

	// Company and interest, never on top of exhaustion: "as few words as
	// will do" and "say something with content in it" cannot both be
	// followed, and the more physical one wins.
	switch {
	case unset || exhausted:
	case d.Social > lonelyAbove && tired:
		out = append(out, "You have had nobody to talk to for a long time, so stay in the conversation even if you keep it short.")
	case d.Social > lonelyAbove:
		out = append(out, "You have had nobody to talk to for a long time. Engage with this rather than brushing it off.")
	case d.Arousal > absorbedAbove && !tired && !bad:
		out = append(out, "This has your attention. Say something with content in it rather than a one-liner.")
	}

	annoyed := ""
	if who, level := g.mostIrritating(); who != "" {
		if line := IrritationDirective(who, level); line != "" {
			switch {
			case good:
				line = fmt.Sprintf("Your good mood does not extend to %s. %s", who, line)
			case bad:
				line += fmt.Sprintf(" If the bad mood is for anyone, it is for %s.", who)
			}
			out = append(out, line)
			annoyed = who
		}
	}
	// Never for the person she is short with: being told to be curt with
	// someone and to go easy on them in one prompt is resolved at random.
	if who, level := g.mostLiked(annoyed); who != "" {
		if line := WarmthDirective(who, level); line != "" {
			if bad {
				line += fmt.Sprintf(" The bad mood is not for %s.", who)
			}
			out = append(out, line)
		}
	}
	return out
}

// daySentence is how her energy and her mood combine, as one instruction.
func daySentence(exhausted, tired, bad, good bool) string {
	switch {
	case exhausted && bad:
		return "You are exhausted and in a foul mood. As few words as will do, no playing along, and do not explain why."
	case exhausted && good:
		return "You are exhausted, though in a good mood. Few words, but not unkind ones."
	case exhausted:
		return "You are exhausted. Answer in as few words as will do, and do not elaborate or ask follow-up questions."
	case tired && bad:
		return "You are tired and short-tempered today. Keep it short, no playing along, and do not explain why."
	case tired && good:
		return "A good day, if a tired one. Warm, but brief."
	case tired:
		return "You are tired. Keep it shorter than usual."
	case bad:
		return "You are in a bad mood today. Short fuse: fewer words, no playing along, and do not explain why. It is not their fault, so no insults unless they earn one."
	case good:
		return "You are in a good mood today. Be more generous than usual: play along with it, tease rather than dismiss, and give them something to work with."
	default:
		return ""
	}
}
