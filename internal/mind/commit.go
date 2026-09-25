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
	"strings"

	"github.com/keshon/server-domme/internal/memory"
)

// Proposal kinds, as refusals report them.
const (
	proposalNote     = "note"
	proposalFeeling  = "feeling"
	proposalBetween  = "between"
	proposalLater    = "later"
	proposalRemember = "remember"
	proposalPerson   = "person"
	proposalThread   = "thread"
	proposalSelfFact = "self-fact"
	proposalGlance   = "glance"
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

// commitFeeling keeps a feeling that registered. The model names it and says
// what it is about; the code anchors it — the person only if they spoke
// here, the time, the weight the moment was given, the source — lets go of
// the ones that have faded, and replaces one about the same person and
// thing rather than holding it twice.
func (m *Mind) commitFeeling(s Scene, a Appraisal, present bool, from memory.Source) error {
	if a.Weight < memory.FeelingGone {
		m.refuse(s.GuildID, a.Backend, proposalFeeling, "too light to register")
		return nil
	}
	f := memory.Feeling{
		At: s.Now, What: clip(a.FeelingWhat, maxToward), About: clip(a.FeelingAbout, maxNoteChars),
		Weight: a.Weight, Source: from,
	}
	if present {
		f.Person = memory.Ref{ID: s.UserID, Name: s.Username}
	}
	return m.Memory.UpdateSelf(s.GuildID, func(me *memory.Self) {
		var kept []memory.Feeling
		for _, old := range me.Feelings {
			if old.Strength(s.Now) < memory.FeelingGone {
				continue
			}
			// The same feeling about the same person is one feeling,
			// whatever it is said to be about this time: in production
			// "mild amusement" at Big M was kept five times in six
			// minutes, each about a differently worded thing.
			if old.Person.ID == f.Person.ID && (sameAbout(old.About, f.About) || sameAbout(old.What, f.What)) {
				continue
			}
			kept = append(kept, old)
		}
		me.Feelings = append(kept, f)
	})
}

// sameAbout reports whether two feelings are about the same thing: mostly
// the same words.
func sameAbout(a, b string) bool {
	if a == "" || b == "" {
		return a == b
	}
	return overlap(strings.Fields(echoKey(a)), strings.Fields(echoKey(b))) >= noteOverlap
}
