package chat

import (
	"context"
	"testing"
	"time"

	"github.com/keshon/server-domme/internal/ai"
	"github.com/keshon/server-domme/internal/mind"
)

// canned is a backend that always answers the same thing.
type canned string

func (c canned) Generate(context.Context, []ai.Message) (string, error) { return string(c), nil }

func TestNotePeopleFilesWhatWasSaidAboutThemselves(t *testing.T) {
	store := testStore(t)
	svc := newTestService(t, store, 0)
	svc.provider = canned("FACT Big M: job = night shift nurse\n" +
		"IMPRESSION Big M: talks big, backs it up.\n" +
		"FACT John: city = Lisbon")

	now := time.Now()
	turns := []mind.Turn{
		{UserID: "u1", Username: "Big M", Content: "i work nights at the hospital", At: now},
		{FromBot: true, Content: "explains a lot", At: now},
	}
	svc.notePeople(context.Background(), testGuild, turns, now)

	p := store.GetMindPerson(testGuild, "u1")
	if p == nil || len(p.Facts) != 1 || p.Facts[0].Value != "night shift nurse" {
		t.Fatalf("facts not filed: %+v", p)
	}
	if p.Impression != "talks big, backs it up." {
		t.Errorf("impression = %q", p.Impression)
	}
	// John was never in the conversation. A name the model produced on its
	// own is not someone to keep a file on.
	if got := store.GetMindPerson(testGuild, "John"); got != nil {
		t.Errorf("filed notes on someone who was not there: %+v", got)
	}
}

// A second conversation revises the file rather than replacing it: an empty
// impression line keeps the old one, and facts merge.
func TestNotePeopleRevisesRatherThanReplaces(t *testing.T) {
	store := testStore(t)
	svc := newTestService(t, store, 0)
	now := time.Now()
	turns := []mind.Turn{{UserID: "u1", Username: "Big M", Content: "hi", At: now}}

	svc.provider = canned("FACT Big M: job = nurse\nIMPRESSION Big M: all talk.")
	svc.notePeople(context.Background(), testGuild, turns, now.Add(-time.Hour))

	svc.provider = canned("FACT Big M: pet = a cat")
	svc.notePeople(context.Background(), testGuild, turns, now)

	p := store.GetMindPerson(testGuild, "u1")
	if p == nil || len(p.Facts) != 2 || p.Impression != "all talk." {
		t.Errorf("file after two conversations: %+v", p)
	}
}

func TestWarmToGrowsFondnessFromAWarmOneToOne(t *testing.T) {
	store := testStore(t)
	svc := newTestService(t, store, 0)
	now := time.Now()

	svc.warmTo(testGuild, []string{"u1"}, mind.ToneWarm, now)
	svc.warmTo(testGuild, []string{"u1"}, mind.ToneWarm, now)

	p := store.GetMindPerson(testGuild, "u1")
	if p == nil || p.Warmth < 0.29 {
		t.Errorf("warmth after two warm conversations: %+v", p)
	}
}

// She asked Big M something; he replied to John instead. Once, not per
// message after it.
func TestObserveNoticesBeingBrushedOffOnce(t *testing.T) {
	store := testStore(t)
	if err := store.AddChatChannel(testGuild, testChannel); err != nil {
		t.Fatalf("AddChatChannel: %v", err)
	}
	svc := newTestService(t, store, 0.9999)
	now := time.Now()
	svc.conv.Record(testChannel, mind.Turn{UserID: "u2", Username: "John", Content: "anyone?", MessageID: "j1", At: now.Add(-2 * time.Minute)})
	svc.conv.Record(testChannel, mind.Turn{FromBot: true, MessageID: "q1", To: "u1", Content: "what did you break?", At: now.Add(-time.Minute)})

	for i := 0; i < 2; i++ {
		m := replyTo("j1", "yeah i'm here john")
		m.ID = "snub" + string(rune('a'+i))
		svc.Observe(testSession(), m)
	}

	p := store.GetMindPerson(testGuild, "u1")
	if p == nil || p.Irritation < mind.BrushOffStep-0.01 || p.Irritation > mind.BrushOffStep+0.01 {
		t.Errorf("irritation after being brushed off = %+v, want one step", p)
	}
}

// "lame" answering her moves her at once and reaches her next reply to that
// person; the same word in a message not answering her does nothing.
func TestObserveTakesAReactionToHerLastLine(t *testing.T) {
	store := testStore(t)
	if err := store.AddChatChannel(testGuild, testChannel); err != nil {
		t.Fatalf("AddChatChannel: %v", err)
	}
	svc := newTestService(t, store, 0.9999)
	now := time.Now()
	svc.conv.Record(testChannel, mind.Turn{UserID: "u1", Username: "cass", Content: "tell me a joke", At: now.Add(-time.Minute)})
	svc.conv.Record(testChannel, mind.Turn{FromBot: true, MessageID: "b1", To: "u1", Content: "why did the chicken join discord?", At: now.Add(-30 * time.Second)})

	svc.Observe(testSession(), message("this one is lame meeeh", false))

	if p := store.GetMindPerson(testGuild, "u1"); p == nil || p.Irritation < mind.PannedIrritation-0.01 {
		t.Errorf("being panned left irritation at %+v", p)
	}
	if got := svc.receptionFor(testChannel, "u1", "cass", time.Now()); got == "" {
		t.Error("her next reply to them is not told how the last one went")
	}
	if got := svc.receptionFor(testChannel, "u2", "john", time.Now()); got != "" {
		t.Errorf("someone else's reply is coloured by it: %q", got)
	}
}

// "same" after her own line usually ends it, and letting it go must not count
// as ignoring them: otherwise their next message would be forced through and
// a quick one would count as pestering.
func TestObserveLetsACloserGoWithoutHoldingItAgainstThem(t *testing.T) {
	store := testStore(t)
	if err := store.AddChatChannel(testGuild, testChannel); err != nil {
		t.Fatalf("AddChatChannel: %v", err)
	}
	svc := newTestService(t, store, 0.5)
	now := time.Now()
	svc.conv.Record(testChannel, mind.Turn{UserID: "u1", Username: "cass", Content: "how are you?", At: now.Add(-time.Minute)})
	svc.conv.Record(testChannel, mind.Turn{FromBot: true, MessageID: "b1", To: "u1", Content: "still here. you?", At: now.Add(-30 * time.Second)})

	svc.Observe(testSession(), message("same", false))

	if got, ok := queued(svc); ok {
		t.Errorf("answered a closer at even odds: %+v", got.item)
	}
	_, ignoredLast := svc.encounters.Approach(encounterKey(testGuild, testChannel, "u1"))
	if ignoredLast {
		t.Error("letting a closer go was recorded as ignoring them")
	}
}
