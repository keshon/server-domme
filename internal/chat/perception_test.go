package chat

import (
	"testing"
	"time"

	"github.com/keshon/server-domme/internal/mind"
)

// Shadow mode: the label is taken out and recorded, and nothing about her
// moves — not the bond with them, not her mood.
func TestPerceivedRecordsTheLabelAndChangesNothing(t *testing.T) {
	store := testStore(t)
	svc := newTestService(t, store, 0)
	now := time.Now()
	mood := svc.drives(testGuild, testChannel, now).Mood
	tk := task{item: mind.Deferred{GuildID: testGuild, ChannelID: testChannel, UserID: "u1"}}

	sp := &spoken{}
	msg, ok := svc.perceived(tk, sp, "<tone>hostile</tone>\nnot today")
	if !ok || msg != "not today" {
		t.Fatalf("posted %q (ok %v)", msg, ok)
	}
	if sp.perceived != string(mind.PerceivedHostile) {
		t.Errorf("journal gets %q", sp.perceived)
	}
	if p := store.GetMindPerson(testGuild, "u1"); p != nil && (p.Tension > 0 || p.LastEvent != "") {
		t.Errorf("a shadow label moved the bond: %+v", p)
	}
	if svc.drives(testGuild, testChannel, now).Mood != mood {
		t.Error("a shadow label moved her mood")
	}

	sp = &spoken{}
	if _, ok := svc.perceived(tk, sp, "no label here"); !ok || sp.perceived != perceptionUnreadable {
		t.Errorf("a missing label recorded as %q", sp.perceived)
	}
}

// Production, 17:17: "well, maybe I like you.." was answered "thanks", typed,
// and dropped as though the bot were shutting down. The last-word check meant
// for afterthoughts ran on every declinable answer, whose t.after is empty.
func TestAnAnswerIsNeverOvertakenLikeAnAfterthought(t *testing.T) {
	svc := newTestService(t, testStore(t), 0)
	now := time.Now()
	svc.conv.Record(testChannel, mind.Turn{UserID: "u1", Username: "Big M", Content: "well, maybe I like you..", At: now})

	answer := task{item: mind.Deferred{GuildID: testGuild, ChannelID: testChannel, UserID: "u1", Trigger: mind.TriggerFollowUp}}
	if !svc.stillHerMove(answer) {
		t.Error("a follow-up answer was treated as overtaken")
	}

	after := task{item: mind.Deferred{ChannelID: testChannel, Trigger: mind.TriggerAfterthought}, after: "b1"}
	if svc.stillHerMove(after) {
		t.Error("an afterthought went out after they had spoken")
	}
}
