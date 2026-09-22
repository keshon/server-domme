package mind

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/keshon/server-domme/internal/ai"
	"github.com/keshon/server-domme/internal/memory"
)

// Life and wants: the third reflection call, in its own call so a failure
// costs this part of a day and nothing else. Her life is advanced only from
// what actually happened — each item cites moments she observed that day,
// never what she said about herself and never earlier life, which is the
// loop by which something she once made up becomes her history. Wants are
// her interpretation, set and retired here. See docs/persona-v3.md, E2, F3
// and I.

const lifeRules = `How to look back:
- Her life is what has been going on in her days on the server as she sees it: something she keeps noticing, something that has been annoying her, something she is in the middle of. At most four things.
- Every new or changed item must rest on the moments listed below, by number. An item that did not come up again today can be kept as it is, by its number; it cannot be changed without a moment behind it.
- Nothing she only said about herself counts as her life, and nothing is invented.
- Wants are what she wants lately, each with why: at most three. Keep one that still holds, by its number, and say whether it came up today. Let go of one that no longer holds by leaving it out.
- Most days change little. Keeping everything as it is is a fine answer.`

const lifeShape = `Answer with one JSON object and nothing else:
{
  "life": [
    {"text": "the thing, one line", "moments": [numbers of today's moments it rests on], "keeps": number of an existing item it continues, or 0}
  ],
  "wants": [
    {"text": "what she wants", "why": "why, in a few words", "keeps": number of an existing want, or 0, "came_up": true if it came up today}
  ]
}`

// ReflectLife advances her life and her wants from a day. It reports false
// when there was nothing in the day to go on.
func (m *Mind) ReflectLife(ctx context.Context, guildID string, date, now time.Time) (bool, error) {
	day, err := m.Memory.Day(guildID, date)
	if err != nil {
		return false, err
	}
	// Only what happened around her: not her interpretations, and not her
	// own words. See observedPart.
	var observed []memory.Moment
	for _, mo := range day.Moments {
		if _, ok := observedPart(mo); ok && mo.Kind != memory.Interpreted {
			observed = append(observed, mo)
		}
	}
	self, err := m.Memory.Self(guildID)
	if err != nil {
		return false, err
	}
	if len(observed) == 0 {
		return false, m.Memory.UpdateSelf(guildID, func(me *memory.Self) { me.LifeThrough = startOf(date) })
	}
	msgs := m.lifePrompt(self, observed)
	reply, backend, err := m.generate(ai.WithRaw(ai.WithTemperature(ctx, reflectTemperature)), msgs)
	if err != nil {
		return false, err
	}
	obj, ok := decodeObject(reply)
	if !ok {
		return false, fmt.Errorf("%w: %q", ErrUnreadable, clip(reply, 200))
	}
	return true, m.applyLife(guildID, backend, date, now, obj, self, observed)
}

func (m *Mind) lifePrompt(self memory.Self, observed []memory.Moment) []ai.Message {
	name := "her"
	if m.Character != nil {
		name = m.Character.Name
	}
	var sys strings.Builder
	fmt.Fprintf(&sys, "You are the inner life of %s, looking back on yesterday: what has been going on in her days, and what she wants.", name)
	if m.Character != nil && len(m.Character.Specifics) > 0 {
		sys.WriteString("\n\n" + renderSpecifics("Who she is, specifically:", m.Character.Specifics))
	}
	sys.WriteString("\n\n" + lifeRules + "\n\n" + lifeShape)

	var user strings.Builder
	if len(self.Life) > 0 {
		user.WriteString("What has been going on in her days until now:")
		for i, l := range self.Life {
			fmt.Fprintf(&user, "\n%d. %s", i+1, oneLine(l.Text))
		}
		user.WriteString("\n\n")
	}
	if len(self.Wants) > 0 {
		user.WriteString("What she has wanted lately:")
		for i, w := range self.Wants {
			fmt.Fprintf(&user, "\n%d. %s — %s", i+1, oneLine(w.Text), oneLine(w.Why))
		}
		user.WriteString("\n\n")
	}
	user.WriteString("What happened yesterday:")
	for i, mo := range observed {
		text, _ := observedPart(mo)
		fmt.Fprintf(&user, "\n%d. [%s] %s", i+1, mo.At.Format("15:04"), oneLine(clip(text, maxMomentChars)))
	}
	user.WriteString("\n\nWhat has been going on in her days now, and what does she want?")
	return []ai.Message{
		{Role: ai.RoleSystem, Content: sys.String()},
		{Role: ai.RoleUser, Content: user.String()},
	}
}

// momentRef names a moment in a life item's sources.
func momentRef(mo memory.Moment) string { return mo.At.Format("2006-01-02 15:04") }

// applyLife commits her life and wants. A life item must cite today's
// moments or continue an existing item unchanged; anything else is refused.
func (m *Mind) applyLife(guildID, backend string, date, now time.Time, obj map[string]any, self memory.Self, observed []memory.Moment) error {
	refuse := func(kind, reason string) { m.refuse(guildID, backend, kind, reason) }
	day := startOf(date)

	var life []memory.LifeItem
	for _, entry := range objects(obj, "life") {
		var cites []string
		for _, n := range numbers(entry, "moments") {
			if n >= 1 && n <= len(observed) {
				cites = append(cites, momentRef(observed[n-1]))
			} else {
				refuse("life", "cites a moment that does not exist")
			}
		}
		keeps := int(num(entry, "keeps"))
		var old *memory.LifeItem
		if keeps >= 1 && keeps <= len(self.Life) {
			old = &self.Life[keeps-1]
		} else if keeps != 0 {
			refuse("life", "continues an item that does not exist")
		}
		text := clip(str(entry, "text"), maxLaterChars)
		switch {
		case len(cites) > 0 && text != "":
			item := memory.LifeItem{Text: text, Since: day, Advanced: day, Sources: cites}
			if old != nil {
				item.Since = old.Since
				item.Sources = append(append([]string(nil), old.Sources...), cites...)
			}
			life = append(life, item)
		case old != nil:
			// Nothing new behind it: kept exactly as it was. Changing it
			// without a moment would be deriving life from earlier life.
			life = append(life, *old)
		default:
			refuse("life", "rests on nothing that happened")
		}
	}

	var wants []memory.Want
	for _, entry := range objects(obj, "wants") {
		text, why := clip(str(entry, "text"), maxLaterChars), clip(str(entry, "why"), maxLaterChars)
		keeps := int(num(entry, "keeps"))
		cameUp := strings.EqualFold(str(entry, "came_up"), "true")
		switch {
		case keeps >= 1 && keeps <= len(self.Wants):
			w := self.Wants[keeps-1]
			if cameUp {
				w.Touched = day
			}
			wants = append(wants, w)
		case keeps != 0:
			refuse("want", "continues a want that does not exist")
		case text != "":
			wants = append(wants, memory.Want{Text: text, Why: why, Since: day, Touched: day})
		}
	}

	return m.Memory.UpdateSelf(guildID, func(me *memory.Self) {
		me.Life = freshLife(life, now)
		me.Wants = freshWants(wants, now)
		me.LifeThrough = day
	})
}
