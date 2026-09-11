package mind

import (
	"sync"
	"testing"
	"time"
)

func held(channel string, at time.Time) Deferred {
	return Deferred{
		GuildID:   "g1",
		ChannelID: channel,
		MessageID: "m-" + channel,
		UserID:    "u1",
		Username:  "ann",
		Content:   "you around?",
		Trigger:   TriggerMention,
		FormedAt:  at,
	}
}

func TestDeferralsReturnsAnApproachOnceItsRetryIsDue(t *testing.T) {
	d := NewDeferrals()
	now := time.Now()

	d.Hold(held("c1", now), now)

	if got := d.Due(now); len(got) != 0 {
		t.Fatalf("Due returned %d immediately, want none before the retry gap", len(got))
	}

	later := now.Add(DeferralRetry + time.Second)
	got := d.Due(later)
	if len(got) != 1 {
		t.Fatalf("Due returned %d after the retry gap, want 1", len(got))
	}
	if got[0].MessageID != "m-c1" {
		t.Errorf("wrong approach returned: %+v", got[0])
	}
	if d.Len() != 0 {
		t.Error("Due did not remove what it handed out")
	}
}

// Past the TTL the conversation has moved on, and a late answer arrives as a
// non sequitur about something nobody remembers saying.
func TestDeferralsExpiresStaleApproaches(t *testing.T) {
	d := NewDeferrals()
	now := time.Now()

	d.Hold(held("c1", now), now)

	stale := now.Add(DeferralTTL + time.Minute)
	if got := d.Due(stale); len(got) != 0 {
		t.Fatalf("Due returned a stale approach: %+v", got)
	}
	if d.Len() != 0 {
		t.Error("expired approach was not dropped")
	}
}

// Several answers arriving together the moment a relay recovers is a burst of
// catch-up chatter, which reads more like a machine than the silence it is
// making up for.
func TestDeferralsKeepsOnlyTheNewestPerChannel(t *testing.T) {
	d := NewDeferrals()
	now := time.Now()

	first := held("c1", now)
	first.Content = "older question"
	d.Hold(first, now)

	second := held("c1", now.Add(time.Minute))
	second.Content = "newer question"
	second.MessageID = "m-newer"
	d.Hold(second, now.Add(time.Minute))

	if d.Len() != 1 {
		t.Fatalf("holding %d approaches for one channel, want 1", d.Len())
	}

	got := d.Due(now.Add(time.Minute + DeferralRetry + time.Second))
	if len(got) != 1 || got[0].Content != "newer question" {
		t.Errorf("kept the wrong approach: %+v", got)
	}
}

func TestDeferralsKeepsChannelsApart(t *testing.T) {
	d := NewDeferrals()
	now := time.Now()

	d.Hold(held("c1", now), now)
	d.Hold(held("c2", now), now)

	if d.Len() != 2 {
		t.Fatalf("holding %d approaches, want one per channel", d.Len())
	}
}

func TestDeferralsCountsAttempts(t *testing.T) {
	d := NewDeferrals()
	now := time.Now()

	d.Hold(held("c1", now), now)
	got := d.Due(now.Add(DeferralRetry + time.Second))
	if len(got) != 1 || got[0].Attempts != 1 {
		t.Fatalf("Attempts = %+v, want 1 after the first hold", got)
	}

	// Failing again re-holds the same approach, keeping its original age.
	again := got[0]
	d.Hold(again, now.Add(time.Minute))
	got = d.Due(now.Add(time.Minute + DeferralRetry + time.Second))
	if len(got) != 1 || got[0].Attempts != 2 {
		t.Fatalf("Attempts = %+v, want 2 after the second hold", got)
	}
	if !got[0].FormedAt.Equal(now) {
		t.Errorf("FormedAt moved to %v, want the original %v — the TTL would never expire",
			got[0].FormedAt, now)
	}
}

func TestDeferredAgeMeasuresFromWhenItWasSaid(t *testing.T) {
	now := time.Now()
	item := held("c1", now.Add(-5*time.Minute))
	if got := item.Age(now); got != 5*time.Minute {
		t.Errorf("Age = %v, want 5m", got)
	}
}

func TestDeferralsDropForgetsAChannel(t *testing.T) {
	d := NewDeferrals()
	now := time.Now()
	d.Hold(held("c1", now), now)
	d.Drop("c1")
	if d.Len() != 0 {
		t.Error("Drop left the approach in place")
	}
}

func TestDeferralsAreBounded(t *testing.T) {
	d := NewDeferrals()
	base := time.Now()
	for i := 0; i < maxDeferredChannels+50; i++ {
		ch := string(rune('a'+i%26)) + string(rune('a'+i/26))
		d.Hold(held(ch, base.Add(time.Duration(i)*time.Second)), base)
	}
	if d.Len() > maxDeferredChannels {
		t.Errorf("holding %d approaches, want at most %d", d.Len(), maxDeferredChannels)
	}
}

func TestDeferralsAreSafeUnderConcurrentUse(t *testing.T) {
	d := NewDeferrals()
	now := time.Now()

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ch := string(rune('a' + i%4))
			d.Hold(held(ch, now), now)
			_ = d.Due(now.Add(time.Hour))
			_ = d.Len()
		}(i)
	}
	wg.Wait()
}
