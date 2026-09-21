package mind

import (
	"regexp"
	"strings"
)

// Casual makes a reply look typed rather than composed.
//
// Models write the way they were trained to write: finished sentences, a full
// stop at the end, em-dashes, typographer's quotes. People in a chat do none
// of that, and it is the kind of tell nobody names but everybody notices.
// These are edits to the surface only, never to what she said, and they are
// made here rather than asked for in the prompt: asked, a model half-complies
// and then drifts back, and a rule about punctuation is prompt spent on
// punctuation.
//
// Deliberately no typos. Invented ones look invented, and once a reader has
// noticed the pattern the whole persona reads as a trick.
type CasualStyle struct {
	// Curt keeps the full stop. People mostly leave it off, which makes one
	// a signal — "yeah." is not "yeah" — and when she is short with someone
	// the signal is the point.
	Curt bool
	// SlipChance is the odds of dropping the apostrophes from casual
	// contractions in this message: "dont", "im", "thats". Never when Curt:
	// precision is part of being cutting.
	SlipChance float64
}

var (
	codeOrLink = regexp.MustCompile("`|https?://")
	// spacedDash is an em or en dash used as a clause break, with or
	// without the spaces round it.
	spacedDash = regexp.MustCompile(`\s*[—–]\s*`)
	semicolon  = regexp.MustCompile(`\s*;\s+`)
	// contraction matches the contractions people commonly type without the
	// apostrophe, and only those: "we're" and "i'll" would become other
	// words.
	contraction = regexp.MustCompile(`(?i)\b(don|can|won|didn|isn|doesn|wasn|couldn|wouldn|shouldn|haven)'t\b|\b(i)'m\b|\b(that|what|there|it)'s\b|\b(you|they)'re\b|\b(i|you)'ve\b`)
)

var typographic = strings.NewReplacer(
	"’", "'", "‘", "'", "“", `"`, "”", `"`, "…", "...",
)

// Casual applies the style to one reply. roll is in [0,1), taken as a
// parameter so every branch is testable.
func Casual(reply string, s CasualStyle, roll float64) string {
	// Code and links are copied, not typed: leave them exactly as they are.
	if codeOrLink.MatchString(reply) {
		return reply
	}

	out := typographic.Replace(reply)
	out = spacedDash.ReplaceAllString(out, " - ")
	out = semicolon.ReplaceAllString(out, ", ")

	if !s.Curt && s.SlipChance > 0 && roll < s.SlipChance {
		out = contraction.ReplaceAllStringFunc(out, func(m string) string {
			return strings.ReplaceAll(m, "'", "")
		})
	}

	if !s.Curt {
		out = dropFinalStop(out)
	}
	return strings.TrimSpace(out)
}

// dropFinalStop removes a single full stop at the very end. An ellipsis is
// kept — "sure..." is a different message from "sure" — and so is anything
// that is not a full stop.
func dropFinalStop(s string) string {
	s = strings.TrimRight(s, " \t\n")
	if strings.HasSuffix(s, ".") && !strings.HasSuffix(s, "..") {
		return strings.TrimSuffix(s, ".")
	}
	return s
}
