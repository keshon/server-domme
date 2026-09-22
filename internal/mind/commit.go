package mind

// Commit. Everything the model says about her or anyone — a note, a feeling,
// a line on how things stand, something to remember, something to do, a
// fact about herself — is a proposal. It is written only after the code has
// checked it: the person it is about is really there, what it rests on is
// real and of the right kind, it is not a duplicate, and it does not
// overwrite something of a stronger kind. The code stamps where it came
// from; the model never cites a source it could have made up.
//
// Absorb, applyReflection and applySelfFacts are the three doors, and every
// write the model proposes goes through one of them. Whatever they refuse is
// logged with why: the rate of refusals is itself a measurement of how much a
// model invents. See docs/persona-v3.md, workstream I.

import (
	"github.com/keshon/server-domme/internal/memory"
)

// Proposal kinds, as refusals report them.
const (
	proposalNote     = "note"
	proposalFeeling  = "feeling"
	proposalBetween  = "between"
	proposalLater    = "later"
	proposalPerson   = "person"
	proposalThread   = "thread"
	proposalSelfFact = "self-fact"
)

// Longest texts accepted from a proposal, in characters. Longer is clipped,
// not refused: the length is mechanics, the content is hers.
const (
	maxNoteChars    = 300
	maxToward       = 80
	maxLaterChars   = 200
	maxSelfFactChar = 200
)

// refuse reports a proposal that was not written.
func (m *Mind) refuse(guildID, backend, kind, reason string) {
	m.Log.Info().
		Str("guild_id", guildID).
		Str("backend", backend).
		Str("kind", kind).
		Str("reason", reason).
		Msg("mind_proposal_refused")
}

// inScene reports whether someone spoke in the scene: the only people a
// moment's proposals may be about.
func inScene(s Scene, userID string) bool {
	if userID == "" {
		return false
	}
	for _, t := range s.Turns {
		if !t.FromBot && t.UserID == userID {
			return true
		}
	}
	return false
}

// mayOverwrite reports whether something of kind may replace what is there,
// which came from existing. A stronger kind is never overwritten silently.
func mayOverwrite(existing memory.Source, kind memory.Kind) bool {
	return existing.IsZero() || !existing.Kind.Outranks(kind)
}
