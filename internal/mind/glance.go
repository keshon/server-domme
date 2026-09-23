package mind

import (
	"context"
	"fmt"
	"strings"

	"github.com/keshon/server-domme/internal/ai"
	"github.com/keshon/server-domme/internal/memory"
)

// Going to look. Her walks happen on a timer: she passes through a channel
// she reads when the idle mind says so, and what she has is whatever the
// last pass caught. Asked "anything happening in there?", the honest answer
// was often that she had not been by — and once, that she had, ten minutes
// earlier, and could not reach it.
//
// So she may go and look. The appraisal names one channel she reads; the
// code decides whether she may (it must be a channel she reads, she must
// not have been by lately, and she has a small budget for a day), goes,
// and hands her the gist before she answers. She is away while she does it,
// the way anyone is who says "hang on".
//
// The model chooses; the code keeps the gate. Left to itself a model would
// look every time, which is a search engine with a character card — and
// perception that costs nothing is what workstream D exists to prevent.
// See docs/persona-v3.md, D and F3.

// glanceRules are how one look is read. A glance is not a transcript: what
// she comes back with is a few words of gist, the way anyone reports a room
// they just put their head into.
const glanceRules = `How to look:
- She puts her head into the room and takes in the gist, not every line.
- What she comes back with is a few words: what is going on in there, or what caught her. If nothing is going on, say so plainly.
- Her own words for it, not a summary of the transcript, and never a quote.
- She does not pass judgement on someone's work or words to anyone else. What she takes from the room is neutral unless she would say it to the person themselves.
- What she keeps is safe for work. Anything sexual or explicit she passes by.
- She knows only what is written here, and does not invent.`

const glanceShape = `Answer with one JSON object and nothing else:
{
  "caught": "what is going on in there, or what caught her — a few words, first person, or empty if the room has nothing in it",
  "weight": how much it got to her, 0 to 0.3
}`

// Glance is one look into a channel she reads: it writes what she took from
// it as a walk moment, and returns it for the answer she is about to give.
func (m *Mind) Glance(ctx context.Context, s Scene, w Walk) (string, error) {
	name := m.name(s)
	var sys strings.Builder
	fmt.Fprintf(&sys, "You are the inner life of %s, a person on a Discord server — not an assistant, "+
		"not a bot. She has just put her head into #%s to see what is going on in there.", name, w.Channel)
	if m.Character != nil && m.Character.Persona != "" {
		sys.WriteString("\n\nWho she is:\n" + m.Character.Persona)
	}
	sys.WriteString("\n\n" + glanceRules + "\n\n" + glanceShape)

	var user strings.Builder
	fmt.Fprintf(&user, "What has been said in #%s since she last passed through:\n", w.Channel)
	user.WriteString(renderTranscript(w.Lines, s.Now))
	user.WriteString("\n\nWhat does she take from it?")

	msgs := []ai.Message{
		{Role: ai.RoleSystem, Content: sys.String()},
		{Role: ai.RoleUser, Content: user.String()},
	}
	reply, backend, err := m.generate(ai.WithRaw(ai.WithTemperature(ctx, reflectTemperature)), msgs)
	if err != nil {
		return "", err
	}
	obj, ok := decodeObject(reply)
	if !ok {
		return "", fmt.Errorf("%w: %q", ErrUnreadable, clip(reply, 200))
	}
	caught := clip(str(obj, "caught"), maxMomentChars)
	if caught == "" {
		m.refuse(s.GuildID, backend, proposalGlance, "nothing came back from the room")
		return "", nil
	}
	weight := min(maxWalkWeight, max(0, num(obj, "weight")))
	err = m.Memory.AddMoment(s.GuildID, memory.Moment{
		At: s.Now, Channel: w.Channel, Text: "went to look at #" + w.Channel + ": " + caught,
		Weight: weight, Walk: true,
	})
	return caught, err
}
