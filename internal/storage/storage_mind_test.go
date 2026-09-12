package storage

import (
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

func TestAddMindMemoryKeepsThemOldestFirst(t *testing.T) {
	store := newTestStore(t)
	now := time.Now()

	for i, gist := range []string{"first thing", "second thing", "third thing"} {
		err := store.AddMindMemory(MindMemory{
			GuildID:   "g1",
			ChannelID: "c1",
			At:        now.Add(time.Duration(i) * time.Minute),
			Gist:      gist,
		})
		if err != nil {
			t.Fatalf("AddMindMemory: %v", err)
		}
	}

	got := store.MindMemories("g1", "c1")
	if len(got) != 3 {
		t.Fatalf("got %d memories, want 3", len(got))
	}
	for i, want := range []string{"first thing", "second thing", "third thing"} {
		if got[i].Gist != want {
			t.Errorf("memory %d = %q, want %q", i, got[i].Gist, want)
		}
	}
}

func TestMindMemoriesAreScopedToTheirChannelAndGuild(t *testing.T) {
	store := newTestStore(t)

	for _, m := range []MindMemory{
		{GuildID: "g1", ChannelID: "c1", Gist: "in c1"},
		{GuildID: "g1", ChannelID: "c2", Gist: "in c2"},
		{GuildID: "g2", ChannelID: "c1", Gist: "another guild"},
	} {
		if err := store.AddMindMemory(m); err != nil {
			t.Fatalf("AddMindMemory: %v", err)
		}
	}

	got := store.MindMemories("g1", "c1")
	if len(got) != 1 || got[0].Gist != "in c1" {
		t.Errorf("channel scoping leaked: %+v", got)
	}
	if all := store.MindMemories("g1", ""); len(all) != 2 {
		t.Errorf("guild-wide read returned %d, want 2", len(all))
	}
}

// A memory with nothing to remember is not a memory.
func TestAddMindMemoryRefusesAnEmptyGist(t *testing.T) {
	store := newTestStore(t)

	if err := store.AddMindMemory(MindMemory{GuildID: "g1", Gist: "   "}); err == nil {
		t.Error("accepted a memory with a blank gist")
	}
	if err := store.AddMindMemory(MindMemory{Gist: "something"}); err == nil {
		t.Error("accepted a memory with no guild")
	}
}

func TestMindMemoryWeightAndPeopleSurviveARoundTrip(t *testing.T) {
	store := newTestStore(t)

	err := store.AddMindMemory(MindMemory{
		GuildID: "g1", ChannelID: "c1",
		Gist:   "the row",
		Weight: 0.8,
		People: []string{"u1", "u2"},
	})
	if err != nil {
		t.Fatalf("AddMindMemory: %v", err)
	}

	got := store.MindMemories("g1", "c1")
	if len(got) != 1 {
		t.Fatalf("got %d memories", len(got))
	}
	if got[0].Weight != 0.8 {
		t.Errorf("Weight = %v, want 0.8", got[0].Weight)
	}
	if len(got[0].People) != 2 {
		t.Errorf("People = %v, want two of them", got[0].People)
	}
}
