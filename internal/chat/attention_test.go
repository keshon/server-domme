package chat

import (
	"strings"
	"testing"
	"time"

	"github.com/keshon/server-domme/internal/mind"
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
	if err := store.WarmMindPerson(testGuild, "u1", 0.9, now); err != nil {
		t.Fatalf("WarmMindPerson: %v", err)
	}
	if err := store.SetMindAttention(testGuild, "u1", string(mind.AttentionKeen), now); err != nil {
		t.Fatalf("SetMindAttention: %v", err)
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
	if err := store.WarmMindPerson(testGuild, "u1", 0.9, now); err != nil {
		t.Fatalf("WarmMindPerson: %v", err)
	}

	svc.considerReaching(now)
	if _, ok := queued(svc); ok {
		t.Error("reached out to someone who never opted in")
	}

	// Opted in, but the server switched it off.
	if err := store.SetMindAttention(testGuild, "u1", string(mind.AttentionKeen), now); err != nil {
		t.Fatalf("SetMindAttention: %v", err)
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
	if err := store.SetMindAttention(testGuild, "u1", string(mind.AttentionInsistent), time.Now()); err != nil {
		t.Fatalf("SetMindAttention: %v", err)
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

	if err := store.SetMindAttention(testGuild, "u1", string(mind.AttentionKeen), time.Now()); err != nil {
		t.Fatalf("SetMindAttention: %v", err)
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
