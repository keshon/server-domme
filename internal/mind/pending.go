package mind

import (
	"sync"
	"time"

	"github.com/keshon/server-domme/internal/memory"
)

// Deferral timing.
const (
	// DeferralTTL is how long an unanswered approach stays worth answering.
	// Past it the conversation has moved on, and a late answer is worse than
	// none: it arrives as a non sequitur about something nobody remembers
	// saying.
	DeferralTTL = 15 * time.Minute
	// DeferralRetry is the gap between attempts. The backend pool already
	// holds its own per-backend cooldowns, so a retry that arrives too early
	// costs a map lookup rather than a request.
	DeferralRetry = 30 * time.Second
	// MaxDeferralAttempts caps how many times one approach is retried.
	//
	// The TTL alone is not enough. When every backend is refusing, a held
	// approach comes round every DeferralRetry for the whole TTL, and each
	// pass shows a typing indicator in the channel — so a single unanswerable
	// message had the bot appearing to type, on and off, for a quarter of an
	// hour. Observed in production against a relay answering 402.
	MaxDeferralAttempts = 3
	// maxDeferredChannels bounds the map, the same way Conversations is
	// bounded.
	maxDeferredChannels = 256
)

// Deferred is an approach she meant to answer and could not.
//
// This exists because the backends are donated public relays that fail
// routinely, and the honest response to "I cannot reach a backend" is not a
// machine apology in the channel. Someone who was busy answers later; only
// software announces its own unavailability. The deferral is also what keeps
// failing distinct from choosing not to answer — an appraisal that chose
// to ignore never lands here.
type Deferred struct {
	GuildID   string
	ChannelID string
	// MessageID anchors the eventual reply to what it answers. Sent as a
	// Discord reply, a late answer reads as "getting back to you" instead of
	// as an interruption, which is what makes the delay work at all.
	MessageID string
	UserID    string
	Username  string
	Content   string
	Trigger   Trigger
	FormedAt  time.Time
	// Considered is what she made of it, once she has. An answer held
	// because the voice call failed keeps its appraisal, so the retry only
	// speaks: considering it again would write the same notes to memory
	// twice and might decide differently the second time.
	Considered *Appraisal
	// Thread is what she meant to follow up on, for TriggerSight.
	Thread *memory.Thread
	// Journal is the caller's record of this approach, carried so a late
	// answer completes the same entry the decision opened.
	Journal  uint64
	Attempts int
	nextTry  time.Time
}

// Age reports how long the approach has been waiting.
func (d Deferred) Age(now time.Time) time.Duration { return now.Sub(d.FormedAt) }

// Deferrals holds approaches waiting for a backend, at most one per channel.
//
// One, not a queue: several answers arriving together the moment a relay
// recovers is a burst of catch-up chatter, which reads far more like a machine
// than the silence it is trying to make up for. A newer approach in the same
// channel replaces the older one, because it is what the person is waiting on
// now.
type Deferrals struct {
	mu        sync.Mutex
	byChannel map[string]Deferred
}

// NewDeferrals returns an empty set.
func NewDeferrals() *Deferrals {
	return &Deferrals{byChannel: make(map[string]Deferred)}
}

// Hold records an approach to answer once a backend is reachable, reporting
// whether it was kept. It is refused once the approach has been tried
// MaxDeferralAttempts times or has outlived DeferralTTL: at that point silence
// is the honest outcome, and continuing to try is visible in the channel.
func (d *Deferrals) Hold(item Deferred, now time.Time) bool {
	if item.ChannelID == "" {
		return false
	}
	if item.Attempts >= MaxDeferralAttempts {
		return false
	}
	if !item.FormedAt.IsZero() && now.Sub(item.FormedAt) >= DeferralTTL {
		return false
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if _, known := d.byChannel[item.ChannelID]; !known && len(d.byChannel) >= maxDeferredChannels {
		d.evictOldestLocked()
	}

	if item.FormedAt.IsZero() {
		item.FormedAt = now
	}
	item.Attempts++
	item.nextTry = now.Add(DeferralRetry)
	d.byChannel[item.ChannelID] = item
	return true
}

// Due returns the approaches worth retrying now, removing them from the set.
// Expired ones are dropped rather than returned: the caller should never be
// handed something it is not supposed to answer.
func (d *Deferrals) Due(now time.Time) []Deferred {
	d.mu.Lock()
	defer d.mu.Unlock()

	var due []Deferred
	for id, item := range d.byChannel {
		if now.Sub(item.FormedAt) >= DeferralTTL {
			delete(d.byChannel, id)
			continue
		}
		if item.nextTry.After(now) {
			continue
		}
		due = append(due, item)
		delete(d.byChannel, id)
	}
	return due
}

// Drop forgets a channel's pending approach, for when it has been answered or
// overtaken.
func (d *Deferrals) Drop(channelID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.byChannel, channelID)
}

// Len reports how many approaches are waiting.
func (d *Deferrals) Len() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.byChannel)
}

// evictOldestLocked drops the longest-waiting entry. Callers hold d.mu.
func (d *Deferrals) evictOldestLocked() {
	var oldestID string
	var oldestAt time.Time
	for id, item := range d.byChannel {
		if oldestID == "" || item.FormedAt.Before(oldestAt) {
			oldestID, oldestAt = id, item.FormedAt
		}
	}
	if oldestID != "" {
		delete(d.byChannel, oldestID)
	}
}
