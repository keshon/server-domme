package mind

import (
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestConversationsKeepsRecentTurnsInOrder(t *testing.T) {
	c := NewConversations()
	now := time.Now()

	c.Record("chan", Turn{Username: "ann", Content: "first", At: now})
	c.Record("chan", Turn{Content: "second", At: now.Add(time.Second), FromBot: true})

	got := c.Recent("chan")
	if len(got) != 2 {
		t.Fatalf("got %d turns, want 2", len(got))
	}
	if got[0].Content != "first" || got[1].Content != "second" {
		t.Errorf("turns out of order: %+v", got)
	}
}

// A gap this long means the conversation ended. Replying into yesterday's
// thread as though it were live is the most visible way a chat bot gets it
// wrong.
// A quiet channel is a conversation too. Three messages across an afternoon
// are one exchange, and cutting at thirty minutes leaves her answering a
// single line with no idea what it is about.
func TestConversationsKeepsAQuietChannelsContext(t *testing.T) {
	c := NewConversations()
	now := time.Now()

	c.Record("chan", Turn{Content: "anyone about", At: now.Add(-4 * time.Hour)})
	c.Record("chan", Turn{Content: "sort of", At: now.Add(-2 * time.Hour)})
	c.Record("chan", Turn{Content: "so about that thing", At: now})

	got := c.Recent("chan")
	if len(got) != 3 {
		t.Fatalf("got %d turns, want all three of a slow conversation: %+v", len(got), got)
	}
}

// Past the horizon it is not context, it is history, and Memory renders
// history.
func TestConversationsDropsTurnsPastTheHorizon(t *testing.T) {
	c := NewConversations()
	now := time.Now()

	c.Record("chan", Turn{Content: "before anyone remembers", At: now.Add(-3 * MaxTurnAge)})
	c.Record("chan", Turn{Content: "still talking", At: now})

	got := c.Recent("chan")
	if len(got) != 1 {
		t.Fatalf("got %d turns, want only the live one: %+v", len(got), got)
	}
	if got[0].Content != "still talking" {
		t.Errorf("kept the wrong turn: %q", got[0].Content)
	}
}

// In a busy channel the window is what decides, because more than the floor
// has been said inside it.
func TestConversationsCutsABusyChannelByTime(t *testing.T) {
	c := NewConversations()
	now := time.Now()

	// Recorded in time order, as Record is always called. The old turn comes
	// first, then well past the count floor of recent chatter, so the floor
	// no longer reaches back far enough to rescue it.
	c.Record("chan", Turn{Content: "ancient", At: now.Add(-2 * TurnStaleAfter)})
	for i := MinLiveTurns * 2; i > 0; i-- {
		c.Record("chan", Turn{Content: "chatter", At: now.Add(-time.Duration(i) * time.Minute)})
	}
	c.Record("chan", Turn{Content: "now", At: now})

	for _, turn := range c.Recent("chan") {
		if turn.Content == "ancient" {
			t.Error("a busy channel kept something from before the window")
		}
	}
}

func TestConversationsBoundsHistoryPerChannel(t *testing.T) {
	c := NewConversations()
	now := time.Now()

	for i := 0; i < maxTurnsPerChannel*3; i++ {
		c.Record("chan", Turn{Content: fmt.Sprint(i), At: now.Add(time.Duration(i) * time.Second)})
	}

	if got := len(c.Recent("chan")); got > maxTurnsPerChannel {
		t.Errorf("kept %d turns, want at most %d", got, maxTurnsPerChannel)
	}
}

func TestConversationsKeepsChannelsApart(t *testing.T) {
	c := NewConversations()
	now := time.Now()

	c.Record("a", Turn{Content: "in a", At: now})
	c.Record("b", Turn{Content: "in b", At: now})

	if got := c.Recent("a"); len(got) != 1 || got[0].Content != "in a" {
		t.Errorf("channel a = %+v", got)
	}
	if got := c.Recent("b"); len(got) != 1 || got[0].Content != "in b" {
		t.Errorf("channel b = %+v", got)
	}
}

func TestConversationsEvictsColdestChannelWhenFull(t *testing.T) {
	c := NewConversations()
	base := time.Now()

	for i := 0; i < maxChannels; i++ {
		c.Record(fmt.Sprintf("chan-%d", i), Turn{
			Content: "hi",
			At:      base.Add(time.Duration(i) * time.Second),
		})
	}
	// chan-0 is the coldest, so the next new channel should displace it.
	c.Record("newcomer", Turn{Content: "hi", At: base.Add(time.Hour)})

	if got := c.Recent("chan-0"); len(got) != 0 {
		t.Errorf("coldest channel survived eviction: %+v", got)
	}
	if got := c.Recent("newcomer"); len(got) != 1 {
		t.Errorf("new channel was not recorded: %+v", got)
	}
}

func TestConversationsForgetClearsAChannel(t *testing.T) {
	c := NewConversations()
	c.Record("chan", Turn{Content: "hi", At: time.Now()})
	c.Forget("chan")
	if got := c.Recent("chan"); len(got) != 0 {
		t.Errorf("Forget left %d turns", len(got))
	}
}

func TestConversationsAreSafeUnderConcurrentUse(t *testing.T) {
	c := NewConversations()
	now := time.Now()

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ch := fmt.Sprintf("chan-%d", i%4)
			c.Record(ch, Turn{Content: "x", At: now})
			_ = c.Recent(ch)
		}(i)
	}
	wg.Wait()
}

