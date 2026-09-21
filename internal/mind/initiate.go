package mind

import (
	"context"
	"fmt"
	"strings"

	"github.com/keshon/server-domme/internal/ai"
	"github.com/keshon/server-domme/internal/memory"
)

// Opening is one thing she could start on her own. The code finds them; the
// model decides whether any is worth acting on. See Mind.Initiate.
type Opening struct {
	// Trigger is TriggerReach for going after one person, TriggerStart for
	// speaking up in a channel.
	Trigger     Trigger
	ChannelID   string
	ChannelName string
	UserID      string
	Username    string
	// Thread is the intention this would act on, when there is one.
	Thread *memory.Thread
	// Detail is what makes it an opening, in words: "you last talked three
	// days ago, and they have been around today".
	Detail string
	// Turns are the recent conversation in the channel.
	Turns []Turn
}

// Plan is what she decided to start, if anything.
type Plan struct {
	// Choice is the index into the openings, or -1 for nothing.
	Choice int
	// Why is her real reason, in her own words. It is written to memory with
	// what she says, so "why did you tag me" has a true answer — v1 answered
	// "because you were mentioned", which nothing had done.
	Why string
	// Intent is what she wants to get across.
	Intent  string
	Backend string
}

// initiativeRules are the judgement on starting something. v1 fired on a
// feeling and a timer, and in production that was a hostile ping out of
// nowhere thirteen hours after an argument; these are what stop it.
const initiativeRules = `How to decide:
- Most of the time the right answer is to do nothing. People who talk first all the time are tiring.
- Only start something she actually wants to: a real reason, something specific she has to say or ask. "Just checking in" is not a reason unless she genuinely misses them.
- Following up on something someone told her — how it went, whether they did it — is the best reason there is.
- Never start something to complain about being ignored, to pick up an old argument, or to be hostile out of nowhere.
- She knows only what is written here.`

const planShape = `Answer with one JSON object and nothing else:
{
  "choice": the number of the opening she acts on, or 0 for none,
  "why": "her real reason, in her own words, first person",
  "intent": "what she wants to get across — the gist, not the wording"
}`

// Initiate asks whether she wants to start any of the openings.
func (m *Mind) Initiate(ctx context.Context, s Scene, k Known, openings []Opening) (Plan, error) {
	if len(openings) == 0 {
		return Plan{Choice: -1}, nil
	}
	name := m.name(s)
	var sys strings.Builder
	fmt.Fprintf(&sys, "You are the inner life of %s, a person on a Discord server — not an assistant, "+
		"not a bot. Nobody is talking to her right now. You decide whether she wants to "+
		"start something herself.", name)
	if m.Character != nil && m.Character.Persona != "" {
		sys.WriteString("\n\nWho she is:\n" + m.Character.Persona)
	}
	sys.WriteString("\n\n" + initiativeRules + "\n\n" + planShape)

	var user strings.Builder
	user.WriteString(renderWorld(s, k))
	user.WriteString("\n\nWhat she could do now:")
	for i, o := range openings {
		fmt.Fprintf(&user, "\n\n%d. ", i+1)
		switch o.Trigger {
		case TriggerReach:
			fmt.Fprintf(&user, "Go to %s in #%s.", nameOr(o.Username), o.ChannelName)
		default:
			fmt.Fprintf(&user, "Say something in #%s.", o.ChannelName)
		}
		if o.Thread != nil {
			user.WriteString(" It would follow up on: " + oneLine(o.Thread.Text) + ".")
		}
		if o.Detail != "" {
			user.WriteString(" " + oneLine(o.Detail))
		}
		if len(o.Turns) > 0 {
			tail := o.Turns
			if len(tail) > transcriptTail {
				tail = tail[len(tail)-transcriptTail:]
			}
			user.WriteString("\nThe last few lines there:\n" + renderTranscript(tail, s.Now))
		}
	}
	user.WriteString("\n\nDoes she want to start any of these?")

	msgs := []ai.Message{
		{Role: ai.RoleSystem, Content: sys.String()},
		{Role: ai.RoleUser, Content: user.String()},
	}
	reply, backend, err := m.generate(ai.WithRaw(ai.WithTemperature(ctx, thinkingTemperature)), msgs)
	if err != nil {
		return Plan{Choice: -1, Backend: backend}, err
	}
	obj, ok := decodeObject(reply)
	if !ok {
		return Plan{Choice: -1, Backend: backend}, fmt.Errorf("%w: %q", ErrUnreadable, clip(reply, 200))
	}
	p := Plan{
		Choice:  int(num(obj, "choice")) - 1,
		Why:     str(obj, "why"),
		Intent:  str(obj, "intent"),
		Backend: backend,
	}
	if p.Choice < 0 || p.Choice >= len(openings) || p.Intent == "" {
		p.Choice = -1
	}
	return p, nil
}
