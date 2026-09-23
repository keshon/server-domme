package mind

import (
	"fmt"
	"strings"
	"time"

	"github.com/keshon/server-domme/internal/memory"
)

// arcFresh is how long a conversation's arc stays in front of her after it
// was last touched. Long enough to carry an afternoon with a few hours' gap
// in it — the case it exists for — and short enough that tomorrow's first
// message starts a conversation of its own.
const arcFresh = 12 * time.Hour

// maxArcChars bounds an arc. Two to four sentences: the shape of a
// conversation, not its minutes.
const maxArcChars = 400

// freshArc is the channel's arc, if there is one still under way.
func (m *Mind) freshArc(s Scene) (*memory.Arc, error) {
	if s.ChannelID == "" {
		return nil, nil
	}
	a, ok, err := m.Memory.Arc(s.GuildID, s.ChannelID)
	if err != nil || !ok || s.Now.Sub(a.Updated) > arcFresh {
		return nil, err
	}
	return &a, nil
}

// renderArc is the arc as the thinking prompt shows it, or "".
func renderArc(a *memory.Arc, now time.Time) string {
	if a == nil || a.Text == "" {
		return ""
	}
	return fmt.Sprintf("How the conversation here has gone, as she saw it (since %s, last touched %s):\n%s",
		when(a.Started, now), ago(now.Sub(a.Updated)), oneLine(a.Text))
}

// absorbArc writes what she made of the conversation so far. An arc gone
// stale is closed into a moment first, so a new conversation does not
// inherit the last one's start, and one that was not rewritten is closed
// once it goes stale all the same.
func (m *Mind) absorbArc(s Scene, a Appraisal, source memory.Source) error {
	if s.ChannelID == "" {
		return nil
	}
	old, ok, err := m.Memory.Arc(s.GuildID, s.ChannelID)
	if err != nil {
		return err
	}
	stale := ok && s.Now.Sub(old.Updated) > arcFresh
	if stale {
		if _, err := m.Memory.CloseArc(s.GuildID, s.ChannelID); err != nil {
			return err
		}
	}
	text := clip(oneLine(a.Arc), maxArcChars)
	if text == "" {
		return nil
	}
	arc := memory.Arc{
		ChannelID: s.ChannelID, Channel: s.ChannelName, Started: s.Now, Updated: s.Now,
		Text: text, Source: source,
	}
	if ok && !stale {
		arc.Started, arc.People = old.Started, old.People
	}
	arc.People = withRefs(arc.People, m.refs(s))
	return m.Memory.SetArc(s.GuildID, arc)
}

// withRefs adds the people not already there, by id or, without one, by name.
func withRefs(have, add []memory.Ref) []memory.Ref {
	out := append([]memory.Ref(nil), have...)
	for _, r := range add {
		seen := false
		for _, h := range out {
			if (r.ID != "" && h.ID == r.ID) || (r.ID == "" && strings.EqualFold(h.Name, r.Name)) {
				seen = true
				break
			}
		}
		if !seen {
			out = append(out, r)
		}
	}
	return out
}
