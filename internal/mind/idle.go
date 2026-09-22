package mind

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/keshon/server-domme/internal/ai"
	"github.com/keshon/server-domme/internal/memory"
)

// The idle mind: what happens in her when nobody is talking to her. Every
// so often, and on waking, one cheap call works out what is on her mind —
// and now and then she passes through a channel she reads without speaking
// in, and the call is what she took from it. It may also come up with an
// impulse: something she has and wants to act on. The code checks the
// impulse against the rails; see docs/persona-v3.md, E and H1.

// Walk is a pass through a channel she reads but does not speak in: the
// lines since her last pass there.
type Walk struct {
	Channel string
	Lines   []Turn
}

// Candidate is someone an impulse could be aimed at, by the name she knows
// them by, and where they are.
type Candidate struct {
	ID   string
	Name string
	// Here is whether they are in a room she answers in right now.
	Here bool
}

// Idle is what the idle mind is given beyond her memory.
type Idle struct {
	GuildID   string
	GuildName string
	Now       time.Time
	// Walk is set when this tick is a walk.
	Walk *Walk
	// Due are the intentions that have come due.
	Due []memory.Thread
	// People are who an impulse may be aimed at; Rooms whether there is a
	// room she may speak up in.
	People []Candidate
	Rooms  bool
	// QuietFor is how long since anyone spoke to her here, when known.
	QuietFor time.Duration
}

// Impulse is something she wants to act on: to someone, or to a room.
type Impulse struct {
	// Person is who it is for; nil for a room.
	Person *Candidate
	About  string
}

// IdleResult is what one tick of the idle mind came to.
type IdleResult struct {
	OnMind  string
	Caught  string
	Impulse *Impulse
	Backend string
}

// Walk moments weigh little: passing something in the street fades within a
// day or two unless something brings it back.
const maxWalkWeight = 0.3

// idleRules are how the idle mind works. Most answer a way this goes wrong:
// her mind filled with reflections on her own mood instead of things, an
// impulse with nothing behind it, a walk turned into a transcript, an
// opinion about someone carried to someone else.
const idleRules = `How to think:
- What is on her mind is usually a thing, not a mood: something she saw, something someone said, something she is in the middle of, a want. Often it has nothing to do with the last conversation.
- Most of the time there is no impulse. Only when she has something specific she wants to say or ask, to a particular person or a room — never "just checking in", never to complain about being ignored, never out of nowhere.
- On a walk she passes through; she does not read every line. What caught her is her gist in a few words, not a quote. Usually one thing or nothing.
- She does not pass judgement on someone's work or words behind their back. An opinion about someone goes to that person, or nowhere.
- She knows only what is written here, and does not invent.`

const idleShape = `Answer with one JSON object and nothing else:
{
  "on_mind": "what is on her mind right now, one short line, first person",
  "caught": "on a walk: what caught her, her gist in a few words, or empty",
  "caught_weight": how much it got to her, 0 to 0.3,
  "impulse": {"to": "the name of one of the people listed, or \"a room\", or empty", "about": "what she wants to say or ask, the gist"}
}`

// IdleThink runs one tick of the idle mind for a guild, commits what it may
// — what is on her mind, what she took from a walk — and returns an impulse
// for the caller to check against the rails.
func (m *Mind) IdleThink(ctx context.Context, in Idle) (IdleResult, error) {
	s := Scene{GuildID: in.GuildID, GuildName: in.GuildName, Now: in.Now, QuietFor: in.QuietFor}
	k, err := m.Know(s)
	if err != nil {
		return IdleResult{}, err
	}
	day, err := m.Memory.Day(in.GuildID, in.Now)
	if err != nil {
		return IdleResult{}, err
	}
	msgs := m.idlePrompt(s, k, day, in)
	reply, backend, err := m.generate(ai.WithRaw(ai.WithTemperature(ctx, reflectTemperature)), msgs)
	if err != nil {
		return IdleResult{Backend: backend}, err
	}
	obj, ok := decodeObject(reply)
	if !ok {
		return IdleResult{Backend: backend}, fmt.Errorf("%w: %q", ErrUnreadable, clip(reply, 200))
	}
	res := IdleResult{OnMind: clip(str(obj, "on_mind"), maxLaterChars), Backend: backend}
	if res.OnMind != "" {
		err := m.Memory.UpdateSelf(in.GuildID, func(me *memory.Self) {
			me.OnMind, me.OnMindAt = res.OnMind, in.Now
		})
		if err != nil {
			return res, err
		}
	}
	if in.Walk != nil {
		if caught := str(obj, "caught"); caught != "" {
			res.Caught = clip(caught, maxMomentChars)
			weight := min(maxWalkWeight, max(0, num(obj, "caught_weight")))
			err := m.Memory.AddMoment(in.GuildID, memory.Moment{
				At: in.Now, Channel: in.Walk.Channel, Text: "passed through #" + in.Walk.Channel + ": " + res.Caught,
				Weight: weight, Walk: true,
			})
			if err != nil {
				return res, err
			}
		}
	}
	res.Impulse = m.readImpulse(in, obj, backend)
	return res, nil
}

