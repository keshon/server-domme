package chat

import (
	"context"
	"testing"

	"github.com/bwmarrin/discordgo"
	"github.com/keshon/server-domme/internal/mind"
)

// Step 7 of docs/persona-v3.md, as the service runs it.

// A channel she only reads is kept for her walks, and nothing is ever
// answered there.
func TestAReadsChannelIsWalkedNeverAnswered(t *testing.T) {
	h := newHarness(t)
	h.svc.walks = true
	if err := h.store.SetChatReads(testGuild, "art", true); err != nil {
		t.Fatal(err)
	}
	m := message("m1", "@Domme look at my dragon", true)
	m.ChannelID = "art"
	h.svc.Observe(h.sess, m)
	if _, ok := h.take(); ok {
		t.Fatal("answered in a channel she only reads")
	}
	w, channelID := h.svc.walk(h.sess, testGuild)
	if w == nil || channelID != "art" || len(w.Lines) != 1 {
		t.Fatalf("walk %+v through %q", w, channelID)
	}
	h.svc.walked["art"] = clock
	if w, _ := h.svc.walk(h.sess, testGuild); w != nil {
		t.Error("walked through again with nothing new")
	}
}

// An impulse to a room is carried out where she may speak up, within the
// day's limits.
func TestAnImpulseToARoomIsSpoken(t *testing.T) {
	h := newHarness(t, "anyone else think the new pins are ugly")
	if err := h.store.SetChatProactive(testGuild, testChannel, true); err != nil {
		t.Fatal(err)
	}
	h.svc.act(context.Background(), h.sess, testGuild, clock, mind.Idle{GuildID: testGuild, Now: clock},
		mind.Impulse{About: "the new pins are ugly"})
	if got := h.discord.posted(); len(got) != 1 {
		t.Fatalf("posted %v", got)
	}
	if len(h.svc.startedMsgs) != 1 {
		t.Error("what she started is not being watched")
	}
}

// Someone neither around nor consenting cannot be gone after.
func TestAnImpulseAtSomeoneAwayNeedsConsent(t *testing.T) {
	h := newHarness(t, "hey")
	h.svc.act(context.Background(), h.sess, testGuild, clock, mind.Idle{GuildID: testGuild, Now: clock},
		mind.Impulse{Person: &mind.Candidate{ID: "u7", Name: "Away"}, About: "say hi"})
	if got := h.discord.posted(); len(got) != 0 {
		t.Errorf("reached someone without consent: %v", got)
	}
}

// Where she only answers, an overheard remark that touches her may get a
// reaction and never words.
func TestWhereSheOnlyAnswersSheOnlyReacts(t *testing.T) {
	h := newHarness(t, appraisal(`"act":"reply","intent":"weigh in on rust"`))
	h.svc.reactFirst, h.svc.interest = true, true
	h.svc.character.Specifics = []string{"hates Rust because the compiler lectures her"}
	m := message("m1", "the rust compiler is lecturing me again today", false)
	m.Author = &discordgo.User{ID: "u9", Username: "Stranger"}
	h.svc.Observe(h.sess, m)
	tk, ok := h.take()
	if !ok || !tk.item.ReactOnly {
		t.Fatalf("queued %+v, %v", tk.item, ok)
	}
	h.svc.handle(context.Background(), tk)
	if got := h.discord.posted(); len(got) != 0 {
		t.Errorf("spoke unasked where she only answers: %v", got)
	}
}
