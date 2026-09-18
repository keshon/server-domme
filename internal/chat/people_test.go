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