// readImpulse turns the model's impulse into one aimed at someone listed or
// at a room, refusing anything else: an impulse at someone who was not
// offered is a name the model made up.
func (m *Mind) readImpulse(in Idle, obj map[string]any, backend string) *Impulse {
	raw, ok := obj["impulse"].(map[string]any)
	if !ok {
		return nil
	}
	to, about := str(raw, "to"), clip(str(raw, "about"), maxLaterChars)
	if to == "" || about == "" {
		return nil
	}
	if strings.EqualFold(strings.Trim(to, `"' `), "a room") {
		if !in.Rooms {
			m.refuse(in.GuildID, backend, "impulse", "a room, where she may not speak first")
			return nil
		}
		return &Impulse{About: about}
	}
	name := strings.TrimPrefix(strings.TrimSpace(to), "@")
	for i := range in.People {
		if strings.EqualFold(in.People[i].Name, name) {
			return &Impulse{Person: &in.People[i], About: about}
		}
	}
	m.refuse(in.GuildID, backend, "impulse", "aimed at someone not offered")
	return nil
}

func (m *Mind) idlePrompt(s Scene, k Known, day memory.Day, in Idle) []ai.Message {
	name := m.name(s)
	var sys strings.Builder
	fmt.Fprintf(&sys, "You are the inner life of %s, a person on a Discord server — not an assistant, "+
		"not a bot. Nobody is talking to her right now. You work out what is on her mind", name)
	if in.Walk != nil {
		sys.WriteString(", and what caught her as she passed through a channel")
	}
	sys.WriteString(".")
	if m.Character != nil && m.Character.Persona != "" {
		sys.WriteString("\n\nWho she is:\n" + m.Character.Persona)
	}
	if sp := renderSpecifics("Specifically:", k.Specifics); sp != "" {
		sys.WriteString("\n\n" + sp)
	}
	sys.WriteString("\n\n" + idleRules + "\n\n" + idleShape)

	var user strings.Builder
	user.WriteString(renderWorld(s, k))
	if len(day.Moments) > 0 {
		user.WriteString("\n\nToday so far:")
		moments := day.Moments
		if len(moments) > idleMoments {
			moments = moments[len(moments)-idleMoments:]
		}
		for _, mo := range moments {
			user.WriteString("\n- " + renderMoment(mo, s.Now))
		}
	}
	if len(in.Due) > 0 {
		user.WriteString("\n\nWhat she meant to do that has come due:")
		for _, t := range in.Due {
			user.WriteString("\n- " + renderThread(t, s.Now))
		}
	}
	if len(in.People) > 0 {
		user.WriteString("\n\nPeople she could go to:")
		for _, p := range in.People {
			where := "not around"
			if p.Here {
				where = "around"
			}
			fmt.Fprintf(&user, "\n- %s (%s)", p.Name, where)
		}
	}
	if in.Walk != nil {
		fmt.Fprintf(&user, "\n\nShe passes through #%s. What has been said there since she last passed:\n", in.Walk.Channel)
		user.WriteString(renderTranscript(in.Walk.Lines, s.Now))
	}
	user.WriteString("\n\nWhat is on her mind?")
	return []ai.Message{
		{Role: ai.RoleSystem, Content: sys.String()},
		{Role: ai.RoleUser, Content: user.String()},
	}
}

// freshLife is the life items still current: advanced within LifeStale.
func freshLife(life []memory.LifeItem, now time.Time) []memory.LifeItem {
	var out []memory.LifeItem
	for _, l := range life {
		if now.Sub(l.Advanced) <= memory.LifeStale {
			out = append(out, l)
		}
	}
	return out
}

// freshWants are the wants still current: touched within WantStale.
func freshWants(wants []memory.Want, now time.Time) []memory.Want {
	var out []memory.Want
	for _, w := range wants {
		if now.Sub(w.Touched) <= memory.WantStale {
			out = append(out, w)
		}
	}
	return out
}

// idleMoments is how many of today's moments the idle mind is shown.
const idleMoments = 12

// renderLife is what has been going on in her days, or "".
func renderLife(life []memory.LifeItem) string {
	if len(life) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("What has been going on in her days:")
	for _, l := range life {
		b.WriteString("\n- " + oneLine(l.Text))
	}
	return b.String()
}

// renderWants is what she wants lately, with why, or "".
func renderWants(wants []memory.Want) string {
	if len(wants) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("What she wants lately:")
	for _, w := range wants {
		line := oneLine(w.Text)
		if w.Why != "" {
			line += " — " + oneLine(w.Why)
		}
		b.WriteString("\n- " + line)
	}
	return b.String()
}

// onMindFresh is how long what is on her mind stays in front of her: until
// the next tick, give or take.
const onMindFresh = 3 * time.Hour

// renderOnMind is what is on her mind, when it is recent, or "".
func renderOnMind(self memory.Self, now time.Time, whose string) string {
	if self.OnMind == "" || now.Sub(self.OnMindAt) > onMindFresh {
		return ""
	}
	return "On " + whose + " mind: " + oneLine(self.OnMind)
}
