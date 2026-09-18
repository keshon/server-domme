package chat

import (
	"strings"
	"testing"
	"time"

	"github.com/keshon/server-domme/internal/mind"
	"github.com/keshon/server-domme/internal/storage"
)

// Someone fond, opted in, and silent for a day: she goes after them.
func TestReachesOutToSomeoneSheMisses(t *testing.T) {
	store := testStore(t)
	proactiveChannel(t, store)
	svc := awakeService(t, store)
	now := time.Now()

	if err := store.ExchangeMindPerson(testGuild, "u1", testChannel, now.Add(-30*time.Hour)); err != nil {
		t.Fatalf("ExchangeMindPerson: %v", err)
	}
	if err := store.UpdateMindPerson(testGuild, "u1", now, func(p *storage.MindPerson) {
		p.Closeness, p.ClosenessAt = 0.9, now
	}); err != nil {
		t.Fatalf("UpdateMindPerson: %v", err)
	}
	if err := store.SetMindConsent(testGuild, "u1", mind.ConsentOn, now); err != nil {
		t.Fatalf("SetMindConsent: %v", err)
	}

	svc.considerReaching(now)

	got, ok := queued(svc)
	if !ok {
		t.Fatal("did not reach out to someone she misses who opted in")
	}
	if got.item.Trigger != mind.TriggerReach || got.item.UserID != "u1" || got.item.Volunteering == "" {
		t.Errorf("queued %+v", got.item)
	}
	if p := store.GetMindPerson(testGuild, "u1"); p.ReachToday != 1 || p.Unanswered != 1 {
		t.Errorf("reach not recorded: %+v", p)
	}
}

func TestNeverReachesOutWithoutConsent(t *testing.T) {
	store := testStore(t)
	proactiveChannel(t, store)
	svc := awakeService(t, store)
	now := time.Now()
	if err := store.ExchangeMindPerson(testGuild, "u1", testChannel, now.Add(-30*time.Hour)); err != nil {
		t.Fatalf("ExchangeMindPerson: %v", err)
	}
	if err := store.UpdateMindPerson(testGuild, "u1", now, func(p *storage.MindPerson) {
		p.Closeness, p.ClosenessAt = 0.9, now
	}); err != nil {
		t.Fatalf("UpdateMindPerson: %v", err)
	}

	svc.considerReaching(now)
	if _, ok := queued(svc); ok {
		t.Error("reached out to someone who never opted in")
	}

	// Opted in, but the server switched it off.
	if err := store.SetMindConsent(testGuild, "u1", mind.ConsentOn, now); err != nil {
		t.Fatalf("SetMindConsent: %v", err)
	}
	if err := store.SetChatAttentionOff(testGuild, true); err != nil {
		t.Fatalf("SetChatAttentionOff: %v", err)
	}
	svc.considerReaching(now)
	if _, ok := queued(svc); ok {
		t.Error("reached out in a server that switched it off")
	}
}

// Consent that takes a command to withdraw is not much of a consent.
func TestLeaveMeAloneWithdrawsConsent(t *testing.T) {
	store := testStore(t)
	proactiveChannel(t, store)
	svc := newTestService(t, store, 0)
	if err := store.SetMindConsent(testGuild, "u1", mind.ConsentOn, time.Now()); err != nil {
		t.Fatalf("SetMindConsent: %v", err)
	}

	svc.Observe(testSession(), message("@Domme leave me alone", true))

	if p := store.GetMindPerson(testGuild, "u1"); p.Attention != "" {
		t.Errorf("still opted in at %q after asking her to back off", p.Attention)
	}
}

// Activity elsewhere is a timestamp for people who opted in, and nothing at
// all for anyone else.
func TestActivityElsewhereIsOnlyATimestampForTheOptedIn(t *testing.T) {
	store := testStore(t)
	proactiveChannel(t, store)
	svc := newTestService(t, store, 0)
	elsewhere := message("anyone up for a game", false)
	elsewhere.ChannelID = "not-hers"

	svc.Observe(testSession(), elsewhere)
	if p := store.GetMindPerson(testGuild, "u1"); p != nil {
		t.Errorf("recorded someone who never opted in: %+v", p)
	}

	if err := store.SetMindConsent(testGuild, "u1", mind.ConsentOn, time.Now()); err != nil {
		t.Fatalf("SetMindConsent: %v", err)
	}
	svc.Observe(testSession(), elsewhere)
	if p := store.GetMindPerson(testGuild, "u1"); p == nil || p.LastActiveAt.IsZero() {
		t.Error("did not notice an opted-in person was around")
	}
	if len(svc.conv.Recent("not-hers")) != 0 {
		t.Error("kept what was said in a channel she does not read")
	}
}

func TestReachingOutAlwaysTagsThem(t *testing.T) {
	store := testStore(t)
	svc := newTestService(t, store, 0)
	msg := svc.outgoing(task{item: mind.Deferred{ChannelID: testChannel, UserID: "u1", Username: "Big M", Trigger: mind.TriggerReach}}, "you went quiet")
	if !strings.HasPrefix(msg.Content, "<@u1> ") {
		t.Errorf("content = %q", msg.Content)
	}
	if got := msg.AllowedMentions.Users; len(got) != 1 || got[0] != "u1" {
		t.Errorf("may ping %v", got)
	}
}

// Welcome is learned: answered soon after she came to them, she feels more
// welcome; left unanswered and going again, less.
func TestWelcomeIsLearnedFromHowTheyTakeIt(t *testing.T) {
	store := testStore(t)
	proactiveChannel(t, store)
	svc := newTestService(t, store, 0)
	now := time.Now()
	if err := store.SetMindConsent(testGuild, "u1", mind.ConsentOn, now); err != nil {
		t.Fatalf("SetMindConsent: %v", err)
	}
	if err := store.MarkReached(testGuild, "u1", svc.day(now), now.Add(-10*time.Minute)); err != nil {
		t.Fatalf("MarkReached: %v", err)
	}

	svc.Observe(testSession(), message("@Domme hey, missed you too", true))

	p := store.GetMindPerson(testGuild, "u1")
	if _, _, welcome := bondOf(p).Now(now); welcome <= 0.5 {
		t.Errorf("answered quickly and welcome is %.2f", welcome)
	}
	if p.LastEvent != string(mind.EventReachAnsweredQuickly) || p.Unanswered != 0 {
		t.Errorf("record after a quick answer: event %q, unanswered %d", p.LastEvent, p.Unanswered)
	}
}
