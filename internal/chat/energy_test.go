package chat

import (
	"math"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/keshon/server-domme/internal/body"
	"github.com/keshon/server-domme/internal/mind"
)

// Step 5 of docs/persona-v3.md: things done to her, and reactions.

// The fifth coffee in an hour does nothing: each lift from the same person
// counts half the last.
func TestGiftsFromOnePersonDiminish(t *testing.T) {
	h := newHarness(t)
	h.svc.body = body.New(time.UTC, nil, clock)
	h.svc.body.Drain(0.8)
	before := h.svc.battery()
	for i := 0; i < 3; i++ {
		h.svc.applyEnergy(testGuild, bigM, 0.1, clock.Add(time.Duration(i)*time.Minute))
	}
	if got := h.svc.battery() - before; math.Abs(got-0.175) > 1e-9 {
		t.Errorf("three lifts gave %.3f, want 0.175", got)
	}
	// Someone else's lift counts in full.
	before = h.svc.battery()
	h.svc.applyEnergy(testGuild, "u2", 0.1, clock)
	if got := h.svc.battery() - before; math.Abs(got-0.1) > 1e-9 {
		t.Errorf("another person's lift gave %.3f", got)
	}
}

// Reactions to her messages reach her as a fact in the next scene there,
// and start again once she has spoken.
func TestReactionsReachHerAsFacts(t *testing.T) {
	h := newHarness(t)
	h.svc.conv.Record(testChannel, mind.Turn{UserID: bigM, Username: "Big M", Content: "look", At: clock})
	h.svc.conv.Record(testChannel, mind.Turn{FromBot: true, MessageID: "s1", Content: "nice", At: clock})
	for _, u := range []string{bigM, "u9"} {
		h.svc.ObserveReaction(&discordgo.MessageReaction{MessageID: "s1", ChannelID: testChannel, UserID: u, Emoji: discordgo.Emoji{Name: "❤️"}})
	}
	h.svc.ObserveReaction(&discordgo.MessageReaction{MessageID: "not-hers", ChannelID: testChannel, UserID: bigM, Emoji: discordgo.Emoji{Name: "😂"}})

	got := h.svc.reactionsIn(testChannel, h.svc.conv.Recent(testChannel))
	if len(got) != 1 || got[0].Count != 2 || len(got[0].Names) != 2 || got[0].Names[0] != "Big M" {
		t.Fatalf("reactions %+v", got)
	}
	h.svc.noteSpoke(mind.Scene{ChannelID: testChannel}, clock)
	if got := h.svc.reactionsIn(testChannel, nil); len(got) != 0 {
		t.Errorf("still %+v after she spoke", got)
	}
}
