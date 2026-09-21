package storage

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestSeeMindPersonAccumulates(t *testing.T) {
	s := newTestStore(t)
	first := time.Now().Add(-time.Hour).UTC()
	later := time.Now().UTC()

	p, err := s.SeeMindPerson("g1", "u1", "ann", first)
	if err != nil {
		t.Fatalf("SeeMindPerson: %v", err)
	}
	if p.Messages != 1 || !p.FirstSeen.Equal(first) {
		t.Fatalf("first sighting = %+v", p)
	}

	p, err = s.SeeMindPerson("g1", "u1", "ann", later)
	if err != nil {
		t.Fatalf("SeeMindPerson: %v", err)
	}
	if p.Messages != 2 {
		t.Errorf("Messages = %d, want 2", p.Messages)
	}
	if !p.FirstSeen.Equal(first) {
		t.Errorf("FirstSeen moved to %v, want it pinned to %v", p.FirstSeen, first)
	}
	if !p.LastSeen.Equal(later) {
		t.Errorf("LastSeen = %v, want %v", p.LastSeen, later)
	}
}

func TestSeeMindPersonTracksARename(t *testing.T) {
	s := newTestStore(t)
	now := time.Now().UTC()

	if _, err := s.SeeMindPerson("g1", "u1", "oldname", now); err != nil {
		t.Fatalf("SeeMindPerson: %v", err)
	}
	p, err := s.SeeMindPerson("g1", "u1", "newname", now)
	if err != nil {
		t.Fatalf("SeeMindPerson: %v", err)
	}
	if p.Username != "newname" {
		t.Errorf("Username = %q, want the current one", p.Username)
	}

	// An empty username must not erase the one already known: the gateway does
	// not always carry it.
	p, err = s.SeeMindPerson("g1", "u1", "", now)
	if err != nil {
		t.Fatalf("SeeMindPerson: %v", err)
	}
	if p.Username != "newname" {
		t.Errorf("Username = %q, want it kept when the event carried none", p.Username)
	}
}

func TestMindPeopleAreGuildScoped(t *testing.T) {
	s := newTestStore(t)
	now := time.Now().UTC()

	if _, err := s.SeeMindPerson("g1", "u1", "ann", now); err != nil {
		t.Fatalf("SeeMindPerson: %v", err)
	}

	if got := s.GetMindPerson("g2", "u1"); got != nil {
		t.Errorf("member leaked into another guild: %+v", got)
	}
	if got := s.GetMindPerson("g1", "u1"); got == nil {
		t.Error("member not found in their own guild")
	}
}

func TestGetMindPersonReturnsNilForAStranger(t *testing.T) {
	s := newTestStore(t)
	if got := s.GetMindPerson("g1", "nobody"); got != nil {
		t.Errorf("GetMindPerson = %+v, want nil for someone never seen", got)
	}
}

// One member posting in two channels lands here twice at once. Without the
// transaction the counts are lost silently.
func TestSeeMindPersonDoesNotLoseConcurrentSightings(t *testing.T) {
	s := newTestStore(t)
	now := time.Now().UTC()

	const sightings = 30
	var wg sync.WaitGroup
	for i := 0; i < sightings; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.SeeMindPerson("g1", "u1", "ann", now); err != nil {
				t.Errorf("SeeMindPerson: %v", err)
			}
		}()
	}
	wg.Wait()

	p := s.GetMindPerson("g1", "u1")
	if p == nil {
		t.Fatal("member not recorded at all")
	}
	if p.Messages != sightings {
		t.Errorf("Messages = %d, want %d — concurrent sightings were lost", p.Messages, sightings)
	}
}

func TestChatChannelOptIn(t *testing.T) {
	s := newTestStore(t)
	const guild = "g1"

	if s.IsChatChannel(guild, "c1") {
		t.Error("a channel is opted in before anyone said so")
	}

	if err := s.AddChatChannel(guild, "c1"); err != nil {
		t.Fatalf("AddChatChannel: %v", err)
	}
	if !s.IsChatChannel(guild, "c1") {
		t.Error("channel not opted in after being added")
	}
	if err := s.AddChatChannel(guild, "c1"); err == nil {
		t.Error("AddChatChannel accepted a duplicate")
	}

	if got := s.GetChatChannels(guild); len(got) != 1 || got[0] != "c1" {
		t.Errorf("GetChatChannels = %v", got)
	}

	if err := s.RemoveChatChannel(guild, "c1"); err != nil {
		t.Fatalf("RemoveChatChannel: %v", err)
	}
	if s.IsChatChannel(guild, "c1") {
		t.Error("channel still opted in after removal")
	}
	if err := s.RemoveChatChannel(guild, "c1"); err == nil {
		t.Error("RemoveChatChannel accepted an unknown channel")
	}
}

