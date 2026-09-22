package mind

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/keshon/server-domme/internal/ai"
	"github.com/keshon/server-domme/internal/memory"
)

// Self-facts: what she says about herself becomes true of her. Read out of
// a day's messages at reflection, in a call of its own so a failure costs
// this part of the day and nothing else. See docs/persona-v3.md, F2.

// selfFactRules are how her own words are read. Each rule answers a way this
// goes wrong: jokes and play hardening into who she is, other people's words
// or her intentions passing for things she said, one remark noted twice.
const selfFactRules = `How to read them:
- Only what she said about HERSELF, sincerely: an opinion she holds, a taste, a habit, something about her past, something she is bad or good at.
- Skip anything said in jest, sarcasm, bluff, teasing, or in play — a roleplay scene, a game, a bit. "I'm basically a toaster" is a joke, not a fact.
- Skip anything about other people, and anything she was only asked or told.
- Skip what she already said before, unless she now says otherwise.
- Each fact one short line in the third person, the way it would read on a card about her: "hates Rust — the compiler lectures her".
- Usually there is nothing new. An empty list is the common answer.`

const selfFactShape = `Answer with one JSON object and nothing else:
{
  "facts": [
    {"text": "the fact, one short line", "line": the number of her line it comes from, "replaces": the number of an earlier fact it overturns or 0, "contradicts": the number of a specific it contradicts or 0}
  ]
}`

// ReflectSelf reads what she said on a day for facts about herself. It
// reports false when there was nothing of hers to read.
func (m *Mind) ReflectSelf(ctx context.Context, guildID string, date time.Time) (bool, error) {
	day, err := m.Memory.Day(guildID, date)
	if err != nil {
		return false, err
	}
	var hers []memory.Moment
	for _, mo := range day.Moments {
		if mo.Said != "" {
			hers = append(hers, mo)
		}
	}
	if len(hers) == 0 {
		return false, m.Memory.UpdateMe(guildID, func(me *memory.Me) { me.Through = date })
	}
	me, err := m.Memory.Me(guildID)
	if err != nil {
		return false, err
	}
	var specifics []string
	if m.Character != nil {
		specifics = m.Character.Specifics
	}

	msgs := m.selfFactPrompt(hers, me.Facts, specifics)
	reply, backend, err := m.generate(ai.WithRaw(ai.WithTemperature(ctx, thinkingTemperature)), msgs)
	if err != nil {
		return false, err
	}
	obj, ok := decodeObject(reply)
	if !ok {
		return false, fmt.Errorf("%w: %q", ErrUnreadable, clip(reply, 200))
	}
	return true, m.applySelfFacts(guildID, backend, date, obj, hers, specifics)
}

func (m *Mind) selfFactPrompt(hers []memory.Moment, facts []memory.SelfFact, specifics []string) []ai.Message {
	name := "her"
	if m.Character != nil {
		name = m.Character.Name
	}
	var sys strings.Builder
	fmt.Fprintf(&sys, "You are reading back what %s said yesterday, to find what she said about herself that is now true of her.", name)
	sys.WriteString("\n\n" + selfFactRules + "\n\n" + selfFactShape)

	var user strings.Builder
	if len(specifics) > 0 {
		user.WriteString("Who she is, as her author wrote it:")
		for i, sp := range specifics {
			fmt.Fprintf(&user, "\n%d. %s", i+1, oneLine(sp))
		}
		user.WriteString("\n\n")
	}
	if len(facts) > 0 {
		user.WriteString("What she has said about herself before:")
		for i, f := range facts {
			fmt.Fprintf(&user, "\n%d. %s", i+1, oneLine(f.Text))
		}
		user.WriteString("\n\n")
	}
	user.WriteString("Her lines yesterday:")
	for i, mo := range hers {
		fmt.Fprintf(&user, "\n%d. [%s] %s", i+1, mo.At.Format("15:04"), herWords(mo.Text))
	}
	user.WriteString("\n\nWhat, if anything, did she say about herself that is now true of her?")
	return []ai.Message{
		{Role: ai.RoleSystem, Content: sys.String()},
		{Role: ai.RoleUser, Content: user.String()},
	}
}

// herWords is a moment of hers without what she meant or why: a self-fact
// comes from what she said, never from an intention. Said writes the
// intention after these markers, so cutting at them is exact.
func herWords(text string) string {
	for _, marker := range []string{" — meant: ", " — because "} {
		if i := strings.Index(text, marker); i >= 0 {
			text = text[:i]
		}
	}
	return oneLine(text)
}

// applySelfFacts commits the facts reflection proposed. A fact must cite one
// of her lines that day, which the code turns into its source; one that
// cites nothing real, or says what she has already said, is refused.
func (m *Mind) applySelfFacts(guildID, backend string, date time.Time, obj map[string]any, hers []memory.Moment, specifics []string) error {
	refuse := func(reason string) { m.refuse(guildID, backend, proposalSelfFact, reason) }
	day := startOf(date)
	return m.Memory.UpdateMe(guildID, func(me *memory.Me) {
		for _, entry := range objects(obj, "facts") {
			text := clip(str(entry, "text"), maxSelfFactChar)
			if text == "" {
				continue
			}
			line := int(num(entry, "line"))
			if line < 1 || line > len(hers) {
				refuse("cites no line of hers")
				continue
			}
			f := memory.SelfFact{Day: day, Text: text, Source: memory.Message(memory.Stated, hers[line-1].Said)}

			if n := int(num(entry, "replaces")); n != 0 {
				if n < 1 || n > len(me.Facts) {
					refuse("replaces a fact that does not exist")
				} else {
					old := me.Facts[n-1]
					old.Superseded = day
					me.Superseded = append(me.Superseded, old)
					me.Facts = append(me.Facts[:n-1], me.Facts[n:]...)
				}
			}
			if hasFact(me.Facts, text) {
				refuse("already said")
				continue
			}
			if n := int(num(entry, "contradicts")); n != 0 {
				if n < 1 || n > len(specifics) {
					refuse("contradicts a specific that does not exist")
				} else {
					// The card stays canon; the fact stays as history and
					// waits for the author. See memory.SelfFact.Conflict.
					f.Conflict = oneLine(specifics[n-1])
				}
			}
			me.Facts = append(me.Facts, f)
		}
		me.Through = day
	})
}

// hasFact reports whether a fact says what a current one already does.
func hasFact(facts []memory.SelfFact, text string) bool {
	words := strings.Fields(echoKey(text))
	for _, f := range facts {
		if overlap(words, strings.Fields(echoKey(f.Text))) >= noteOverlap {
			return true
		}
	}
	return false
}

func startOf(t time.Time) time.Time {
	y, mo, d := t.Date()
	return time.Date(y, mo, d, 0, 0, 0, 0, t.Location())
}
