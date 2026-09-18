package chat

import (
	"strings"
	"testing"
	"time"

	"github.com/keshon/server-domme/internal/mind"
	"github.com/keshon/server-domme/internal/storage"
)

// greeted puts a greeting of hers to u1 in flight, as speak does once it is
// posted, and returns the journal entry it belongs to.
func greeted(t *testing.T, svc *Service, at time.Time) uint64 {
	t.Helper()
	id := svc.journalOpen(storage.MindJournal{
		GuildID: testGuild, ChannelID: testChannel, At: at, UserID: "u1",
		Trigger: string(mind.TriggerReturn), Outcome: outcomeAnswered,
	})
	svc.awaitPayoff(task{item: mind.Deferred{
		GuildID: testGuild, ChannelID: testChannel, UserID: "u1",
		Trigger: mind.TriggerReturn, Journal: id,
	}}, at)
	return id
}

func payoffNote(svc *Service) string {
	entries := svc.Journal(testGuild, testChannel)
	if len(entries) == 0 {
		return ""
	}
	return entries[len(entries)-1].Payoff
}

func TestBeingTakenUpRewardsHer(t *testing.T) {
	store := testStore(t)
	svc := newTestService(t, store, 0)
	now := time.Now()
	mood := svc.drives(testGuild, testChannel, now).Mood

	greeted(t, svc, now)
	svc.settlePayoffs(testChannel, "u1", "hahaha good to be back", now.Add(time.Minute))

	after := now.Add(time.Minute)
	if svc.drives(testGuild, testChannel, after).Mood <= mood {
		t.Error("a laugh she had no reason to expect did not lift her")
	}
	if svc.welcomeOf(testGuild, "u1", after) <= 0.5 {
		t.Error("being taken up taught her nothing about them")
	}
	if note := payoffNote(svc); !strings.HasPrefix(note, string(mind.PayoffLaughed)) {
		t.Errorf("journal payoff %q", note)
	}
}

func TestBeingIgnoredStingsAndTeaches(t *testing.T) {
	store := testStore(t)
	svc := newTestService(t, store, 0)
	now := time.Now()
	mood := svc.drives(testGuild, testChannel, now).Mood

	greeted(t, svc, now)
	later := now.Add(mind.PayoffWindow + time.Minute)
	svc.expirePayoffs(later)

	if svc.drives(testGuild, testChannel, later).Mood >= mood {
		t.Error("being ignored did not touch her")
	}
	if svc.welcomeOf(testGuild, "u1", later) >= 0.5 {
		t.Error("being ignored taught her nothing about them")
	}
	if note := payoffNote(svc); !strings.HasPrefix(note, string(mind.PayoffIgnored)) {
		t.Errorf("journal payoff %q", note)
	}
}

// A greeting to one person is settled by that person, not by whoever speaks.
func TestSomeoneElseDoesNotSettleAGreeting(t *testing.T) {
	store := testStore(t)
	svc := newTestService(t, store, 0)
	now := time.Now()
	greeted(t, svc, now)

	svc.settlePayoffs(testChannel, "u2", "lol", now.Add(time.Minute))
	if note := payoffNote(svc); note != "" {
		t.Errorf("settled by someone else: %q", note)
	}
}

// Talking into a room that does not answer cures no loneliness; being spoken
// to does.
func TestContactNotHerOwnSpeakingSatisfiesTheNeed(t *testing.T) {
	store := testStore(t)
	svc := newTestService(t, store, 0)
	now := time.Now()
	svc.markContact(testGuild, now.Add(-8*time.Hour))
	if err := store.MarkMindSpoke(testGuild, now); err != nil {
		t.Fatal(err)
	}
	if lonely := svc.drives(testGuild, testChannel, now).Social; lonely < 0.9 {
		t.Errorf("speaking into silence satisfied her: social need %.2f", lonely)
	}
	svc.markContact(testGuild, now)
	if lonely := svc.drives(testGuild, testChannel, now).Social; lonely > 0.1 {
		t.Errorf("being spoken to did not satisfy her: social need %.2f", lonely)
	}
}

// The dopamine loop, end to end. She has been alone all afternoon and greets
// someone. If it pays off, the need behind it is satisfied and greeting the
// next person comes less readily; if it falls flat, the need is still there
// and she is about as ready to try someone else as before. Fatigue is left
// out, since it is paid either way.
func TestAGreetingThatPaysOffSatisfiesTheNeedBehindIt(t *testing.T) {
	chanceToGreet := func(svc *Service, at time.Time) float64 {
		_, c := mind.MayVolunteer(mind.Volunteer{
			Trigger: mind.TriggerReturn, Now: at, Enabled: true,
			EngagedWindow: time.Minute, UserID: "u2",
			Drives: svc.drives(testGuild, testChannel, at),
		}, 1)
		return c
	}

	run := func(answered bool) (before, after float64) {
		store := testStore(t)
		svc := awakeService(t, store)
		now := time.Now()
		svc.markContact(testGuild, now.Add(-6*time.Hour))
		before = chanceToGreet(svc, now)

		greeted(t, svc, now)
		later := now.Add(2 * time.Minute)
		if answered {
			svc.markContact(testGuild, later)
			svc.settlePayoffs(testChannel, "u1", "haha hey, good to be back", later)
		} else {
			later = now.Add(mind.PayoffWindow + time.Minute)
			svc.expirePayoffs(later)
		}
		return before, chanceToGreet(svc, later)
	}

	before, sated := run(true)
	_, unsated := run(false)
	t.Logf("chance to greet the next person: %.2f before; %.2f after being taken up; %.2f after being ignored", before, sated, unsated)
	if sated >= before-0.05 {
		t.Errorf("a greeting that paid off left her as eager as before: %.2f → %.2f", before, sated)
	}
	if unsated <= sated {
		t.Errorf("being ignored left her less inclined than being rewarded: ignored %.2f, rewarded %.2f", unsated, sated)
	}
}
