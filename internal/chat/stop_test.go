package chat

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/keshon/server-domme/internal/memory"
	"github.com/keshon/server-domme/internal/mind"
)

// The 2026-09-23 evening: he said it plainly, and she carried on for forty
// minutes. Asked to drop it, she drops it, and the intentions that kept her
// coming back are closed with it.
func TestBeingAskedToDropItClosesWhatSheMeantToDo(t *testing.T) {
	h := newHarness(t)
	for _, th := range []memory.Thread{
		{Text: "watch whether he answers the actual question", Person: memory.Ref{ID: bigM, Name: "Big M"}},
		{Text: "see if Duchess posts the pics", Person: memory.Ref{ID: "u9", Name: "Duchess"}},
	} {
		if err := h.memory.AddThread(testGuild, th); err != nil {
			t.Fatal(err)
		}
	}

	h.svc.dropIt(testGuild, bigM, clock)

	if at := h.svc.droppedAt(testGuild, bigM, clock); !at.Equal(clock) {
		t.Errorf("being asked to stop was not recorded: %v", at)
	}
	threads, err := h.memory.Threads(testGuild)
	if err != nil {
		t.Fatal(err)
	}
	open := memory.Unfinished(threads)
	if len(open) != 1 || open[0].Person.ID != "u9" {
		t.Errorf("left open %+v", open)
	}
}

// It wears off, and it never touched anyone else.
func TestBeingAskedToDropItHoldsForTheDayAndOnlyForThem(t *testing.T) {
	h := newHarness(t)
	h.svc.dropIt(testGuild, bigM, clock)
	if !h.svc.droppedAt(testGuild, "u9", clock).IsZero() {
		t.Error("somebody else was dropped too")
	}
	if !h.svc.droppedAt(testGuild, bigM, clock.Add(droppedFor+time.Minute)).IsZero() {
		t.Error("it still held a day later")
	}
}

// She does not go to someone who asked her to stop, however good the
// opening looks: "sleep well - i'll be here if you meant it" was one of
// these, sent after he had asked her twice to leave it.
func TestSheDoesNotStartAnythingWithSomeoneWhoAskedHerToStop(t *testing.T) {
	h := newHarness(t, "still thinking about what you said")
	h.svc.dropIt(testGuild, bigM, clock)
	base := mind.Scene{GuildID: testGuild, Now: clock}
	opening := mind.Opening{ChannelID: testChannel, ChannelName: "chat", UserID: bigM, Username: "Big M",
		Trigger: mind.TriggerStart}
	h.svc.start(context.Background(), h.sess, base, opening, mind.Plan{Intent: "ask him if he meant it"})
	if posted := h.discord.posted(); len(posted) != 0 {
		t.Errorf("posted %q", posted)
	}
}

// The same demand in new words is the same demand. The third time, the code
// lets it go without asking her to.
func TestPuttingTheSameAskTwiceIsEnough(t *testing.T) {
	h := newHarness(t)
	asks := []string{
		"get him to answer whether he actually meant it",
		"press him on whether he meant what he said",
		"ask him again whether he meant it, actually",
	}
	for i, ask := range asks {
		got := h.svc.pressing(testGuild, testChannel, bigM, ask, clock.Add(time.Duration(i)*time.Minute))
		if want := i == len(asks)-1; got != want {
			t.Errorf("ask %d: pressing %v, want %v", i+1, got, want)
		}
	}
	// A different thing to say is not pressing.
	if h.svc.pressing(testGuild, testChannel, bigM, "tell him the pins are sorted", clock.Add(time.Minute)) {
		t.Error("a new subject counted as pressing")
	}
	// And the count does not last the evening.
	if h.svc.pressing(testGuild, testChannel, bigM, asks[0], clock.Add(pressWithin+time.Minute)) {
		t.Error("the count carried past its window")
	}
}

// The whole flow: she decides to reply, the appraisal says they asked her to
// drop it, and nothing goes out.
func TestAnAskToDropItStopsTheReply(t *testing.T) {
	h := newHarness(t,
		appraisal(`"read":"he wants this to end","feel":"stung","act":"reply","drop":true,"intent":"push him on it once more","then":"see if he comes back"`),
		"you're not walking away from this again",
	)
	h.say(t, "m1", "@Domme drop it. I want out of this conversation.", true)

	// She may say one thing, but not the thing she was carrying.
	told := h.provider.sent[len(h.provider.sent)-1]
	said := told[len(told)-1].Content
	if strings.Contains(said, "push him on it") || !strings.Contains(said, "let it go") {
		t.Errorf("the voice was told: %q", said)
	}
	entries := h.store.MindJournalIn(testGuild, testChannel)
	if len(entries) != 1 || !strings.Contains(entries[0].Reason, "drop it") {
		t.Errorf("journal is %+v", entries)
	}
	if h.svc.droppedAt(testGuild, bigM, clock).IsZero() {
		t.Error("she did not take it as being asked to stop")
	}
}
