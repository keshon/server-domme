package mind

import (
	"sort"
	"sync"
	"time"
)

// Conversation limits.
const (
	// maxTurnsPerChannel bounds the in-memory history per channel. It is a
	// turn count rather than a character count because the prompt budget
	// trims again by size later; this only stops one busy channel growing
	// without limit.
	maxTurnsPerChannel = 40
	// TurnStaleAfter is how long a turn stays part of "the conversation" in a
	// channel busy enough for that to mean something.
	TurnStaleAfter = 30 * time.Minute
	// MinLiveTurns is how many of the most recent turns are kept whatever
	// their age.
	//
	// A time window alone describes a busy channel and destroys a quiet one:
	// three messages across two days are still one conversation, and cutting
	// at thirty minutes leaves her answering a single line with no idea what
	// it is about. The floor is what makes the two cases the same code.
	//
	// This would have been a bad idea when TurnStaleAfter was written, because
	// the transcript then carried no time at all and every line read as
	// equally recent. labelled() now stamps the age of anything older than
	// StaleTurnAge into the line itself, so an old turn arrives visibly old
	// and can be treated as such.
	MinLiveTurns = 8
	// MaxTurnAge is the absolute horizon. Past it a turn is not context at any
	// count: a channel with four messages in a month should not open with
	// something from before anyone remembers. What is older than this belongs
	// to the memory layer, which renders it as a memory rather than as
	// something just said.
	MaxTurnAge = 24 * time.Hour
	// maxChannels caps how many channels are tracked at once, so a bot in
	// many busy guilds cannot grow this map without bound.
	maxChannels = 512
)

// Turn is one message the bot saw, or one it sent.
type Turn struct {
	UserID   string
	Username string
	Content  string
	At       time.Time
	// MessageID is Discord's id for this message, recorded for her own turns
	// so a later reply pointing at one can be recognised as a reply to her.
	//
	// Needed because discordgo documents ReferencedMessage as best-effort —
	// "the backend did not attempt to fetch the message that was being replied
	// to" — while MessageReference.MessageID is always present. Matching ids
	// is the only reliable way to know a reply was aimed at her.
	MessageID string
	// FromBot marks the bot's own messages, which are replayed as assistant
	// turns so it can see what it already said and not repeat itself.
	FromBot bool
	// Mentioned records that this message addressed the bot directly.
	Mentioned bool
	// To is who her own message was answering, for her turns only. It is
	// what lets a question she asked be recognised as ignored; see
	// BrushedOff.
	To string
}

// Conversations holds recent messages per channel.
//
// In memory, and empty on restart. Channel history is Discord's job and it
// already has it, so what is lost is re-readable rather than gone: Seed fills
// a channel from the real history the first time the bot needs it, which is
// what stops a redeploy costing her the thread everyone else can still scroll
// up and read.
type Conversations struct {
	mu       sync.Mutex
	byChanID map[string][]Turn
	// touched records last write per channel, used to evict the coldest
	// channel when maxChannels is reached.
	touched map[string]time.Time
	// seeded marks channels that have had a backfill attempted, successful or
	// not. Attempted rather than succeeded on purpose: a channel the bot
	// cannot read history in fails every time, and retrying on every message
	// would spend a REST call per reply to learn the same thing.
	seeded map[string]bool
}

// NewConversations returns an empty conversation store.
// NeedsSeed reports whether a channel has yet to be backfilled.
func (c *Conversations) NeedsSeed(channelID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.seeded[channelID]
}

// Seed merges history fetched from Discord into a channel, oldest first, and
// marks the channel as backfilled either way.
//
// Merging rather than replacing because the message that triggered the reply
// is already recorded by the time this runs, and it is also in the fetched
// history — deduplicating on message id is what stops it appearing twice, once
// as itself and once as its own echo.
func (c *Conversations) Seed(channelID string, turns []Turn) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.seeded[channelID] = true
	if len(turns) == 0 {
		return
	}

	existing := c.byChanID[channelID]
	seen := make(map[string]bool, len(existing))
	for _, t := range existing {
		if t.MessageID != "" {
			seen[t.MessageID] = true
		}
	}

	merged := make([]Turn, 0, len(turns)+len(existing))
	for _, t := range turns {
		if t.MessageID != "" && seen[t.MessageID] {
			continue
		}
		merged = append(merged, t)
	}
	merged = append(merged, existing...)

	// Fetched history is older than anything already recorded, but a message
	// can arrive while the fetch is in flight, so sort rather than assume.
	sort.SliceStable(merged, func(i, j int) bool { return merged[i].At.Before(merged[j].At) })

	if len(merged) > maxTurnsPerChannel {
		merged = merged[len(merged)-maxTurnsPerChannel:]
	}
	c.byChanID[channelID] = merged
	c.touched[channelID] = time.Now()
}

func NewConversations() *Conversations {
	return &Conversations{
		byChanID: make(map[string][]Turn),
		seeded:   make(map[string]bool),
		touched:  make(map[string]time.Time),
	}
}

