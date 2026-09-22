package chat

import (
	"testing"
	"time"

	"github.com/keshon/server-domme/internal/body"
	"github.com/keshon/server-domme/internal/mind"
)

// Woken from sleep she is online, marked woken, and says so under her name;
// waking her again does nothing; without a body there is nothing to wake.
func TestWakingHer(t *testing.T) {
	h := newHarness(t)
	if got := h.svc.Wake(); got != WakeNoBody {
		t.Errorf("without a body: %s", got)
	}
	night := time.Date(2026, 9, 23, 3, 0, 0, 0, time.UTC)
	h.svc.now = func() time.Time { return night }
	h.svc.body = body.New(time.UTC, nil, night)
	if got := h.svc.statusText(night); got != statusAsleep {
		t.Errorf("asleep, the status reads %q", got)
	}
	if got := h.svc.Wake(); got != WakeFromBed {
		t.Fatalf("woke: %s", got)
	}
	if !h.svc.online() || h.svc.statusText(night) != statusWoken {
		t.Errorf("after waking: online %v, status %q", h.svc.online(), h.svc.statusText(night))
	}
	if got := h.svc.Wake(); got != WakeAwake {
		t.Errorf("woken twice: %s", got)
	}
	var sc mind.Scene
	h.svc.bodyScene(&sc, night)
	if !sc.WokenEarly || !sc.Woke.Equal(night) {
		t.Errorf("her scene does not say she was woken: %+v", sc)
	}
}

// The status line follows facts, and names no channel, server or person.
func TestTheStatusLineFollowsWhatSheIsDoing(t *testing.T) {
	h := newHarness(t)
	if got := h.svc.statusText(clock); got != statusAround {
		t.Errorf("quiet, without a body: %q", got)
	}
	h.svc.noteSpoke(mind.Scene{GuildID: testGuild, ChannelID: testChannel}, clock)
	if got := h.svc.statusText(clock); got != statusChat {
		t.Errorf("having just spoken: %q", got)
	}
	if got := h.svc.statusText(clock.Add(time.Hour)); got != statusAround {
		t.Errorf("an hour later: %q", got)
	}
}
