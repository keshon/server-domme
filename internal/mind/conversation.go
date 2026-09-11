package mind

import (
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
	// TurnStaleAfter is how long a turn stays part of "the conversation".
	// Older messages are still in the channel, but treating a thread from
	// yesterday as live context is what makes a bot reply to the wrong thing.
	TurnStaleAfter = 30 * time.Minute
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
}

// Conversations holds recent messages per channel.
//
// This is deliberately in memory and deliberately lost on restart. Channel
// history is Discord's job and it already has it; what this holds is the
// working context of a conversation in progress, which stops being true the
// moment the process is down long enough for people to move on.
type Conversations struct {
	mu       sync.Mutex
	byChanID map[string][]Turn
	// touched records last write per channel, used to evict the coldest
	// channel when maxChannels is reached.
	touched map[string]time.Time
}

// NewConversations returns an empty conversation store.
func NewConversations() *Conversations {
	return &Conversations{
		byChanID: make(map[string][]Turn),
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
// Turns older than TurnStaleAfter relative to the newest one are dropped: a
// gap that long means the conversation ended, and replying into yesterday's
// thread as though it were still running is the most visible way a chat bot
// gets it wrong.
func (c *Conversations) Recent(channelID string) []Turn {
	c.mu.Lock()
	defer c.mu.Unlock()

	turns := c.byChanID[channelID]
	if len(turns) == 0 {
		return nil
	}

	newest := turns[len(turns)-1].At
	cutoff := newest.Add(-TurnStaleAfter)

	start := 0
	for i := len(turns) - 1; i >= 0; i-- {
		if turns[i].At.Before(cutoff) {
			start = i + 1
			break
		}
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
	}
}
