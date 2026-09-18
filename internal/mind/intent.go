package mind

import (
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Attention holds how readily the character answers each kind of approach.
//
// These are probabilities, not certainties, because someone who answers every
// single time is recognisably a machine: the tell is not what a bot says, it
// is that it always says something. Silence has to be in the repertoire for
// speech to read as a choice.
//
// The rails in Decide matter more than the numbers. A bot that ignores you is
// indistinguishable from a bot that is broken, so ignoring is bounded by
// conditions that can be stated rather than tuned — see Decide.
type Attention struct {
	// MentionChance is the odds of answering a direct @mention.
	MentionChance float64
	// NamedChance is the odds of answering when someone says her name without
	// addressing her. Lower on purpose: being talked about is an invitation,
	// not a question.
	NamedChance float64
	// ReplyChance is the odds of answering a Discord reply to something she
	// said. Highest of the three — she started that exchange.
	ReplyChance float64
	// EngagedWindow is how recently she must have spoken for a channel to
	// count as an exchange in progress.
	EngagedWindow time.Duration
	// EngagedBoost is added to MentionChance and ReplyChance inside that
	// window: dropping out halfway through a back-and-forth reads as a fault,
	// not as reticence.
	EngagedBoost float64
	// CrowdingPenalty is subtracted from NamedChance inside that window, so
	// she does not answer every passing use of her name in a conversation she
	// is already part of.
	CrowdingPenalty float64
	// AboutChance is the odds of chiming in when she is talked about rather
	// than to. Low: being mentioned in passing is not an invitation, and a
	// character who answers every remark about herself is a character with
	// nothing better to do.
	AboutChance float64
	// FollowUpChance is the odds of answering the next thing said by whoever
	// she is mid-conversation with, untagged. High, but short of certain: an
	// open exchange is strong evidence a message is for her, not proof.
	FollowUpChance float64
	// CloserChance replaces FollowUpChance and ReplyChance for a message that
	// closes the topic — "same", "ok", "lol". Low: people mostly let those be
	// the end. See IsCloser.
	CloserChance float64
}

// DefaultAttention returns settings that answer nearly every direct approach
// and about half of the indirect ones.
func DefaultAttention() Attention {
	return Attention{
		MentionChance: 0.88,
		// Higher than it was when this trigger also covered being talked
		// about. Someone using her name and speaking to her is asking a
		// question in all but punctuation; the passing remarks that used to
		// share this number are now TriggerAbout.
		NamedChance:   0.65,
		AboutChance:   0.25,
		ReplyChance:   0.92,
		EngagedWindow: 3 * time.Minute,
		// Enough to carry a mention or a reply to certainty inside an open
		// exchange. Staying quiet is for cold approaches; dropping a direct
		// question mid-conversation does not read as reticence, it reads as
		// the bot being broken — which is what it looked like in testing.
		EngagedBoost:    0.12,
		CrowdingPenalty: 0.20,
		FollowUpChance:  0.80,
		CloserChance:    0.15,
	}
}

// Trigger is what made the character consider speaking.
type Trigger string

const (
	// TriggerMention is a direct @mention.
	TriggerMention Trigger = "mention"
	// TriggerNamed is her name appearing in a message that does not address
	// her — being talked about rather than talked to.
	TriggerNamed Trigger = "named"
	// TriggerReply is a Discord reply to a message of hers.
	TriggerReply Trigger = "reply"
	// TriggerAbout is her name coming up in a message aimed at the room
	// rather than at her. Overheard, not asked — answered least readily of
	// anything that reaches her at all.
	TriggerAbout Trigger = "about"
	// TriggerFollowUp is the next thing said by the person she is already
	// talking to, in a channel where she spoke last.
	//
	// People stop addressing someone by name once a conversation is running —
	// re-tagging every line is what you do with a machine. Without this she
	// answers the first message and then goes deaf, which is exactly how a bot
	// gives itself away.
	TriggerFollowUp Trigger = "follow-up"
)

// Outcome is what Decide concluded.
type Outcome string

const (
	// OutcomeSpeak means answer now.
	OutcomeSpeak Outcome = "speak"
	// OutcomeIgnore means she chose not to answer. It is final: an ignored
	// approach is never retried, which is exactly what separates it from the
	// deferral a failed backend produces. See Pending.
	OutcomeIgnore Outcome = "ignore"
)

// Situation is everything Decide needs, gathered by the caller.
type Situation struct {
	Trigger Trigger
	Now     time.Time
	// FirstApproach is true when she has never answered this person in this
	// guild. Ignoring someone's first ever approach is not reticence, it is a
	// bot that appears not to work.
	FirstApproach bool
	// IgnoredLast is true when she already ignored this person's previous
	// approach here. Twice running stops reading as character.
	IgnoredLast bool
	// LastSpokeAt is when she last spoke in this channel.
	LastSpokeAt time.Time
	// Drives is how she is doing. It moves the odds on the indirect
	// approaches only — see Drives.Nudge.
	Drives Drives
	// Irritation is how much this particular person has got on her nerves,
	// already decayed. It lowers their odds and nobody else's.
	Irritation float64
	// Regard is what this person's roles are worth to her, -1 to +1. Standing
	// rather than feeling: it is set by an operator and does not decay.
	Regard float64
	// Closer marks a message that closes the topic rather than moving it.
	Closer bool
}

// Decide reports whether to answer. roll is a value in [0,1) from the caller's
// random source, kept as a parameter so the decision is a pure function and
// every branch below is reachable from a test.
//
// The two overrides come first and are not probabilistic, because they are the
// cases where silence would be read as a defect rather than as a choice.
func Decide(a Attention, s Situation, roll float64) Outcome {
	outcome, _ := DecideWhy(a, s, roll)
	return outcome
}

// Decision is how Decide reached its answer: which rule applied, and for the
// odds, what they were. Kept for the journal, so "why did she not answer?"
// has an answer that is not a reconstruction.
type Decision struct {
	Rule   string
	Chance float64
}

// Decision rules.
const (
	RuleCloser        = "closer"
	RuleFirstApproach = "first approach"
	RuleIgnoredLast   = "ignored them last time"
	RuleOdds          = "odds"
)

// DecideWhy is Decide, reporting how it decided.
func DecideWhy(a Attention, s Situation, roll float64) (Outcome, Decision) {
	// A closer inside an exchange she is part of is weighed on its own and
	// skips the overrides: letting "same" go unanswered is not the silence
	// that reads as a broken bot, it is how a conversation ends.
	if s.Closer && (s.Trigger == TriggerFollowUp || s.Trigger == TriggerReply) {
		chance := clamp01(a.CloserChance + s.Drives.Nudge() + IrritationNudge(s.Irritation) + RegardNudge(s.Regard))
		d := Decision{Rule: RuleCloser, Chance: chance}
		if roll < chance {
			return OutcomeSpeak, d
		}
		return OutcomeIgnore, d
	}

	if s.FirstApproach {
		return OutcomeSpeak, Decision{Rule: RuleFirstApproach, Chance: 1}
	}
	if s.IgnoredLast {
		return OutcomeSpeak, Decision{Rule: RuleIgnoredLast, Chance: 1}
	}

	engaged := !s.LastSpokeAt.IsZero() && s.Now.Sub(s.LastSpokeAt) < a.EngagedWindow

	var chance float64
	switch s.Trigger {
	case TriggerMention:
		chance = a.MentionChance
		if engaged {
			chance += a.EngagedBoost
		}
	case TriggerReply:
		chance = a.ReplyChance
		if engaged {
			chance += a.EngagedBoost
		}
	case TriggerNamed:
		chance = a.NamedChance
		if engaged {
			chance -= a.CrowdingPenalty
		}
		chance += s.Drives.Nudge()
	case TriggerAbout:
		chance = a.AboutChance
		if engaged {
			chance -= a.CrowdingPenalty
		}
		chance += s.Drives.Nudge()
	case TriggerFollowUp:
		// No engagement adjustment: a follow-up only exists inside an open
		// exchange, so the boost would apply to every one of them and is
		// already priced into FollowUpChance.
		chance = a.FollowUpChance + s.Drives.Nudge()
	default:
		return OutcomeIgnore, Decision{Rule: RuleOdds}
	}

	// Applied to every trigger, including a direct one, and unlike the mood.
	// Being shorter with someone who is pushing is the whole point of holding
	// this per person, and the rails above still guarantee that a first
	// approach and a second-in-a-row are always answered — so this can never
	// turn into the silence that reads as a broken bot.
	chance += IrritationNudge(s.Irritation)
	chance += RegardNudge(s.Regard)

	d := Decision{Rule: RuleOdds, Chance: clamp01(chance)}
	if roll < d.Chance {
		return OutcomeSpeak, d
	}
	return OutcomeIgnore, d
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// SaysName reports whether text refers to the character by one of names.
//
// Deliberately a string match rather than a model call: this runs on every
// message in an opted-in channel, and a classifier there would cost a backend
// request per message, which is not affordable against free relays. The cost
// of the cheap version is that an oblique reference is missed, which fails
// towards silence — the safe direction.
//
// Stateless and allocation-light on purpose. The names are not fixed at
// startup: what she is called depends on the guild, because a nickname is
// per-guild and the account username is a third thing again, so the caller
// assembles the list per message rather than compiling patterns once. See
// chat.Service.namesFor.
func SaysName(text string, names []string) bool {
	if text == "" || len(names) == 0 {
		return false
	}
	lower := strings.ToLower(text)
	for _, name := range names {
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" {
			continue
		}
		if containsWord(lower, name) {
			return true
		}
	}
	return false
}

// containsWord reports whether needle appears in haystack without a letter or
// digit pressed up against either end. Both must already be lowercased.
//
// This is what stops "veranda" matching "vera". It is not a regex word
// boundary: a name may contain spaces, dots or hyphens ("Server Domme",
// "Server-Domme"), and the rule that survives all of those is simply that the
// characters either side are not alphanumeric.
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
	return isAlphanumeric(r)
}

func alphanumericAt(s string, i int) bool {
	if i >= len(s) {
		return false
	}
	r, _ := utf8.DecodeRuneInString(s[i:])
	return isAlphanumeric(r)
}

func isAlphanumeric(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

// CleanNames trims, drops empties and removes case-insensitive duplicates,
// preserving order. The first surviving entry is the canonical name.
//
// It exists because these come from an env var and from Discord at once:
// "ServerDomme, Server Domme" arrives with stray spaces, and the account
// username frequently repeats the configured name exactly.
func CleanNames(names []string) []string {
	seen := make(map[string]bool, len(names))
	out := make([]string, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, name)
	}
	return out
}
