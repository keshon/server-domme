package mind

import (
	"fmt"
	"regexp"
	"time"
)

// Reception is how what she just said landed with the person answering it.
//
// The feelings she had were all slow: irritation from being pestered, and
// warmth and weight from a summary written after a conversation had gone
// quiet for six minutes. Within a conversation nothing moved. Someone could
// laugh at her joke, call the next one lame and tell her she was repeating
// herself, and she walked into the fourth reply exactly as she walked into the
// first — so the model had no reason to change course, and did not.
//
// Read from the words with a short list rather than a model call per message.
// It is crude, English-only, and applied only to a message that is answering
// her, which is what keeps "lame" about somebody else's game from counting.
type Reception string

const (
	ReceptionNone Reception = ""
	// ReceptionLiked is laughter or praise.
	ReceptionLiked Reception = "liked"
	// ReceptionPanned is being told it was bad.
	ReceptionPanned Reception = "panned"
	// ReceptionRepeating is being told she is repeating herself, which
	// needs its own answer: it is true, and she should change course.
	ReceptionRepeating Reception = "repeating"
)

// Reception effects.
const (
	// ReceptionWindow is how long a reaction colours her next reply to that
	// person. After it, whatever she says next is not a response to it.
	ReceptionWindow = 5 * time.Minute
	// LikedWarmth and PannedIrritation are the immediate nudges. Small: one
	// laugh is not friendship and one "meh" is not a feud, but several in a
	// row add up, which is the point.
	LikedWarmth      = 0.04
	PannedIrritation = 0.15
)

// Checked in this order, most specific first: "you are repeating yourself,
// lame" is about the repetition.
var (
	receptionRepeating = regexp.MustCompile(`(?i)\b(repeat(ing)?\s+(yourself|urself)|same\s+(joke|thing|one)|you\s+(already\s+)?said\s+that|heard\s+(that|this)\s+(one\s+)?(already|before))\b`)
	receptionPanned    = regexp.MustCompile(`(?i)\b(lame|boring|not\s+funny|unfunny|cringe|meh+|bad\s+one|terrible|awful|weak|dumb\s+joke|try\s+harder)\b`)
	receptionLiked     = regexp.MustCompile(`(?i)(\b(ha(ha)+|he(he)+|lol+|lmao+|rofl|good\s+one|nice\s+one|funny|love\s+(it|that)|well\s+played|brilliant)\b|😂|🤣|😆)`)
)

// ReadReception classifies a message answering her.
func ReadReception(content string) Reception {
	switch {
	case receptionRepeating.MatchString(content):
		return ReceptionRepeating
	case receptionPanned.MatchString(content):
		return ReceptionPanned
	case receptionLiked.MatchString(content):
		return ReceptionLiked
	default:
		return ReceptionNone
	}
}

// ReceptionDirective is the instruction for her next reply to that person, or
// "" for none. Phrased as how she takes it, not what to say: she should not
// perform for anyone, and a character who fishes for more after a laugh is as
// off as one who ignores being told she is boring.
func ReceptionDirective(name string, r Reception) string {
	if name == "" {
		name = "They"
	}
	switch r {
	case ReceptionLiked:
		return fmt.Sprintf(
			"That last thing you said landed with %s. You can be privately pleased — "+
				"do not fish for more, and if you go again, make it something new.", name)
	case ReceptionPanned:
		return fmt.Sprintf(
			"%s was not impressed by what you just said. Do not try the same thing again. "+
				"Take it the way you would — you are not here to perform for them, so do not apologise "+
				"and do not offer more to win them back.", name)
	case ReceptionRepeating:
		return fmt.Sprintf(
			"%s pointed out that you are repeating yourself, and they are right. Own it in "+
				"your own way and change course; do not repeat anything you have already said.", name)
	default:
		return ""
	}
}

// Received is a reaction she is still carrying.
type Received struct {
	UserID   string
	Username string
	Kind     Reception
	At       time.Time
}

// Current reports whether the reaction still colours her next reply.
func (r Received) Current(userID string, now time.Time) bool {
	return r.Kind != ReceptionNone && r.UserID == userID && now.Sub(r.At) <= ReceptionWindow
}
