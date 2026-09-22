package mind

import (
	"sort"
	"time"

	"github.com/keshon/server-domme/internal/memory"
)

// Associative drift. Recall by relevance always brings back the right
// memory, which no one's memory does. Now and then the weakest of the
// recalled slots goes instead to a neighbour: a moment that shares a person
// or a word with this one but ranked too low to come back, or something that
// hit hard in the last week. It is not marked. It is where "that reminds
// me" comes from, and where a tangent comes from.
//
// A cheap first step, not a model of memory: whether anyone notices it, and
// whether it produces useful associations more often than made-up links,
// is what the drift metric measures. See docs/persona-v3.md, D.

const (
	// driftHeavy is the weight of a moment that can come back on its own,
	// related or not, if it happened within driftRecent.
	driftHeavy  = 0.7
	driftRecent = 7 * 24 * time.Hour
)

// drift picks what she recalls from a scored pool: the best recallMoments by
// relevance, with the weakest sometimes swapped for a neighbour. Returned
// oldest first, as recall is shown.
func (m *Mind) drift(s Scene, pool []memory.Moment, people []string) []memory.Moment {
	byScore := append([]memory.Moment(nil), pool...)
	sort.SliceStable(byScore, func(i, j int) bool { return byScore[i].Score > byScore[j].Score })

	shown := byScore
	if len(shown) > recallMoments {
		shown = append([]memory.Moment(nil), byScore[:recallMoments]...)
	}
	if m.Drift > 0 && len(byScore) > recallMoments && m.roll() < m.Drift {
		if n := neighbours(s, byScore[recallMoments:], people); len(n) > 0 {
			pick := n[int(m.roll()*float64(len(n)))]
			shown[len(shown)-1] = pick
			m.Log.Debug().Str("guild_id", s.GuildID).Str("moment", clip(pick.Text, 80)).Msg("mind_recall_drifted")
		}
	}
	sort.SliceStable(shown, func(i, j int) bool { return shown[i].At.Before(shown[j].At) })
	return shown
}

// neighbours are the moments below the cut that are loosely related: they
// share a person or a word with the scene, or hit hard recently.
func neighbours(s Scene, below []memory.Moment, people []string) []memory.Moment {
	present := wordSet(people)
	words := wordSet(topicWords(s.Turns))
	var out []memory.Moment
	for _, mo := range below {
		related := false
		for _, p := range mo.People {
			if present[p.ID] {
				related = true
				break
			}
		}
		if !related && overlapCount(memory.Keywords(mo.Text), words) > 0 {
			related = true
		}
		if !related && mo.Weight >= driftHeavy && s.Now.Sub(mo.At) < driftRecent {
			related = true
		}
		if related {
			out = append(out, mo)
		}
	}
	return out
}