// Record appends a turn to a channel's history.
func (c *Conversations) Record(channelID string, t Turn) {
	if channelID == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, known := c.byChanID[channelID]; !known && len(c.byChanID) >= maxChannels {
		c.evictColdestLocked()
	}

	turns := append(c.byChanID[channelID], t)
	if len(turns) > maxTurnsPerChannel {
		turns = turns[len(turns)-maxTurnsPerChannel:]
	}
	c.byChanID[channelID] = turns
	c.touched[channelID] = t.At
}

// Recent returns the live turns for a channel, oldest first.
//
// Two bounds, whichever gives more: everything within TurnStaleAfter of the
// newest turn, or the last MinLiveTurns whatever their age. A busy channel is
// decided by the window and a quiet one by the count, which is the only way
// one rule serves both — thirty minutes of a fast channel is a conversation,
// and thirty minutes of a slow one is a single line with nothing around it.
//
// Nothing past MaxTurnAge survives either way. Older than that it is not
// context, it is history, and mind.Memory is what renders history.
func (c *Conversations) Recent(channelID string) []Turn {
	c.mu.Lock()
	defer c.mu.Unlock()

	turns := c.byChanID[channelID]
	if len(turns) == 0 {
		return nil
	}

	newest := turns[len(turns)-1].At
	window := newest.Add(-TurnStaleAfter)
	horizon := newest.Add(-MaxTurnAge)

	start := len(turns)
	for i := len(turns) - 1; i >= 0; i-- {
		if turns[i].At.Before(horizon) {
			break
		}
		withinWindow := !turns[i].At.Before(window)
		withinCount := len(turns)-i <= MinLiveTurns
		if !withinWindow && !withinCount {
			break
		}
		start = i
	}

	live := make([]Turn, len(turns)-start)
	copy(live, turns[start:])
	return live
}

// Forget drops a channel's history, for when a conversation should not carry
// forward — a purge, or the bot being told to drop it.
func (c *Conversations) Forget(channelID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.byChanID, channelID)
	delete(c.touched, channelID)
	// The seed flag goes too. Forgetting a channel and then refusing to read
	// its history again would leave her permanently blank there, which is a
	// stronger amnesia than anything the caller asked for.
	delete(c.seeded, channelID)
}

// evictColdestLocked drops the least recently written channel. Callers hold
// c.mu.
func (c *Conversations) evictColdestLocked() {
	var coldest string
	var coldestAt time.Time
	for id, at := range c.touched {
		if coldest == "" || at.Before(coldestAt) {
			coldest, coldestAt = id, at
		}
	}
	if coldest != "" {
		delete(c.byChanID, coldest)
		delete(c.touched, coldest)
		// Evicting the history without the flag would bring the channel back
		// empty and never refill it.
		delete(c.seeded, coldest)
	}
}

// Channels lists the channels with a conversation in memory.
//
// For the memory writer, which has to find rooms worth summarising without
// being told about them: every channel it should consider is one that has
// already been talked in, and this is where that is known.
func (c *Conversations) Channels() []string {
	c.mu.Lock()
	defer c.mu.Unlock()

	out := make([]string, 0, len(c.byChanID))
	for id := range c.byChanID {
		out = append(out, id)
	}
	return out
}

// All returns a channel's turns regardless of age, oldest first.
//
// Recent drops anything past TurnStaleAfter because a stale turn is not live
// context. A memory is the opposite case: it is written precisely when the
// conversation has stopped being live, so it needs the turns Recent has
// already discarded.
func (c *Conversations) All(channelID string) []Turn {
	c.mu.Lock()
	defer c.mu.Unlock()

	turns := c.byChanID[channelID]
	out := make([]Turn, len(turns))
	copy(out, turns)
	return out
}

// NeedsAnchor reports whether a reply to messageID would be ambiguous without
// Discord's reply anchor — that is, whether anyone else has spoken since.
//
// Anchoring every reply would be unambiguous and wrong. In a quiet two-person
// exchange, formally quoting each line is the machine tell this design keeps
// removing: people use the reply affordance when the thread has moved on and
// simply talk when it is obvious who they mean. This is that rule, and it is
// the same question either way — has anything come between.
//
// A message no longer in the buffer counts as needing one. It has aged out,
// which means time has passed, which is exactly when an unanchored reply
// arrives with nothing around it to explain what it answers.
func NeedsAnchor(turns []Turn, messageID, userID string) bool {
	if messageID == "" {
		return false
	}

	var found bool
	for _, t := range turns {
		if t.MessageID == messageID {
			found = true
			continue
		}
		if !found {
			continue
		}
		// Her own lines do not confuse anyone about who she is answering, and
		// the same person carrying on is still the same conversation.
		if t.FromBot || t.UserID == "" || t.UserID == userID {
			continue
		}
		return true
	}
	return !found
}

// LastSession returns the trailing run of turns with no silence longer than
// gap between them: the latest conversation, as opposed to everything the
// buffer happens to hold.
func LastSession(turns []Turn, gap time.Duration) []Turn {
	if len(turns) == 0 {
		return nil
	}
	start := len(turns) - 1
	for start > 0 && turns[start].At.Sub(turns[start-1].At) <= gap {
		start--
	}
	return turns[start:]
}
