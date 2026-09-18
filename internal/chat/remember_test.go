package chat

import (
	"testing"
	"time"

	"github.com/keshon/server-domme/internal/mind"
	"github.com/keshon/server-domme/internal/storage"
)

// After a restart the buffer is refilled from Discord and can hold turns a
// stored memory already covers. Summarising those again is a duplicate memory
// and a wasted backend call.
func TestUnrememberedDropsWhatAMemoryAlreadyCovers(t *testing.T) {
	store := testStore(t)
	svc := newTestService(t, store, 0)
	now := time.Now()

	if err := store.AddMindMemory(storage.MindMemory{
		GuildID: testGuild, ChannelID: testChannel,
		At: now.Add(-time.Hour), Gist: "the earlier conversation",
	}); err != nil {
		t.Fatalf("AddMindMemory: %v", err)
	}

	turns := []mind.Turn{
		{Content: "covered", At: now.Add(-2 * time.Hour)},
		{Content: "covered too", At: now.Add(-time.Hour)},
		{Content: "new", At: now.Add(-10 * time.Minute)},
	}
	got := svc.unremembered(testGuild, testChannel, turns)
	if len(got) != 1 || got[0].Content != "new" {
		t.Errorf("unremembered = %+v, want only the turn after the memory", got)
	}
}

func TestAllChatChannelsSpansGuilds(t *testing.T) {
	store := testStore(t)
	for _, pair := range [][2]string{{"g1", "c1"}, {"g1", "c2"}, {"g2", "c3"}} {
		if err := store.AddChatChannel(pair[0], pair[1]); err != nil {
			t.Fatalf("AddChatChannel: %v", err)
		}
	}
	got := store.AllChatChannels()
	if len(got) != 3 || got["c3"] != "g2" || got["c1"] != "g1" {
		t.Errorf("AllChatChannels = %v", got)
	}
}
