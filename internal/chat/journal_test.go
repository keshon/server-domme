package chat

import (
	"testing"
	"time"

	"github.com/keshon/server-domme/internal/mind"
)

// The question the journal exists for: she did not answer — why? The entry
// has to say which rule, the odds and the roll.
func TestJournalRecordsWhyShedStayedQuiet(t *testing.T) {
	store := testStore(t)
	if err := store.AddChatChannel(testGuild, testChannel); err != nil {
		t.Fatalf("AddChatChannel: %v", err)
	}
	svc := newTestService(t, store, 0.9999)
	svc.encounters.Record(encounterKey(testGuild, testChannel, "u1"), mind.OutcomeSpeak, mind.TriggerMention)
	now := time.Now()
	svc.conv.Record(testChannel, mind.Turn{UserID: "u1", Username: "cass", Content: "hi", At: now.Add(-time.Minute)})
	svc.conv.Record(testChannel, mind.Turn{FromBot: true, MessageID: "b1", To: "u1", Content: "hey", At: now.Add(-30 * time.Second)})

	svc.Observe(testSession(), message("took you time to type it heh", false))

	entries := svc.Journal(testGuild, testChannel)
	if len(entries) != 1 {
		t.Fatalf("journal holds %d entries, want 1", len(entries))
	}
	e := entries[0]
	if e.Outcome != outcomeSilent || e.Trigger != string(mind.TriggerFollowUp) || e.Rule != mind.RuleOdds {
		t.Errorf("entry = %+v", e)
	}
	if e.Chance <= 0 || e.Roll < 0.99 || e.Excerpt == "" || e.Mood == "" {
		t.Errorf("entry is missing how she decided: %+v", e)
	}
	if got := svc.Today(testGuild)[countSilent]; got != 1 {
		t.Errorf("today's silent count = %d", got)
	}
}

// An answer opens its entry at the decision and completes it when it
// resolves, whichever way — the task carries the entry with it.
func TestJournalFollowsAnAnswerToItsEnd(t *testing.T) {
	store := testStore(t)
	if err := store.AddChatChannel(testGuild, testChannel); err != nil {
		t.Fatalf("AddChatChannel: %v", err)
	}
	svc := newTestService(t, store, 0)

	svc.Observe(testSession(), message("@Domme hello", true))
	task, ok := queued(svc)
	if !ok || task.item.Journal == 0 {
		t.Fatalf("queued task carries no journal entry: %+v", task.item)
	}

	svc.closeSpoken(task, &spoken{
		outcome: outcomeAnswered, posted: "hey", raw: "hey.", replyID: "b9",
		backend: "g4f:test", took: 2 * time.Second, told: []string{"You are tired."},
	})
	svc.journalReaction(testGuild, testChannel, "u1", mind.ReceptionLiked)

	e := svc.Journal(testGuild, testChannel)[0]
	if e.Outcome != outcomeAnswered || e.Posted != "hey" || e.Raw != "hey." || e.Backend != "g4f:test" {
		t.Errorf("entry after the answer: %+v", e)
	}
	if e.Reaction != string(mind.ReceptionLiked) {
		t.Errorf("reaction not tied to her reply: %q", e.Reaction)
	}
}

func TestJournalNotesALineThatJoinedAnAnswerInProgress(t *testing.T) {
	store := testStore(t)
	if err := store.AddChatChannel(testGuild, testChannel); err != nil {
		t.Fatalf("AddChatChannel: %v", err)
	}
	svc := newTestService(t, store, 0)

	svc.Observe(testSession(), message("@Domme hey", true))
	svc.Observe(testSession(), message("@Domme stop ignoring me", true))

	entries := svc.Journal(testGuild, testChannel)
	if len(entries) != 2 || entries[1].Outcome != outcomeJoined {
		t.Errorf("journal = %+v", entries)
	}
}
