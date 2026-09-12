package mind

import (
	"strconv"
	"strings"
)

// SpeechStyle is the character's settled temperament, on 0..1 with 0.5 as
// unremarkable.
//
// Taken from the cognitum experiment's Biology, which had the structure right:
// a stable temperament under a changing mood. Mood on its own produces someone
// with no consistent self, which is the opposite of the thing being built.
//
// Dials rather than a named reference. "Write like <famous person>" is a large
// prior in very few tokens, and that is its whole appeal, but it composes
// badly — set against a mood directive the model picks whichever prior is
// stronger rather than combining them — and it drags in everything else about
// that person, which here would contradict a character whose first paragraph
// is that she is not a performer. A dial is also tunable; a name is a coin
// flip on which version of them the model has in mind.
type SpeechStyle struct {
	// Warmth is how kindly she reads. Low is not hostile, it is reserved.
	Warmth float64
	// Sarcasm is how much edge the humour carries.
	Sarcasm float64
	// Formality is register: low is lowercase and clipped, high is measured.
	Formality float64
	// Verbosity is how much she says when she has the option to say less.
	Verbosity float64
	// Dominance is how much she directs the exchange rather than following it.
	Dominance float64
}

// DefaultSpeechStyle is an unremarkable temperament: every dial at the middle,
// which produces no directives at all.
func DefaultSpeechStyle() SpeechStyle {
	return SpeechStyle{Warmth: 0.5, Sarcasm: 0.5, Formality: 0.5, Verbosity: 0.5, Dominance: 0.5}
}

// Temper dial names, as written in the character file.
const (
	dialWarmth    = "warmth"
	dialSarcasm   = "sarcasm"
	dialFormality = "formality"
	dialVerbosity = "verbosity"
	dialDominance = "dominance"
)

// The band around 0.5 that says nothing. A dial has to be set deliberately
// before it earns a line in the prompt.
const (
	lowDial  = 0.35
	highDial = 0.65
)

// Directives turns the temperament into instructions about how to write.
//
// Same shape as Drives.Directives and for the same measured reason: a
// description of how she sounds is not acted on, and an instruction is. These
// sit above the mood, because temperament is what she is like generally and
// mood is what she is like today — the more transient thing takes the later
// and stronger position.
//
// Only dials set away from the middle speak, so a character file that
// configures two of them costs two lines rather than five. Silence is the
// default on purpose: five hedges on every reply is how a strong instruction
// becomes a weak one.
func (s SpeechStyle) Directives() []string {
	if s == (SpeechStyle{}) {
		// Unset, not "cold, humourless and meek".
		return nil
	}

	var out []string

	switch {
	case s.Warmth < lowDial:
		out = append(out, "Keep a reserved distance. Warmth is earned here, not offered.")
	case s.Warmth > highDial:
		out = append(out, "Be warm with people, even when you are declining them.")
	}

	switch {
	case s.Sarcasm < lowDial:
		out = append(out, "Say things straight. No irony, no needling.")
	case s.Sarcasm > highDial:
		out = append(out, "Dry and sharp is your register, but never cruel to someone who did not ask for it.")
	}

	switch {
	case s.Formality < lowDial:
		out = append(out, "Write the way people type in chat: lowercase, clipped, no ceremony.")
	case s.Formality > highDial:
		out = append(out, "Write in full, measured sentences.")
	}

	switch {
	case s.Verbosity < lowDial:
		out = append(out, "Say less than you could. A short answer is a complete one.")
	case s.Verbosity > highDial:
		out = append(out, "Give an answer with some substance to it rather than a line.")
	}

	switch {
	case s.Dominance < lowDial:
		out = append(out, "Follow where the conversation goes rather than steering it.")
	case s.Dominance > highDial:
		out = append(out, "Set the terms of the exchange. You decide what is worth answering.")
	}

	return out
}

// parseDial reads one "name: 0.7" line from the character file's temper
// section, reporting whether it was understood.
//
// An unreadable line is ignored rather than fatal: this is authored content,
// and refusing to start the bot over a typo in a dial serves nobody. The whole
// section is optional, and a character with no temper section simply has no
// temperament directives.
func parseDial(line string, into *SpeechStyle) bool {
	name, value, found := strings.Cut(line, ":")
	if !found {
		return false
	}

	v, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || v < 0 || v > 1 {
		return false
	}

	switch strings.ToLower(strings.TrimSpace(name)) {
	case dialWarmth:
		into.Warmth = v
	case dialSarcasm:
		into.Sarcasm = v
	case dialFormality:
		into.Formality = v
	case dialVerbosity:
		into.Verbosity = v
	case dialDominance:
		into.Dominance = v
	default:
		return false
	}
	return true
}