func TestChatChannelsAreGuildScoped(t *testing.T) {
	s := newTestStore(t)
	if err := s.AddChatChannel("g1", "c1"); err != nil {
		t.Fatalf("AddChatChannel: %v", err)
	}
	if s.IsChatChannel("g2", "c1") {
		t.Error("opt-in leaked into another guild")
	}
}

func TestChatBriefRoundTrip(t *testing.T) {
	s := newTestStore(t)
	if got := s.GetChatBrief("g1"); got != "" {
		t.Errorf("GetChatBrief = %q, want empty before it is set", got)
	}
	if err := s.SetChatBrief("g1", "a small server"); err != nil {
		t.Fatalf("SetChatBrief: %v", err)
	}
	if got := s.GetChatBrief("g1"); got != "a small server" {
		t.Errorf("GetChatBrief = %q", got)
	}
	if got := s.GetChatBrief("g2"); got != "" {
		t.Errorf("brief leaked into another guild: %q", got)
	}
}

// What v1 learned about someone would seed a dossier again the next time she
// met them, so a forgotten guild has to lose it too.
func TestForgetMindClearsLegacyNotesAndKeepsWhoPeopleAre(t *testing.T) {
	store := newTestStore(t)
	now := time.Now()
	for _, guild := range []string{"g1", "g2"} {
		for i := 0; i < 5; i++ {
			if _, err := store.SeeMindPerson(guild, "u1", "cass", now); err != nil {
				t.Fatalf("SeeMindPerson: %v", err)
			}
		}
		if err := store.UpdateMindPerson(guild, "u1", now, func(p *MindPerson) {
			p.Impression = "sharp"
			p.Facts = []MindFact{{Key: "pet", Value: "a cat"}}
		}); err != nil {
			t.Fatalf("UpdateMindPerson: %v", err)
		}
	}

	if err := store.ForgetMind("g1"); err != nil {
		t.Fatalf("ForgetMind: %v", err)
	}
	person := store.GetMindPerson("g1", "u1")
	if person == nil || person.Messages != 5 {
		t.Fatalf("lost how well she knows them: %+v", person)
	}
	if person.Impression != "" || len(person.Facts) != 0 {
		t.Errorf("legacy notes survived: %+v", person)
	}
	if other := store.GetMindPerson("g2", "u1"); other == nil || other.Impression != "sharp" {
		t.Errorf("reached into another guild: %+v", other)
	}
}

func TestChatProactiveNeedsTheChannelFirst(t *testing.T) {
	store := newTestStore(t)

	if err := store.SetChatProactive("g1", "c1", true); !errors.Is(err, ErrChatChannelRequired) {
		t.Fatalf("SetChatProactive on a channel she cannot read = %v, want ErrChatChannelRequired", err)
	}
	if store.IsChatProactive("g1", "c1") {
		t.Error("proactive in a channel she was never let into")
	}

	if err := store.AddChatChannel("g1", "c1"); err != nil {
		t.Fatalf("AddChatChannel: %v", err)
	}
	if err := store.SetChatProactive("g1", "c1", true); err != nil {
		t.Fatalf("SetChatProactive: %v", err)
	}
	if !store.IsChatProactive("g1", "c1") {
		t.Error("not proactive after being switched on")
	}
}

// Left behind, proactivity would come back switched on the next time someone
// ran /chat here, which nobody asking for her back would expect.
func TestSilencingAChannelTakesProactivityWithIt(t *testing.T) {
	store := newTestStore(t)
	if err := store.AddChatChannel("g1", "c1"); err != nil {
		t.Fatalf("AddChatChannel: %v", err)
	}
	if err := store.SetChatProactive("g1", "c1", true); err != nil {
		t.Fatalf("SetChatProactive: %v", err)
	}

	if err := store.RemoveChatChannel("g1", "c1"); err != nil {
		t.Fatalf("RemoveChatChannel: %v", err)
	}
	if err := store.AddChatChannel("g1", "c1"); err != nil {
		t.Fatalf("AddChatChannel: %v", err)
	}
	if store.IsChatProactive("g1", "c1") {
		t.Error("proactivity survived being silenced and came back on its own")
	}
}

func TestMarkVolunteeredCountsPerDay(t *testing.T) {
	store := newTestStore(t)
	now := time.Now()

	for i := 0; i < 2; i++ {
		if err := store.MarkVolunteered("g1", "c1", "2026-09-18", now); err != nil {
			t.Fatalf("MarkVolunteered: %v", err)
		}
	}
	if got := store.MindChannelState("g1", "c1"); got.Today != 2 {
		t.Errorf("Today = %d, want 2", got.Today)
	}

	// A new day starts the budget again rather than carrying yesterday's.
	if err := store.MarkVolunteered("g1", "c1", "2026-09-19", now); err != nil {
		t.Fatalf("MarkVolunteered: %v", err)
	}
	if got := store.MindChannelState("g1", "c1"); got.Today != 1 || got.Day != "2026-09-19" {
		t.Errorf("after the day turned: %+v", got)
	}
}
