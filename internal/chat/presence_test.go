package chat

import (
	"context"
	"testing"
	"time"

	"github.com/keshon/server-domme/internal/body"
	"github.com/keshon/server-domme/internal/mind"
)

// Step 4 of docs/persona-v3.md: the body, as the service lives it.

// asleepBody is a body that is asleep: made at three in the morning.
func asleepBody() *body.Body {
	return body.New(time.UTC, nil, time.Date(2026, 9, 19, 3, 0, 0, 0, time.UTC))
}

// Asleep, someone speaking to her waits for her; nothing is handled.
func TestAsleepSheMissesWhatIsSaidToHer(t *testing.T) {
	h := newHarness(t)
	h.svc.body = asleepBody()
	h.svc.Observe(h.sess, message("m1", "@Domme you up?", true))
	if _, ok := h.take(); ok {
		t.Fatal("handled while asleep")
	}
	if len(h.svc.missed) != 1 {
		t.Fatalf("missed %d", len(h.svc.missed))
	}
	entries := h.store.MindJournalIn(testGuild, testChannel)
	if len(entries) != 1 || entries[0].Outcome != outcomeAsleep {
		t.Errorf("journal %+v", entries)
	}
	// Asleep, nothing gets through.
	if !h.svc.noticeAt.IsZero() {
		t.Error("a mention got through to her asleep")
	}
}

// Back online, she catches up a room at a time, spaced out, and answers
// late only when it is late.
func TestComingBackSheCatchesUpPaced(t *testing.T) {
	h := newHarness(t)
	h.svc.body = asleepBody()
	for i, id := range []string{"m1", "m2"} {
		m := message(id, "@Domme hello?", true)
		if i == 1 {
			m.Author.ID, m.Author.Username = "u2", "Big N"
		}
		h.svc.Observe(h.sess, m)
	}
	h.svc.scheduleCatchUp(clock)
	if len(h.svc.catchUp) != 2 || !h.svc.catchUp[1].due.After(h.svc.catchUp[0].due) {
		t.Fatalf("catch-up %+v", h.svc.catchUp)
	}
	if !h.svc.catchUp[0].due.After(clock) {
		t.Error("answered the instant she came back")
	}

	h.svc.body = nil // online from here on
	h.svc.dispatchCatchUp(context.Background(), h.svc.catchUp[0].due)
	first, ok := h.take()
	if !ok || !first.catchUp {
		t.Fatalf("dispatched %+v", first)
	}
	if first.late {
		t.Error("a message from minutes ago was framed as late")
	}
}

// Tired, she does not take on other people's conversations.
func TestTiredSheDoesNotOverhear(t *testing.T) {
	h := newHarness(t)
	h.svc.body = body.New(time.UTC, nil, clock)
	h.svc.body.Drain(0.5)
	if err := h.store.SetChatProactive(testGuild, testChannel, true); err != nil {
		t.Fatal(err)
	}
	if h.svc.overhear(testGuild, testChannel, "u9", "a long enough remark about anything at all", clock) {
		t.Error("overheard while tired")
	}
}

// How much she takes in follows her energy: a shorter transcript and less
// recall when tired, the terse examples when exhausted.
func TestTiredSheTakesInLess(t *testing.T) {
	h := newHarness(t)
	var turns []mind.Turn
	for i := 0; i < 20; i++ {
		turns = append(turns, mind.Turn{UserID: bigM, Username: "Big M", Content: "line", At: clock})
	}
	h.svc.body = body.New(time.UTC, nil, clock)

	sc := mind.Scene{Turns: turns, Now: clock}
	h.svc.bodyScene(&sc, clock)
	if len(sc.Turns) != 20 || sc.RecallCap != 0 {
		t.Errorf("full: %d turns, recall cap %d", len(sc.Turns), sc.RecallCap)
	}

	h.svc.body.Drain(0.5)
	sc = mind.Scene{Turns: turns, Now: clock}
	h.svc.bodyScene(&sc, clock)
	if len(sc.Turns) != tailTired || sc.RecallCap != recallTired {
		t.Errorf("tired: %d turns, recall cap %d", len(sc.Turns), sc.RecallCap)
	}

	h.svc.body.Drain(0.3)
	sc = mind.Scene{Turns: turns, Now: clock}
	h.svc.bodyScene(&sc, clock)
	if len(sc.Turns) != tailExhaust || sc.RecallCap >= 0 || !sc.ShortExamples {
		t.Errorf("exhausted: %d turns, recall cap %d, short %v", len(sc.Turns), sc.RecallCap, sc.ShortExamples)
	}
}

// Going to sleep straight out of a conversation, she sometimes says so.
func TestGoingSheSometimesSaysSo(t *testing.T) {
	h := newHarness(t)
	h.svc.body = body.New(time.UTC, nil, clock)
	h.svc.noteSpoke(mind.Scene{GuildID: testGuild, ChannelID: testChannel, UserID: bigM, Username: "Big M"}, clock)
	h.svc.onPresence(body.Event{From: body.Online, To: body.Asleep, Why: body.WhySlept, At: clock.Add(time.Minute)})
	tk, ok := h.take()
	if !ok || tk.item.Trigger != mind.TriggerLeave || tk.item.ChannelID != testChannel {
		t.Fatalf("queued %+v, %v", tk.item, ok)
	}
}

// The body and what she missed survive a restart.
func TestTheBodySurvivesARestart(t *testing.T) {
	h := newHarness(t)
	h.svc.body = asleepBody()
	h.svc.Observe(h.sess, message("m1", "@Domme night?", true))
	h.svc.saveBody(clock)

	again := New(Deps{Storage: h.store, Memory: h.memory, Session: h.svc.session, Body: true,
		Now: func() time.Time { return clock }, Roll: func() float64 { return 0.99 }})
	if again.body == nil || len(again.missed) != 1 || again.missed[0].MessageID != "m1" {
		t.Fatalf("restored %+v with %d missed", again.body, len(again.missed))
	}
}
