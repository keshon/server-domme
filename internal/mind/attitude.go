package mind

import "strings"

// Her state in words, for the people running the server.
//
// Numbers say how much; they do not say what she is like. These are read by an
// administrator looking at /chat state, never by the model — the model gets
// instructions (Drives.Directives, IrritationDirective, WarmthDirective), which
// are about how to write, not about how she feels. Same state, two readers.

// MoodWords describes how she is, from her drives: "awake, has had company,
// interested". Empty for unset drives.
func MoodWords(d Drives) string {
	if d == (Drives{}) {
		return ""
	}
	var parts []string
	switch {
	case d.Energy < 0.25:
		parts = append(parts, "exhausted")
	case d.Energy < 0.45:
		parts = append(parts, "tired")
	case d.Energy > 0.75:
		parts = append(parts, "sharp")
	default:
		parts = append(parts, "awake")
	}
	switch {
	case d.Social > 0.8:
		parts = append(parts, "lonely")
	case d.Social > 0.5:
		parts = append(parts, "been alone a while")
	case d.Social < 0.1:
		parts = append(parts, "has had company")
	}
	switch {
	case d.Interest > 0.75:
		parts = append(parts, "absorbed")
	case d.Interest > 0.5:
		parts = append(parts, "interested")
	case d.Interest < 0.2:
		parts = append(parts, "bored")
	}
	return strings.Join(parts, ", ")
}

// Wants is what her drives make her want right now, most pressing first.
// Derived, not stored: a want here is a drive read as a direction.
func Wants(d Drives) []string {
	if d == (Drives{}) {
		return nil
	}
	var out []string
	if d.Social > 0.6 {
		out = append(out, "company — someone to actually talk to")
	}
	if d.Energy < 0.35 {
		out = append(out, "quiet, and short exchanges")
	}
	if d.Interest > 0.6 && d.Energy >= 0.45 {
		out = append(out, "something with substance to get into")
	}
	if d.Interest < 0.2 && d.Energy >= 0.45 {
		out = append(out, "something to happen")
	}
	return out
}

// Attitude is her stance towards one person, in a word or two, from how she
// feels about them and what their roles are worth to her. Feelings first:
// being short with someone outranks liking them, the same order the prompt
// uses (see buildSystem), and both outrank standing, which is the settled
// background they move against.
func Attitude(warmth, irritation, regard float64) string {
	switch {
	case irritation > sharpAbove:
		return "short with"
	case irritation > coolAbove:
		return "cool towards"
	case warmth > fondAbove:
		return "fond of"
	case warmth > likesAbove:
		return "likes"
	case regard > 0.25:
		return "well disposed to"
	case regard < -0.25:
		return "has little time for"
	default:
		return "neutral"
	}
}

// ReceptionWords is how a reaction reads on the panel.
func ReceptionWords(r Reception) string {
	switch r {
	case ReceptionLiked:
		return "pleased — it landed"
	case ReceptionPanned:
		return "stung — it was panned"
	case ReceptionRepeating:
		return "caught repeating herself"
	default:
		return ""
	}
}
