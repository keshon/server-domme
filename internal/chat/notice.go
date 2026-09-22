package chat

import (
	"time"

	"github.com/keshon/server-domme/internal/memory"
)

// What gets her attention in a room she reads, besides being spoken to:
// someone she meant to follow up with turning up (H2), and a remark that
// touches something of hers (H3). Both decide only what she notices. What it
// means, and whether she says anything, is still the appraisal's. See
// docs/persona-v3.md, workstream H.

const (
	// noticeRefresh is how often the cached threads and interests are read
	// again from her memory. This runs on the gateway goroutine, so it reads
	// the files at most this often per guild.
	noticeRefresh = 2 * time.Minute
	// sightRetry is how long before the same follow-up is considered again
	// after she let the moment pass: someone who chats all afternoon is not
	// asked about their interview on every message.
	sightRetry = 3 * time.Hour
	// knownWithin is how recently she must have talked with someone for
	// what they say to get her attention by itself.
	knownWithin = 7 * 24 * time.Hour
)

// noticeCache is what notice reads from her memory, per guild.
type noticeCache struct {
	at time.Time
	// threads are her open intentions.
	threads []memory.Thread
	// words are what she cares about: the words of her specifics, of what
	// she has said about herself, and of what she means to do.
	words map[string]bool
}

func (s *Service) noticeFor(guildID string, now time.Time) *noticeCache {
	s.noticeMu.Lock()
	defer s.noticeMu.Unlock()
	if c := s.noticed[guildID]; c != nil && now.Sub(c.at) < noticeRefresh {
		return c
	}
	c := &noticeCache{at: now, words: make(map[string]bool)}
	add := func(text string) {
		for _, w := range memory.Keywords(text) {
			c.words[w] = true
		}
	}
	if threads, err := s.memory.Threads(guildID); err == nil {
		c.threads = memory.Unfinished(threads)
	}
	for _, t := range c.threads {
		add(t.Text)
	}
	if s.character != nil {
		for _, sp := range s.character.Specifics {
			add(sp)
		}
	}
	if me, err := s.memory.Me(guildID); err == nil {
		for _, f := range me.Facts {
			add(f.Text)
		}
	}
	// What she saw on her walks lately: a remark about the dragon sketch
	// gets her attention because she saw it.
	if day, err := s.memory.Day(guildID, now); err == nil {
		for _, mo := range day.Moments {
			if mo.Walk {
				add(mo.Text)
			}
		}
	}
	s.noticed[guildID] = c
	return c
}

// dueFollowUp is an intention about someone who has just spoken, if one has
// come due and she may act on it now. The intention comes due when the
// person turns up, not on a timer while they are away — and without a tag
// or a consent question, since she is joining a room they are in.
func (s *Service) dueFollowUp(guildID, userID string, now time.Time) *memory.Thread {
	if !s.followUpOnSight || userID == "" {
		return nil
	}
	local := now.In(s.location)
	if !s.awakeToStart(local) || !s.mayStart(guildID, local) {
		return nil
	}
	c := s.noticeFor(guildID, now)
	s.noticeMu.Lock()
	defer s.noticeMu.Unlock()
	for _, t := range c.threads {
		if t.Person.ID != userID || (!t.Due.IsZero() && t.Due.After(now)) {
			continue
		}
		key := guildID + "|" + t.Key()
		if now.Sub(s.sightTried[key]) < sightRetry {
			continue
		}
		s.sightTried[key] = now
		th := t
		return &th
	}
	return nil
}

// mayReactFirst decides whether a remark in a room where she only answers
// gets her attention enough to react to: with ReactFirst on, rested, and
// only what touches something of hers. Never in a room she only reads.
func (s *Service) mayReactFirst(guildID, channelID, authorID, content string, now time.Time) bool {
	if !s.reactFirst || s.store.IsChatProactive(guildID, channelID) || s.battery() <= batteryFull {
		return false
	}
	s.overheardMu.Lock()
	recent := now.Sub(s.overheard[channelID]) < overhearEvery
	s.overheardMu.Unlock()
	if recent || !s.interesting(guildID, authorID, content, now) {
		return false
	}
	if !s.mayStart(guildID, now.In(s.location)) {
		return false
	}
	s.overheardMu.Lock()
	s.overheard[channelID] = now
	s.overheardMu.Unlock()
	return true
}

// interesting reports whether an overheard remark touches something of
// hers: a word from her specifics, from what she has said about herself or
// from what she means to do, or someone she has been talking with lately.
//
// Keywords are acceptable here where v1's word lists were not because they
// decide only what she notices. The day this decides what she does, it is
// v1 again.
func (s *Service) interesting(guildID, authorID, content string, now time.Time) bool {
	c := s.noticeFor(guildID, now)
	for _, w := range memory.Keywords(content) {
		if c.words[w] {
			return true
		}
	}
	if p := s.store.GetMindPerson(guildID, authorID); p != nil && !p.LastExchangeAt.IsZero() && now.Sub(p.LastExchangeAt) < knownWithin {
		return true
	}
	return false
}