func seedTurn(id, user, content string, at time.Time) Turn {
	return Turn{UserID: user, Username: user, Content: content, At: at, MessageID: id}
}

// The message that triggered the reply is already recorded by the time the
// backfill runs, and it is also in the history Discord returns. Without
// deduplication it appears twice, once as itself and once as its own echo.
func TestSeedDoesNotDuplicateTheTriggeringMessage(t *testing.T) {
	c := NewConversations()
	now := time.Now()

	live := seedTurn("m3", "cass", "so what do you think", now)
	c.Record("c1", live)

	c.Seed("c1", []Turn{
		seedTurn("m1", "cass", "anyone around", now.Add(-2*time.Minute)),
		seedTurn("m2", "newbie", "just got here", now.Add(-time.Minute)),
		live,
	})

	got := c.Recent("c1")
	if len(got) != 3 {
		t.Fatalf("got %d turns, want 3:\n%+v", len(got), got)
	}
	for i, want := range []string{"anyone around", "just got here", "so what do you think"} {
		if got[i].Content != want {
			t.Errorf("turn %d = %q, want %q", i, got[i].Content, want)
		}
	}
}

// Discord returns newest first; the buffer and the prompt both read oldest
// first.
func TestSeedKeepsChronologicalOrder(t *testing.T) {
	c := NewConversations()
	now := time.Now()

	c.Seed("c1", []Turn{
		seedTurn("m1", "a", "first", now.Add(-3*time.Minute)),
		seedTurn("m2", "b", "second", now.Add(-2*time.Minute)),
		seedTurn("m3", "c", "third", now.Add(-time.Minute)),
	})

	got := c.Recent("c1")
	for i := 1; i < len(got); i++ {
		if got[i].At.Before(got[i-1].At) {
			t.Fatalf("turn %d is older than the one before it:\n%+v", i, got)
		}
	}
}

func TestSeedTrimsToTheBufferCap(t *testing.T) {
	c := NewConversations()
	now := time.Now()

	var many []Turn
	for i := 0; i < maxTurnsPerChannel*2; i++ {
		many = append(many, seedTurn(
			"m"+strconv.Itoa(i), "a", "line", now.Add(-time.Duration(i)*time.Second)))
	}
	c.Seed("c1", many)

	if got := len(c.Recent("c1")); got > maxTurnsPerChannel {
		t.Errorf("kept %d turns, want at most %d", got, maxTurnsPerChannel)
	}
}

// A channel the bot cannot read history in fails identically every time, so
// one attempt is recorded either way.
func TestSeedIsAttemptedOnlyOnce(t *testing.T) {
	c := NewConversations()

	if !c.NeedsSeed("c1") {
		t.Fatal("a fresh channel should want seeding")
	}
	c.Seed("c1", nil)
	if c.NeedsSeed("c1") {
		t.Error("still wants seeding after a failed attempt; every reply would re-fetch")
	}
}

// Forgetting a channel and then refusing to read its history again would leave
// her permanently blank there.
func TestForgetAllowsSeedingAgain(t *testing.T) {
	c := NewConversations()
	c.Seed("c1", []Turn{seedTurn("m1", "a", "hello", time.Now())})

	c.Forget("c1")

	if !c.NeedsSeed("c1") {
		t.Error("a forgotten channel refuses to be re-seeded")
	}
}
