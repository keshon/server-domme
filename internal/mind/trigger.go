package mind

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Trigger is what brought a moment to her attention.
//
// It is a fact about how the message reached her, stated to the model in
// words, and it no longer carries odds: whether she answers is hers to
// decide. The code keeps only the rails; see docs/persona.md.
type Trigger string

// Triggers. The values are written to the journal, so they are frozen once
// shipped the same way stored values are everywhere else here.
const (
	// TriggerMention is a direct @mention.
	TriggerMention Trigger = "mention"
	// TriggerReply is a Discord reply to something she said.
	TriggerReply Trigger = "reply"
	// TriggerNamed is her name in a message, without an @.
	TriggerNamed Trigger = "named"
	// TriggerFollowUp is the next thing said by the person she is in the
	// middle of talking to, untagged — see chat.Service.followsUp.
	TriggerFollowUp Trigger = "follow-up"
	// TriggerOverheard is a message in the room not aimed at her at all.
	// Only considered in channels where she may speak up; see
	// chat.Service.overhear.
	TriggerOverheard Trigger = "overheard"
	// TriggerReach is her going after one person, who agreed to it.
	TriggerReach Trigger = "reach"
	// TriggerStart is her starting something in a channel on her own: an
	// intention come due, or a quiet room.
	TriggerStart Trigger = "start"
	// TriggerThen is a second thought: something she decided to add a moment
	// after her own reply, when she made it. See Appraisal.Then.
	TriggerThen Trigger = "then"
)

// Direct reports whether someone plainly addressed her. A direct approach
// left unanswered twice running looks like a broken bot, which is the one
// silence the code overrules; see chat.Service.overrule.
func Direct(t Trigger) bool {
	return t == TriggerMention || t == TriggerReply || t == TriggerNamed
}

// Owed reports whether a failure to deliver should be retried later. Only an
// answer is owed. Something she started belongs to its moment: delivered
// late it is stranger than never said at all.
func Owed(t Trigger) bool {
	return t != TriggerReach && t != TriggerStart && t != TriggerOverheard && t != TriggerThen
}

// Initiated reports whether she started this herself rather than answering.
func Initiated(t Trigger) bool {
	return t == TriggerReach || t == TriggerStart
}

// describe says how a moment reached her, for the prompt.
func (t Trigger) describe(name string) string {
	switch t {
	case TriggerMention:
		return name + " tagged you"
	case TriggerReply:
		return name + " replied to something you said"
	case TriggerNamed:
		return name + " said your name"
	case TriggerFollowUp:
		return name + " carried on talking with you"
	case TriggerOverheard:
		return name + " said something in the room — not to you"
	default:
		return name + " spoke"
	}
}

// SaysName reports whether text contains one of her names as a word.
//
// A string match rather than a model call: this runs on every message in a
// watched channel, and deciding whether a message is for her is what the
// appraisal is for once it gets there.
func SaysName(text string, names []string) bool {
	if text == "" || len(names) == 0 {
		return false
	}
	lower := strings.ToLower(text)
	for _, name := range names {
		name = strings.ToLower(strings.TrimSpace(name))
		if name != "" && containsWord(lower, name) {
			return true
		}
	}
	return false
}

// containsWord reports whether needle appears in haystack without a letter or
// digit pressed up against either end, so "veranda" does not match "vera".
// Not a regex word boundary: a name may contain spaces, dots or hyphens
// ("Server Domme", "Server-Domme"). Both must already be lowercased.
func containsWord(haystack, needle string) bool {
	for from := 0; from+len(needle) <= len(haystack); {
		idx := strings.Index(haystack[from:], needle)
		if idx < 0 {
			return false
		}
		start := from + idx
		end := start + len(needle)
		if !alphanumericBefore(haystack, start) && !alphanumericAt(haystack, end) {
			return true
		}
		from = start + 1
	}
	return false
}

func alphanumericBefore(s string, i int) bool {
	if i <= 0 {
		return false
	}
	r, _ := utf8.DecodeLastRuneInString(s[:i])
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

func alphanumericAt(s string, i int) bool {
	if i >= len(s) {
		return false
	}
	r, _ := utf8.DecodeRuneInString(s[i:])
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

// CleanNames trims, drops empties and removes case-insensitive duplicates,
// preserving order. The first surviving entry is the canonical name. The
// names come from an env var and from Discord at once, so "ServerDomme,
// Server Domme" arrives with stray spaces and repeats.
func CleanNames(names []string) []string {
	seen := make(map[string]bool, len(names))
	out := make([]string, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" || seen[strings.ToLower(name)] {
			continue
		}
		seen[strings.ToLower(name)] = true
		out = append(out, name)
	}
	return out
}
