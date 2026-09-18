package chat

import (
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/keshon/server-domme/internal/mind"
	"github.com/keshon/server-domme/internal/storage"
	"github.com/rs/zerolog"
)

// awakeService is a test service whose community clock reads early afternoon
// whenever the test runs. Energy follows the hour, and a suite run at 3am would
// otherwise find her too tired to volunteer and fail for a reason that has
// nothing to do with what is being tested.
func awakeService(t *testing.T, store *storage.Storage) *Service {
	t.Helper()
	offset := (13 - time.Now().UTC().Hour()) * 3600
	sess := testSession()
	return New(Deps{
		Character: &mind.Character{Name: "Domme", Persona: "someone"},
		Storage:   store,
		Session:   func() *discordgo.Session { return sess },
		Log:       zerolog.Nop(),
		Location:  time.FixedZone("afternoon", offset),
		Roll:      func() float64 { return 0 },
	})
}

// proactiveChannel opts the test channel in to both listening and speaking
// first.
func proactiveChannel(t *testing.T, store *storage.Storage) {
	t.Helper()
	if err := store.AddChatChannel(testGuild, testChannel); err != nil {
		t.Fatalf("AddChatChannel: %v", err)
	}
	if err := store.SetChatProactive(testGuild, testChannel, true); err != nil {
		t.Fatalf("SetChatProactive: %v", err)
	}
}

// regularAwayFor records u1 as someone she knows, last seen that long ago.
func regularAwayFor(t *testing.T, store *storage.Storage, away time.Duration) {
	t.Helper()
	then := time.Now().Add(-away)
	for i := 0; i < 20; i++ {
		if _, err := store.SeeMindPerson(testGuild, "u1", "cass", then.Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatalf("SeeMindPerson: %v", err)
		}
	}
}

func TestObserveGreetsARegularComingBack(t *testing.T) {
	store := testStore(t)
	proactiveChannel(t, store)
	regularAwayFor(t, store, 30*24*time.Hour)
	svc := awakeService(t, store)

	svc.Observe(testSession(), message("hey all", false))

	got, ok := queued(svc)
	if !ok {
		t.Fatal("said nothing to a regular back after a month")
	}
	if got.item.Trigger != mind.TriggerReturn {
		t.Errorf("trigger = %q, want %q", got.item.Trigger, mind.TriggerReturn)
	}
	if got.item.Volunteering == "" {
		t.Error("queued without saying why she is speaking")
	}
	if state := store.MindChannelState(testGuild, testChannel); state.Today != 1 {
		t.Errorf("budget used = %d, want 1", state.Today)
	}
	if svc.fatigue(testGuild, time.Now()) <= 0 {
		t.Error("speaking up unprompted did not tire her")
	}
}

// Off by default: a channel that was let in but never told she may speak first
// only ever gets answers.
func TestObserveDoesNotVolunteerUnlessSwitchedOn(t *testing.T) {
	store := testStore(t)
	if err := store.AddChatChannel(testGuild, testChannel); err != nil {
		t.Fatalf("AddChatChannel: %v", err)
	}
	regularAwayFor(t, store, 30*24*time.Hour)
	svc := awakeService(t, store)

	svc.Observe(testSession(), message("hey all", false))

	if got, ok := queued(svc); ok {
		t.Errorf("volunteered in a channel that never allowed it: %+v", got.item)
	}
}

// Greeting a stranger's return reads as being watched, not as recognition.
func TestObserveDoesNotGreetANewcomerComingBack(t *testing.T) {
	store := testStore(t)
	proactiveChannel(t, store)
	if _, err := store.SeeMindPerson(testGuild, "u1", "cass", time.Now().Add(-30*24*time.Hour)); err != nil {
		t.Fatalf("SeeMindPerson: %v", err)
	}
	svc := awakeService(t, store)

	svc.Observe(testSession(), message("hey all", false))

	if _, ok := queued(svc); ok {
		t.Error("greeted someone she has seen say one thing")
	}
}

