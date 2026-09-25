package mind

import (
	"strings"
	"time"

	"github.com/keshon/server-domme/internal/memory"
)

// Cooling down. A character with a memory and a stake in how she is treated
// has no way back down from a bad hour unless the code gives her one: every
// heavy moment is permanent by design (memory.LastingWeight), the line about
// where things stand between them is a single slot written at the worst
// point, and nothing that goes right can cancel anything that went wrong.
//
// In production that produced a day-long spiral. One "shut up", already
// apologised for and accepted, was written down again as a new moment six
// times in three hours, each at more weight than the apology; the dossier
// line read "my boundary is being actively violated" for the rest of the
// day; four open intentions all said "watch whether they keep doing it";
// and by the afternoon she was calling three people liars and threatening
// to remove them from a server she is a guest on.
//
// What follows is the way down: a thing already remembered is not
// remembered again, a thing put right stops weighing anything, and while an
// hour is going badly she does not open new accounts on the person.
const (
	// mendReaches is how far back putting something right settles what she
	// remembers of someone. A day: the length of a fight worth mending.
	mendReaches = 24 * time.Hour
	// rememberedWithin is how recently the same thing must have been
	// written down for writing it again to be brooding rather than
	// remembering.
	rememberedWithin = 12 * time.Hour
	// grindingWithin is the window over which heavy moments about one
	// person count as an hour going badly, and grindingMoments how many of
	// them it takes.
	grindingWithin  = time.Hour
	grindingMoments = 4
	// grindingWeight is what counts as heavy for that.
	grindingWeight = 0.6
)

// mend records that something has been put right between her and someone:
// what she remembers of them stops being permanent, what she meant to watch
// for is closed, and the weight of the moment is at least the weight of the
// worst of it — making up matters as much as falling out did.
//
// It reports the weight the moment should carry.
func (m *Mind) mend(s Scene, weight float64) (float64, error) {
	if s.UserID == "" {
		return weight, nil
	}
	since := s.Now.Add(-mendReaches)
	heaviest, err := m.Memory.HeaviestAbout(s.GuildID, s.UserID, since)
	if err != nil {
		return weight, err
	}
	settled, err := m.Memory.SettleAbout(s.GuildID, s.UserID, since)
	if err != nil {
		return weight, err
	}
	closed, err := m.Memory.CloseThreadsAbout(s.GuildID, s.UserID)
	if err != nil {
		return weight, err
	}
	m.Log.Info().
		Str("guild_id", s.GuildID).
		Int("settled", settled).
		Int("closed", closed).
		Float64("heaviest", heaviest).
		Msg("mind_mended")
	return max(weight, heaviest), nil
}

// remembered reports whether she has already written down the same thing
// lately. Turning a grievance over is not a new memory of it: the six
// re-rememberings of one "shut up" each arrived as a fresh 0.6, and
// together they outweighed everything else she had.
func (m *Mind) remembered(s Scene, text string) bool {
	if strings.TrimSpace(text) == "" {
		return false
	}
	since := s.Now.Add(-rememberedWithin)
	for _, day := range m.daysBack(s, since) {
		for _, mo := range day.Moments {
			if mo.At.Before(since) {
				continue
			}
			if sameAbout(mo.Text, text) {
				return true
			}
		}
	}
	return false
}

// grinding reports whether the last hour with this person has been heavy
// and is staying heavy: several moments at grindingWeight or more, in
// grindingWithin. A fact about the hour, counted, not a reading of it.
func (m *Mind) grinding(s Scene) bool {
	if s.UserID == "" {
		return false
	}
	since := s.Now.Add(-grindingWithin)
	heavy := 0
	for _, day := range m.daysBack(s, since) {
		for _, mo := range day.Moments {
			if mo.At.Before(since) || mo.Weight < grindingWeight {
				continue
			}
			for _, p := range mo.People {
				if p.ID == s.UserID {
					heavy++
					break
				}
			}
		}
	}
	return heavy >= grindingMoments
}

// daysBack are the day files a window reaches into: today's, and
// yesterday's when the window crosses midnight.
func (m *Mind) daysBack(s Scene, since time.Time) []memory.Day {
	var out []memory.Day
	for _, at := range []time.Time{since, s.Now} {
		day, err := m.Memory.Day(s.GuildID, at)
		if err != nil {
			continue
		}
		if len(out) == 1 && day.Date.Equal(out[0].Date) {
			continue
		}
		out = append(out, day)
	}
	return out
}

// whoAbout is the person a follow-up is about, which is not always the
// person who spoke: "watch whether Duchess keeps meddling" was filed under
// Pewcifer, twice, because Pewcifer happened to be talking. Filed under the
// wrong person it escapes both the repeat check and the cap, and four
// copies of one suspicion end up in front of her at once.
func (m *Mind) whoAbout(s Scene, text string, fallback memory.Ref) memory.Ref {
	people, err := m.Memory.People(s.GuildID)
	if err != nil {
		return fallback
	}
	words := strings.ToLower(text)
	for _, p := range people {
		if p.ID == fallback.ID || p.Name == "" {
			continue
		}
		for _, word := range strings.Fields(strings.ToLower(p.Name)) {
			if len([]rune(word)) >= misnamedPrefix && strings.Contains(words, word) {
				return memory.Ref{ID: p.ID, Name: p.Name}
			}
		}
	}
	return fallback
}