func TestObserveBringsUpSomethingSheRemembers(t *testing.T) {
	store := testStore(t)
	proactiveChannel(t, store)
	if err := store.AddMindMemory(storage.MindMemory{
		GuildID:   testGuild,
		ChannelID: testChannel,
		At:        time.Now().Add(-3 * 24 * time.Hour),
		Gist:      "argument about the purge rules",
		Detail:    "whether pins survive a purge",
		Weight:    0.6,
	}); err != nil {
		t.Fatalf("AddMindMemory: %v", err)
	}
	svc := awakeService(t, store)

	svc.Observe(testSession(), message("are the purge rules changing again", false))

	got, ok := queued(svc)
	if !ok {
		t.Fatal("said nothing when a subject she remembers came round")
	}
	if got.item.Trigger != mind.TriggerRecall {
		t.Errorf("trigger = %q, want %q", got.item.Trigger, mind.TriggerRecall)
	}
}

// The allowance is persisted, not held in memory: the bot is redeployed often
// enough that a counter which reset on restart would be no limit at all.
func TestObserveStopsVolunteeringOnceTheDayIsSpent(t *testing.T) {
	store := testStore(t)
	proactiveChannel(t, store)
	regularAwayFor(t, store, 30*24*time.Hour)
	svc := awakeService(t, store)
	for i := 0; i < mind.VolunteerDailyMax; i++ {
		if err := store.MarkVolunteered(testGuild, testChannel, svc.day(time.Now()), time.Now().Add(-24*time.Hour)); err != nil {
			t.Fatalf("MarkVolunteered: %v", err)
		}
	}

	svc.Observe(testSession(), message("hey all", false))

	if _, ok := queued(svc); ok {
		t.Error("volunteered past the day's allowance")
	}
}

// An answer is owed and waits for a backend; a remark nobody asked for is not,
// and arriving long after its moment would be stranger than never saying it.
func TestHoldDropsWhatSheVolunteered(t *testing.T) {
	store := testStore(t)
	svc := awakeService(t, store)

	svc.hold(task{item: mind.Deferred{
		GuildID:   testGuild,
		ChannelID: testChannel,
		UserID:    "u1",
		Trigger:   mind.TriggerReturn,
		FormedAt:  time.Now(),
	}}, "no backend")

	if n := svc.Status().Waiting; n != 0 {
		t.Errorf("held %d volunteered remarks, want none", n)
	}
}

// An afterthought is dropped when the person has spoken since her message,
// not sent after their reply.
func TestAfterthoughtIsDroppedOnceOvertaken(t *testing.T) {
	store := testStore(t)
	svc := awakeService(t, store)
	svc.conv.Record(testChannel, mind.Turn{FromBot: true, MessageID: "b1", Content: "yes", At: time.Now()})
	svc.conv.Record(testChannel, mind.Turn{UserID: "u1", Username: "cass", Content: "why", At: time.Now()})

	svc.enqueueAfterthought(task{
		item:  mind.Deferred{ChannelID: testChannel, Trigger: mind.TriggerAfterthought, FirstLine: "yes"},
		after: "b1",
	})

	if _, ok := queued(svc); ok {
		t.Error("queued an afterthought after the person had already answered")
	}
}

func TestAfterthoughtIsQueuedWhileSheHasTheLastWord(t *testing.T) {
	store := testStore(t)
	svc := awakeService(t, store)
	svc.conv.Record(testChannel, mind.Turn{UserID: "u1", Username: "cass", Content: "bored?", At: time.Now()})
	svc.conv.Record(testChannel, mind.Turn{FromBot: true, MessageID: "b1", Content: "yes", At: time.Now()})

	svc.enqueueAfterthought(task{
		item:  mind.Deferred{ChannelID: testChannel, Trigger: mind.TriggerAfterthought, FirstLine: "yes"},
		after: "b1",
	})

	if _, ok := queued(svc); !ok {
		t.Error("dropped an afterthought while her message was still the last word")
	}
}

func TestAfterthoughtStandsOnlyWhenItAddsSomething(t *testing.T) {
	store := testStore(t)
	svc := awakeService(t, store)
	svc.conv.Record(testChannel, mind.Turn{FromBot: true, MessageID: "b1", Content: "yes", At: time.Now()})
	at := task{item: mind.Deferred{ChannelID: testChannel, FirstLine: "yes"}, after: "b1"}

	for reply, want := range map[string]bool{
		"entertain me, then": true,
		"SKIP":               false,
		"Yes.":               false,
	} {
		if got := svc.afterthoughtStands(at, reply); got != want {
			t.Errorf("afterthoughtStands(%q) = %v, want %v", reply, got, want)
		}
	}
}
